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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	for _, name := range []string{"http", "ssh", "serv"} {
		t.Run(name, func(t *testing.T) {
			repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
			remote := gitTransports(t, owner, repo)[name]
			dir := remote.clone(t)
			assert.Equal(t, gitMainOnDisk(t, repo), gitRun(t, dir, "rev-parse", "HEAD"))
			fileName := UniqueName("file-") + ".txt"
			want := gitCommitFile(t, dir, fileName)
			out, err := remote.git(dir, "push", "origin", "main")
			require.NoError(t, err, "push: %s", out)
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

			// The server records the push in the news feed of the owner.
			actions := gitRepoActions(t, owner.ID, repo.ID)
			require.NotEmpty(t, actions)
			pushed := actions[0]
			assert.Equal(t, database.ActionCommitRepo, pushed.OpType)
			assert.Equal(t, owner.ID, pushed.ActUserID)
			assert.Equal(t, "main", pushed.RefName)
			var commits database.PushCommits
			require.NoError(t, json.Unmarshal([]byte(pushed.Content), &commits))
			require.Len(t, commits.Commits, 1)
			assert.Equal(t, want, commits.Commits[0].Sha1)
			assert.Equal(t, "Add "+fileName, strings.TrimSpace(commits.Commits[0].Message))
		})
	}

	t.Run("token", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		token, err := CreateAccessToken(t.Context(), owner.ID)
		require.NoError(t, err)
		dir := gitClone(t, gitRemoteURL(t, repo, token, ""))
		want := gitCommitFile(t, dir, UniqueName("file-")+".txt")
		gitRun(t, dir, "push", "origin", "main")
		assert.Equal(t, want, gitMainOnDisk(t, repo))
	})
}

// gitRepoActions returns the actions of the repository in the news feed of the
// owner, newest first.
func gitRepoActions(t *testing.T, ownerID, repoID int64) []*database.Action {
	t.Helper()

	actions, err := database.Handle.Actions().ListByUser(t.Context(), ownerID, ownerID, 0, false)
	require.NoError(t, err)
	var repoActions []*database.Action
	for _, a := range actions {
		if a.RepoID == repoID {
			repoActions = append(repoActions, a)
		}
	}
	return repoActions
}

// gitRefAction is the type and reference of an action.
type gitRefAction struct {
	OpType  database.ActionType
	RefName string
}

// gitNewRefActions returns the type and reference of the actions of the
// repository in the news feed of the owner that come after the first skip
// actions, oldest first.
func gitNewRefActions(t *testing.T, ownerID, repoID int64, skip int) []gitRefAction {
	t.Helper()

	actions := gitRepoActions(t, ownerID, repoID)
	var got []gitRefAction
	for i := len(actions) - 1 - skip; i >= 0; i-- {
		assert.Equal(t, ownerID, actions[i].ActUserID)
		got = append(got, gitRefAction{OpType: actions[i].OpType, RefName: actions[i].RefName})
	}
	return got
}

// gitTransport runs Git over one transport to the server.
type gitTransport struct {
	// url is the URL of the repository over the transport.
	url string
	// git runs Git in the directory and returns its trimmed output.
	git func(dir string, args ...string) (string, error)
}

// gitTransports returns, by name, each transport to the repository as the user:
// HTTP with the password, the builtin SSH server, and "gogs serv", which
// OpenSSH runs.
func gitTransports(t *testing.T, u *User, repo *Repository) map[string]gitTransport {
	t.Helper()

	key := sshUserKey(t, u.ID)
	serv := servProgram(t, key.id)
	run := func(cmd *exec.Cmd) (string, error) {
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	return map[string]gitTransport{
		"http": {
			url: gitRemoteURL(t, repo, u.Name, u.Password),
			git: func(dir string, args ...string) (string, error) {
				return run(gitCommand(dir, args...))
			},
		},
		"ssh": {url: sshRemoteURL(repo), git: key.git},
		"serv": {
			url: "ssh://git@localhost/" + repo.Owner + "/" + repo.Name + ".git",
			git: func(dir string, args ...string) (string, error) {
				cmd := gitCommand(dir, args...)
				cmd.Env = append(cmd.Env, "GIT_SSH="+serv)
				return run(cmd)
			},
		},
	}
}

// clone clones the repository over the transport into a new directory and
// returns the directory.
func (tr gitTransport) clone(t *testing.T) string {
	t.Helper()

	dir := filepath.Join(t.TempDir(), "clone")
	out, err := tr.git(filepath.Dir(dir), "clone", tr.url, dir)
	require.NoError(t, err, "clone: %s", out)
	return dir
}

func TestGitPushRecordsRefUpdates(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	for _, name := range []string{"http", "ssh"} {
		t.Run(name, func(t *testing.T) {
			repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
			remote := gitTransports(t, owner, repo)[name]
			dir := remote.clone(t)
			gitCommitFile(t, dir, UniqueName("file-")+".txt")
			gitRun(t, dir, "branch", "gone")
			out, err := remote.git(dir, "push", "origin", "main", "gone")
			require.NoError(t, err, "push: %s", out)
			before := len(gitRepoActions(t, owner.ID, repo.ID))

			// One push that creates a branch and a tag and deletes a branch records
			// each update, as Git reports it applied them.
			want := gitCommitFile(t, dir, UniqueName("file-")+".txt")
			gitRun(t, dir, "tag", "v1")
			out, err = remote.git(dir, "push", "origin", "HEAD:refs/heads/feature", "v1", ":gone")
			require.NoError(t, err, "push: %s", out)

			// A new branch with commits also records its commits.
			assert.ElementsMatch(t, []gitRefAction{
				{OpType: database.ActionCreateBranch, RefName: "feature"},
				{OpType: database.ActionCommitRepo, RefName: "feature"},
				{OpType: database.ActionPushTag, RefName: "v1"},
				{OpType: database.ActionDeleteBranch, RefName: "gone"},
			}, gitNewRefActions(t, owner.ID, repo.ID, before))
			assert.Equal(t, want, gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "rev-parse", "refs/heads/feature"))
		})
	}
}

func TestGitPushProtectedBranch(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	branch := repoSettingsDefaultBranch(t, repo)
	hooksDir := filepath.Join(database.RepoPath(repo.Owner, repo.Name), "hooks")
	path := repoSettingsAPIPath(repo, "/settings/branches/"+branch)
	c := authenticatedClient(t, owner)

	// Pushes to a repository without protected branches or custom hooks run no
	// Gogs hook.
	for _, hook := range []string{"pre-receive", "update", "post-receive"} {
		assert.NoFileExists(t, filepath.Join(hooksDir, hook))
	}

	status, body := c.do(http.MethodPut, path, map[string]any{"protected": true})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.FileExists(t, filepath.Join(hooksDir, "pre-receive"))

	for name, remote := range gitTransports(t, owner, repo) {
		t.Run(name, func(t *testing.T) {
			dir := remote.clone(t)
			gitCommitFile(t, dir, UniqueName("file-")+".txt")
			out, err := remote.git(dir, "push", "origin", branch)
			require.NoError(t, err, "push: %s", out)
			head := gitMainOnDisk(t, repo)
			before := len(gitRepoActions(t, owner.ID, repo.ID))

			gitRun(t, dir, "commit", "--amend", "-m", "Rewritten")
			out, err = remote.git(dir, "push", "--force", "origin", branch)
			require.Error(t, err, "force push succeeded: %s", out)
			assert.Contains(t, out, "Gogs: Branch '"+branch+"' is protected from force push")
			assert.Equal(t, head, gitMainOnDisk(t, repo))
			assert.Len(t, gitRepoActions(t, owner.ID, repo.ID), before)
		})
	}

	status, body = c.do(http.MethodPut, path, map[string]any{"protected": false})
	require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	assert.NoFileExists(t, filepath.Join(hooksDir, "pre-receive"))
	dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	gitRun(t, dir, "commit", "--amend", "-m", "Rewritten")
	want := gitRun(t, dir, "rev-parse", "HEAD")
	gitRun(t, dir, "push", "--force", "origin", branch)
	assert.Equal(t, want, gitMainOnDisk(t, repo))
}

func TestGitPushWithoutWriteAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	head := gitMainOnDisk(t, repo)

	t.Run("anonymous", func(t *testing.T) {
		dir := gitClone(t, gitRemoteURL(t, repo, "", ""))
		gitCommitFile(t, dir, UniqueName("file-")+".txt")
		out, err := gitCommand(dir, "push", "origin", "main").CombinedOutput()
		require.Error(t, err, "push succeeded: %s", out)
		assert.Equal(t, head, gitMainOnDisk(t, repo))
	})

	// The user can read the public repository but not push to it.
	denied := map[string]string{
		"http": "User permission denied",
		"ssh":  "You do not have sufficient authorization for this action",
		"serv": "You do not have sufficient authorization for this action",
	}
	for name, remote := range gitTransports(t, other, repo) {
		t.Run("user without write access over "+name, func(t *testing.T) {
			dir := remote.clone(t)
			gitCommitFile(t, dir, UniqueName("file-")+".txt")
			out, err := remote.git(dir, "push", "origin", "main")
			require.Error(t, err, "push succeeded: %s", out)
			assert.Equal(t, head, gitMainOnDisk(t, repo))
			assert.Contains(t, out, denied[name])
		})
	}
}

func TestGitPushWiki(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	// wikiOf returns the repository whose URLs are those of the wiki.
	wikiOf := func(repo *Repository) *Repository {
		return &Repository{ID: repo.ID, Owner: repo.Owner, Name: repo.Name + ".wiki"}
	}
	wikiPath := func(repo *Repository) string {
		return database.WikiPath(repo.Owner, repo.Name)
	}
	// newWikiCheckout returns a new checkout with a commit of the home page on
	// the branch "main", whose remote "origin" is the URL.
	newWikiCheckout := func(t *testing.T, remoteURL string) (dir, commit string) {
		dir = t.TempDir()
		gitRun(t, dir, "init", "--initial-branch=main")
		gitRun(t, dir, "remote", "add", "origin", remoteURL)
		return dir, gitCommitFile(t, dir, "Home.md")
	}
	getWiki := func(t *testing.T, repo *Repository) (hasWiki bool, pages []string) {
		var wiki struct {
			HasWiki bool     `json:"hasWiki"`
			Pages   []string `json:"pages"`
		}
		newClient(t).doJSON(http.MethodGet, repoCodeAPI(repo)+"/wiki", nil, http.StatusOK, &wiki)
		return wiki.HasWiki, wiki.Pages
	}

	for _, name := range []string{"http", "ssh", "serv"} {
		t.Run("first push over "+name, func(t *testing.T) {
			repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
			remote := gitTransports(t, owner, wikiOf(repo))[name]
			hasWiki, _ := getWiki(t, repo)
			require.False(t, hasWiki)

			dir, want := newWikiCheckout(t, remote.url)
			// Git names "master" in HEAD of a new wiki by default, and clones
			// still check out the pushed branch.
			out, err := remote.git(dir, "push", "origin", "main")
			require.NoError(t, err, "push: %s", out)

			assert.Equal(t, want, gitRun(t, t.TempDir(), "--git-dir", wikiPath(repo), "rev-parse", "refs/heads/main"))
			hasWiki, pages := getWiki(t, repo)
			assert.True(t, hasWiki)
			assert.Equal(t, []string{"Home"}, pages)
			assert.Equal(t, want, gitRun(t, remote.clone(t), "rev-parse", "HEAD"))
		})
	}

	// Each case expects the push to fail and no wiki to be created.
	other := seedUser(t, CreateUserOptions{})
	readOnly := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoSettingsAddCollaborator(t, readOnly, other, database.AccessModeRead)
	disabled := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	external := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	api := apiV1Client(t, owner)
	for repo, form := range map[*Repository]map[string]any{
		disabled: {"enable_wiki": false},
		external: {"enable_wiki": true, "enable_external_wiki": true, "external_wiki_url": "https://wiki.example.com"},
	} {
		status, body := api.do(http.MethodPatch, apiV1RepoPath(repo, "/wiki"), form)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
	}
	for _, test := range []struct {
		name   string
		pusher *User
		repo   *Repository
	}{
		{name: "user without write access", pusher: other, repo: readOnly},
		{name: "wiki disabled", pusher: owner, repo: disabled},
		{name: "external wiki", pusher: owner, repo: external},
	} {
		for name, remote := range gitTransports(t, test.pusher, wikiOf(test.repo)) {
			t.Run(test.name+" over "+name, func(t *testing.T) {
				dir, _ := newWikiCheckout(t, remote.url)
				out, err := remote.git(dir, "push", "origin", "main")
				require.Error(t, err, "push succeeded: %s", out)
				assert.NoDirExists(t, wikiPath(test.repo))
			})
		}
	}

	t.Run("clone of a wiki without pages", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		for name, remote := range gitTransports(t, owner, wikiOf(repo)) {
			out, err := remote.git(t.TempDir(), "clone", remote.url, "clone")
			require.Error(t, err, "clone over %s succeeded: %s", name, out)
		}
		assert.NoDirExists(t, wikiPath(repo))
	})

	// A push that pushes nothing still creates the wiki repository, and the
	// wiki has no pages until a page is pushed or created on the web.
	t.Run("dry run", func(t *testing.T) {
		repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		remote := gitTransports(t, owner, wikiOf(repo))["http"]
		dir, _ := newWikiCheckout(t, remote.url)
		out, err := remote.git(dir, "push", "--dry-run", "origin", "main")
		require.NoError(t, err, "push: %s", out)
		require.DirExists(t, wikiPath(repo))

		hasWiki, _ := getWiki(t, repo)
		assert.False(t, hasWiki)
		status, body := newClient(t).do(http.MethodGet, repoCodeAPI(repo)+"/wiki/pages", nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = newClient(t).do(http.MethodGet, repoCodePage(repo)+"/wiki/logo.png", nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)

		authenticatedClient(t, owner).doJSON(http.MethodPost, repoCodeAPI(repo)+"/wiki", map[string]any{
			"title":   "Home",
			"content": "Welcome.",
		}, http.StatusCreated, &struct{}{})
		hasWiki, pages := getWiki(t, repo)
		assert.True(t, hasWiki)
		assert.Equal(t, []string{"Home"}, pages)
	})
}
