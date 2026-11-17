package integrationtests

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

// gitCommand returns a Git command that runs in the directory, never prompts
// for credentials and commits as a fixed identity.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_ASKPASS=",
		"GIT_AUTHOR_NAME=Integration Test",
		"GIT_AUTHOR_EMAIL=integration@example.com",
		"GIT_COMMITTER_NAME=Integration Test",
		"GIT_COMMITTER_EMAIL=integration@example.com",
	)
	return cmd
}

// gitRun runs Git in the directory, fails the test when it fails, and returns
// its trimmed output.
func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()

	out, err := gitCommand(dir, args...).CombinedOutput()
	require.NoError(t, err, "git %s: %s", strings.Join(args, " "), out)
	return strings.TrimSpace(string(out))
}

// gitRemoteURL returns the HTTP URL of the repository, with the credentials
// when the username is not empty.
func gitRemoteURL(t *testing.T, repo *Repository, username, password string) string {
	t.Helper()

	u, err := url.Parse(serverURL + "/" + repo.Owner + "/" + repo.Name + ".git")
	require.NoError(t, err)
	if username != "" {
		u.User = url.UserPassword(username, password)
	}
	return u.String()
}

// gitClone clones the repository into a new directory and returns the
// directory.
func gitClone(t *testing.T, remoteURL string) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clone")
	gitRun(t, filepath.Dir(dir), "clone", remoteURL, dir)
	return dir
}

// gitCommitFile commits a new file with unique content in the checkout and
// returns the SHA of the commit.
func gitCommitFile(t *testing.T, dir, name string) string {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(UniqueName("content-")+"\n"), 0o600))
	gitRun(t, dir, "add", name)
	gitRun(t, dir, "commit", "-m", "Add "+name)
	return gitRun(t, dir, "rev-parse", "HEAD")
}

// gitMainOnDisk returns the SHA the main branch points to in the bare
// repository the server stores.
func gitMainOnDisk(t *testing.T, repo *Repository) string {
	t.Helper()

	return gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "refs/heads/main")
}

func TestGitInfoRefs(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	head := gitMainOnDisk(t, repo)
	infoRefs := "/" + repo.Owner + "/" + repo.Name + ".git/info/refs"

	c := newClient(t)
	for _, service := range []string{"git-upload-pack", "git-receive-pack"} {
		t.Run(service, func(t *testing.T) {
			c := c
			if service == "git-receive-pack" {
				c = newClient(t)
				c.header.Set("Authorization", "Basic "+gitBasicAuth(owner.Name, owner.Password))
			}
			status, header, body := c.doRaw(http.MethodGet, infoRefs+"?service="+service, "", nil)
			require.Equal(t, http.StatusOK, status, "response: %s", body)
			assert.Equal(t, "application/x-"+service+"-advertisement", header.Get("Content-Type"))
			assert.Equal(t, "no-cache, max-age=0, must-revalidate", header.Get("Cache-Control"))

			firstLine := "# service=" + service + "\n"
			require.True(t, strings.HasPrefix(string(body), gitPktLine(firstLine)+"0000"), "response: %s", body)
			assert.Contains(t, string(body), head+" refs/heads/main")
		})
	}

	t.Run("missing service", func(t *testing.T) {
		status, _, body := c.doRaw(http.MethodGet, infoRefs, "", nil)
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	})

	t.Run("pushing needs credentials", func(t *testing.T) {
		status, header, body := c.doRaw(http.MethodGet, infoRefs+"?service=git-receive-pack", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Equal(t, `Basic realm="."`, header.Get("WWW-Authenticate"))
	})

	t.Run("missing repository", func(t *testing.T) {
		status, _, body := c.doRaw(http.MethodGet, "/"+repo.Owner+"/"+UniqueName("missing-")+".git/info/refs?service=git-upload-pack", "", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

// gitPktLine encodes the string as a pkt-line of the Git protocol.
func gitPktLine(s string) string {
	const hex = "0123456789abcdef"
	n := len(s) + 4
	return string([]byte{hex[n>>12&0xf], hex[n>>8&0xf], hex[n>>4&0xf], hex[n&0xf]}) + s
}

// gitBasicAuth returns the credentials of HTTP basic authentication, to send
// after "Basic " in the Authorization header.
func gitBasicAuth(username, password string) string {
	return base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
}

// Gogs answers preflight requests only when [http] ACCESS_CONTROL_ALLOW_ORIGIN
// is set, which the server under test leaves empty, so OPTIONS requests are
// rejected without running Git.
func TestGitOptionsWithoutCORS(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	base := "/" + repo.Owner + "/" + repo.Name + ".git"

	anonymous := newClient(t)
	authed := newClient(t)
	authed.header.Set("Authorization", "Basic "+gitBasicAuth(owner.Name, owner.Password))

	for _, path := range []string{"/info/refs?service=git-upload-pack", "/git-upload-pack", "/git-receive-pack"} {
		t.Run(path, func(t *testing.T) {
			status, header, body := authed.doRaw(http.MethodOptions, base+path, "", nil)
			assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
			assert.Empty(t, header.Get("Access-Control-Allow-Origin"))
			assert.NotContains(t, string(body), "refs/heads/main")

			status, header, body = anonymous.doRaw(http.MethodOptions, base+path, "", nil)
			if path == "/git-receive-pack" {
				assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
				assert.Equal(t, `Basic realm="."`, header.Get("WWW-Authenticate"))
			} else {
				assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
			}
			assert.Empty(t, header.Get("Access-Control-Allow-Origin"))
		})
	}
}

func TestGitCloneAndFetch(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	dir := gitClone(t, gitRemoteURL(t, repo, "", ""))
	assert.Equal(t, gitMainOnDisk(t, repo), gitRun(t, dir, "rev-parse", "HEAD"))
	_, err := os.Stat(filepath.Join(dir, "README.md"))
	require.NoError(t, err)

	// A commit pushed by another checkout reaches this one by fetching.
	pusher := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	want := gitCommitFile(t, pusher, "fetched.txt")
	gitRun(t, pusher, "push", "origin", "main")
	gitRun(t, dir, "fetch", "origin")
	assert.Equal(t, want, gitRun(t, dir, "rev-parse", "origin/main"))

	t.Run("wrong content type", func(t *testing.T) {
		status, _, body := newClient(t).doRaw(http.MethodPost, "/"+repo.Owner+"/"+repo.Name+".git/git-upload-pack", "application/octet-stream", strings.NewReader("0000"))
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})
}

func TestGitClonePrivate(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	head := gitMainOnDisk(t, repo)
	token, err := CreateAccessToken(t.Context(), owner.ID)
	require.NoError(t, err)

	cloneFails := func(t *testing.T, remoteURL string) string {
		t.Helper()

		out, err := gitCommand(t.TempDir(), "clone", remoteURL, "clone").CombinedOutput()
		require.Error(t, err, "clone succeeded: %s", out)
		return string(out)
	}

	t.Run("anonymous", func(t *testing.T) {
		out := cloneFails(t, gitRemoteURL(t, repo, "", ""))
		assert.Contains(t, out, "could not read Username")

		status, header, body := newClient(t).doRaw(http.MethodGet, "/"+repo.Owner+"/"+repo.Name+".git/info/refs?service=git-upload-pack", "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Equal(t, `Basic realm="."`, header.Get("WWW-Authenticate"))
		assert.NotContains(t, string(body), head)
	})

	t.Run("wrong password", func(t *testing.T) {
		out := cloneFails(t, gitRemoteURL(t, repo, owner.Name, other.Password))
		assert.Contains(t, out, "Authentication failed")
	})

	t.Run("user without access", func(t *testing.T) {
		out := cloneFails(t, gitRemoteURL(t, repo, other.Name, other.Password))
		assert.Contains(t, out, "403")
	})

	for name, creds := range map[string][2]string{
		"password":          {owner.Name, owner.Password},
		"token as username": {token, ""},
		"token as password": {owner.Name, token},
	} {
		t.Run(name, func(t *testing.T) {
			dir := gitClone(t, gitRemoteURL(t, repo, creds[0], creds[1]))
			assert.Equal(t, head, gitRun(t, dir, "rev-parse", "HEAD"))
		})
	}
}

func TestGitPush(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	fileName := UniqueName("file-") + ".txt"
	want := gitCommitFile(t, dir, fileName)
	gitRun(t, dir, "push", "origin", "main")

	assert.Equal(t, want, gitMainOnDisk(t, repo))

	var src struct {
		Entries []struct {
			Name string `json:"name"`
		} `json:"entries"`
	}
	newClient(t).doJSON(http.MethodGet, "/api/web/"+repo.Owner+"/"+repo.Name+"/src", nil, http.StatusOK, &src)
	var names []string
	for _, e := range src.Entries {
		names = append(names, e.Name)
	}
	assert.ElementsMatch(t, []string{"README.md", fileName}, names)

	// The post-receive hook records the push in the news feed of the owner.
	actions, err := database.Handle.Actions().ListByUser(t.Context(), owner.ID, owner.ID, 0, false)
	require.NoError(t, err)
	var pushed *database.Action
	for _, a := range actions {
		if a.OpType == database.ActionCommitRepo && a.RepoID == repo.ID {
			pushed = a
			break
		}
	}
	require.NotNil(t, pushed, "no push action recorded")
	assert.Equal(t, owner.ID, pushed.ActUserID)
	assert.Equal(t, "main", pushed.RefName)
	var commits database.PushCommits
	require.NoError(t, json.Unmarshal([]byte(pushed.Content), &commits))
	require.Len(t, commits.Commits, 1)
	assert.Equal(t, want, commits.Commits[0].Sha1)
	assert.Equal(t, "Add "+fileName, strings.TrimSpace(commits.Commits[0].Message))

	t.Run("token", func(t *testing.T) {
		token, err := CreateAccessToken(t.Context(), owner.ID)
		require.NoError(t, err)
		dir := gitClone(t, gitRemoteURL(t, repo, token, ""))
		want := gitCommitFile(t, dir, UniqueName("file-")+".txt")
		gitRun(t, dir, "push", "origin", "main")
		assert.Equal(t, want, gitMainOnDisk(t, repo))
	})
}

func TestGitPushWithoutWriteAccess(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	head := gitMainOnDisk(t, repo)

	for name, creds := range map[string][2]string{
		"anonymous":           {"", ""},
		"user without access": {other.Name, other.Password},
	} {
		t.Run(name, func(t *testing.T) {
			dir := gitClone(t, gitRemoteURL(t, repo, creds[0], creds[1]))
			gitCommitFile(t, dir, UniqueName("file-")+".txt")
			out, err := gitCommand(dir, "push", "origin", "main").CombinedOutput()
			require.Error(t, err, "push succeeded: %s", out)
			assert.Equal(t, head, gitMainOnDisk(t, repo))
		})
	}

	t.Run("receive-pack request", func(t *testing.T) {
		c := newClient(t)
		c.header.Set("Authorization", "Basic "+gitBasicAuth(other.Name, other.Password))
		status, _, body := c.doRaw(http.MethodPost, "/"+repo.Owner+"/"+repo.Name+".git/git-receive-pack", "application/x-git-receive-pack-request", strings.NewReader("0000"))
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		assert.Contains(t, string(body), "User permission denied")
	})
}
