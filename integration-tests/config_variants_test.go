package integrationtests

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRequireSigninView covers what [auth] REQUIRE_SIGNIN_VIEW changes on every
// way in. Each case also calls the server without it, so a case fails when it
// tests behavior the option does not decide.
func TestRequireSigninView(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	head := gitMainOnDisk(t, repo)

	// anonymousClients returns an anonymous client of the server without the
	// option and one of the server with it.
	anonymousClients := func(t *testing.T) (open, closed *client) {
		open = newClient(t)
		closed = newClient(t)
		closed.baseURL = requireSigninViewServerURL
		return open, closed
	}

	t.Run("web API", func(t *testing.T) {
		open, closed := anonymousClients(t)
		path := "/api/web/" + repo.Owner + "/" + repo.Name + "/header"
		status, body := open.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = closed.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)

		// Visitors still sign in, and then see the repository.
		closed.signIn(owner)
		status, body = closed.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
	})

	t.Run("files", func(t *testing.T) {
		open, closed := anonymousClients(t)
		for _, path := range []string{
			"/" + repo.Owner + "/" + repo.Name + "/archive/main.zip",
			"/" + owner.Name + ".keys",
		} {
			status, _, body := open.doRaw(http.MethodGet, path, "", nil)
			assert.Equal(t, http.StatusOK, status, "%s: %s", path, body)
			status, _, body = closed.doRaw(http.MethodGet, path, "", nil)
			assert.Equal(t, http.StatusUnauthorized, status, "%s: %s", path, body)
		}

		// Visitors cannot find who has an email address.
		path := "/user/email2user?email=" + url.QueryEscape(owner.Email)
		status, header, body := open.doRaw(http.MethodGet, path, "", nil)
		assert.Equal(t, http.StatusFound, status, "response: %s", body)
		assert.Equal(t, "/"+owner.Name, header.Get("Location"))
		status, header, body = closed.doRaw(http.MethodGet, path, "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Empty(t, header.Get("Location"))
	})

	t.Run("API v1", func(t *testing.T) {
		open, closed := anonymousClients(t)
		path := apiV1RepoPath(repo, "")
		status, body := open.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = closed.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)
		assert.Contains(t, string(body), "Only authenticated user is allowed")

		c := apiV1Client(t, owner)
		c.baseURL = requireSigninViewServerURL
		status, body = c.do(http.MethodGet, path, nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
	})

	t.Run("Git over HTTP", func(t *testing.T) {
		repoURL := func(base, username, password string) string {
			u, err := url.Parse(base + "/" + repo.Owner + "/" + repo.Name + ".git")
			require.NoError(t, err)
			if username != "" {
				u.User = url.UserPassword(username, password)
			}
			return u.String()
		}
		gitClone(t, repoURL(serverURL, "", ""))

		// Git prompts for credentials, rather than reporting the repository as
		// missing.
		out, err := gitCommand(t.TempDir(), "clone", repoURL(requireSigninViewServerURL, "", ""), "clone").CombinedOutput()
		require.Error(t, err, "clone succeeded: %s", out)
		assert.Contains(t, string(out), "could not read Username")

		dir := gitClone(t, repoURL(requireSigninViewServerURL, owner.Name, owner.Password))
		assert.Equal(t, head, gitRun(t, dir, "rev-parse", "HEAD"))
	})

	t.Run("LFS", func(t *testing.T) {
		content, oid := lfsObject()
		status, _, body := lfsClient(t, owner).doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+oid, "application/octet-stream", strings.NewReader(string(content)))
		require.Equal(t, http.StatusOK, status, "response: %s", body)

		path := lfsBasePath(repo) + "/basic/" + oid
		status, _, body = lfsClient(t, nil).doRaw(http.MethodGet, path, "", nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)

		closed := lfsClient(t, nil)
		closed.baseURL = requireSigninViewServerURL
		status, header, body := closed.doRaw(http.MethodGet, path, "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Equal(t, `Basic realm="Git LFS"`, header.Get("Lfs-Authenticate"))

		authenticated := lfsClient(t, owner)
		authenticated.baseURL = requireSigninViewServerURL
		status, _, body = authenticated.doRaw(http.MethodGet, path, "", nil)
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)
	})

	// A deploy key stands for no user, so it only reads the repositories it is
	// added to, even public ones.
	t.Run("SSH deploy key", func(t *testing.T) {
		other := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		key := sshDeployKey(t, other)
		remoteURL := func(port int) string {
			return fmt.Sprintf("ssh://git@127.0.0.1:%d/%s/%s.git", port, repo.Owner, repo.Name)
		}

		dir := filepath.Join(t.TempDir(), "clone")
		out, err := key.git(filepath.Dir(dir), "clone", remoteURL(sshPort), dir)
		require.NoError(t, err, "clone: %s", out)

		out, err = key.git(t.TempDir(), "clone", remoteURL(requireSigninViewSSHPort), "clone")
		require.Error(t, err, "clone succeeded: %s", out)
		assert.Contains(t, out, "Key access denied")

		otherURL := fmt.Sprintf("ssh://git@127.0.0.1:%d/%s/%s.git", requireSigninViewSSHPort, other.Owner, other.Name)
		dir = filepath.Join(t.TempDir(), "clone")
		out, err = key.git(filepath.Dir(dir), "clone", otherURL, dir)
		require.NoError(t, err, "clone: %s", out)
		assert.Equal(t, gitMainOnDisk(t, other), gitRun(t, dir, "rev-parse", "HEAD"))
	})
}

// TestSubpath covers a server under a subpath, behind a reverse proxy that
// strips it, which builds every URL it sends with the subpath.
func TestSubpath(t *testing.T) {
	t.Parallel()

	parsed, err := url.Parse(subpathServerURL)
	require.NoError(t, err)
	subpath := parsed.Path

	// The owner is named like the subpath, so a server that strips the subpath
	// from paths that have none mangles the paths of its repositories.
	owner := seedUser(t, CreateUserOptions{NamePrefix: strings.TrimPrefix(subpath, "/") + "-"})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	newSubpathClient := func(t *testing.T) *client {
		c := newClient(t)
		c.baseURL = subpathServerURL
		return c
	}

	t.Run("sign in", func(t *testing.T) {
		// Browsers without Sec-Fetch-Site send the origin of the external URL,
		// while the server sees the host of the proxy target.
		signIn := func(c *client, origin string) (*http.Response, []byte) {
			req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, subpathServerURL+"/api/web/user/sign-in",
				strings.NewReader(fmt.Sprintf(`{"username":%q,"password":%q}`, owner.Name, owner.Password)))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Origin", origin)
			resp, err := c.http.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			require.NoError(t, err)
			return resp, body
		}

		resp, body := signIn(newSubpathClient(t), "http://evil.example.com")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, "response: %s", body)

		c := newSubpathClient(t)
		resp, body = signIn(c, parsed.Scheme+"://"+parsed.Host)
		require.Equal(t, http.StatusOK, resp.StatusCode, "response: %s", body)
		cookies := resp.Cookies()
		require.NotEmpty(t, cookies)
		for _, cookie := range cookies {
			assert.Equal(t, subpath, cookie.Path, "cookie %q", cookie.Name)
		}
		assert.Equal(t, owner.Name, c.username())
	})

	t.Run("page", func(t *testing.T) {
		status, _, body := newSubpathClient(t).doRaw(http.MethodGet, "/", "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Contains(t, string(body), fmt.Sprintf(`"subURL":%q`, subpath))
	})

	t.Run("redirects", func(t *testing.T) {
		for path, want := range map[string]struct {
			status   int
			location string
		}{
			"/explore":                          {http.StatusFound, subpath + "/explore/repos"},
			"/redirect?to=https://example.com/": {http.StatusSeeOther, subpath + "/"},
		} {
			status, header, body := newSubpathClient(t).doRaw(http.MethodGet, path, "", nil)
			assert.Equal(t, want.status, status, "%s: %s", path, body)
			assert.Equal(t, want.location, header.Get("Location"), path)
		}
	})

	t.Run("API v1", func(t *testing.T) {
		c := apiV1Client(t, owner)
		c.baseURL = subpathServerURL
		var got struct {
			HTMLURL  string `json:"html_url"`
			CloneURL string `json:"clone_url"`
		}
		c.doJSON(http.MethodGet, apiV1RepoPath(repo, ""), nil, http.StatusOK, &got)
		assert.Equal(t, subpathServerURL+"/"+repo.Owner+"/"+repo.Name, got.HTMLURL)
		assert.Equal(t, subpathServerURL+"/"+repo.Owner+"/"+repo.Name+".git", got.CloneURL)
	})

	t.Run("LFS", func(t *testing.T) {
		content, oid := lfsObject()
		c := lfsClient(t, owner)
		c.baseURL = subpathServerURL
		upload := lfsBatch(t, c, repo, "upload", oid, len(content)).Objects[0].Actions.Upload
		require.NotNil(t, upload)
		require.Equal(t, subpathServerURL+lfsBasePath(repo)+"/basic/"+oid, upload.Href)

		status, _, body := c.doRaw(http.MethodPut, strings.TrimPrefix(upload.Href, subpathServerURL), "application/octet-stream", strings.NewReader(string(content)))
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		download := lfsBatch(t, c, repo, "download", oid, len(content)).Objects[0].Actions.Download
		require.NotNil(t, download)
		assert.Equal(t, subpathServerURL+lfsBasePath(repo)+"/basic/"+oid, download.Href)
	})

	t.Run("Git over HTTP", func(t *testing.T) {
		u, err := url.Parse(subpathServerURL + "/" + repo.Owner + "/" + repo.Name + ".git")
		require.NoError(t, err)
		dir := gitClone(t, u.String())
		assert.Equal(t, gitMainOnDisk(t, repo), gitRun(t, dir, "rev-parse", "HEAD"))

		u.User = url.UserPassword(owner.Name, owner.Password)
		gitRun(t, dir, "remote", "set-url", "origin", u.String())
		want := gitCommitFile(t, dir, "pushed.txt")
		gitRun(t, dir, "push", "origin", "main")
		assert.Equal(t, want, gitMainOnDisk(t, repo))
	})
}
