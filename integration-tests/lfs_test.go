package integrationtests

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/database"
)

const lfsMediaType = "application/vnd.git-lfs+json"

// lfsClient returns a client that speaks the LFS protocol as the user, or
// anonymously when the user is nil.
func lfsClient(t *testing.T, u *User) *client {
	t.Helper()

	c := newClient(t)
	c.header.Set("Accept", lfsMediaType)
	if u != nil {
		c.header.Set("Authorization", "Basic "+gitBasicAuth(u.Name, u.Password))
	}
	return c
}

// lfsObject returns unique content and its OID.
func lfsObject() ([]byte, string) {
	content := []byte(UniqueName("lfs-content-") + "\n")
	sum := sha256.Sum256(content)
	return content, hex.EncodeToString(sum[:])
}

func lfsBasePath(repo *Repository) string {
	return "/" + repo.Owner + "/" + repo.Name + ".git/info/lfs/objects"
}

// lfsPostJSON sends the body as LFS JSON and returns the status and the body
// of the response, after checking the content type of the response when the
// request is not rejected before the handler.
func lfsPostJSON(t *testing.T, c *client, path string, body any) (int, []byte) {
	t.Helper()

	b, err := json.Marshal(body)
	require.NoError(t, err)
	status, header, respBody := c.doRaw(http.MethodPost, path, lfsMediaType, bytes.NewReader(b))
	if len(respBody) > 0 {
		assert.Equal(t, lfsMediaType, header.Get("Content-Type"))
	}
	return status, respBody
}

type lfsBatchResponse struct {
	Transfer string `json:"transfer"`
	Objects  []struct {
		Oid     string `json:"oid"`
		Size    int64  `json:"size"`
		Actions struct {
			Download *struct {
				Href   string            `json:"href"`
				Header map[string]string `json:"header"`
			} `json:"download"`
			Upload *struct {
				Href   string            `json:"href"`
				Header map[string]string `json:"header"`
			} `json:"upload"`
			Verify *struct {
				Href   string            `json:"href"`
				Header map[string]string `json:"header"`
			} `json:"verify"`
			Error *struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"actions"`
	} `json:"objects"`
}

func lfsBatch(t *testing.T, c *client, repo *Repository, operation, oid string, size int) lfsBatchResponse {
	t.Helper()

	status, body := lfsPostJSON(t, c, lfsBasePath(repo)+"/batch", map[string]any{
		"operation": operation,
		"objects":   []map[string]any{{"oid": oid, "size": size}},
	})
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	var resp lfsBatchResponse
	require.NoError(t, json.Unmarshal(body, &resp), "response: %s", body)
	assert.Equal(t, "basic", resp.Transfer)
	require.Len(t, resp.Objects, 1)
	assert.Equal(t, oid, resp.Objects[0].Oid)
	assert.Equal(t, int64(size), resp.Objects[0].Size)
	return resp
}

func TestLFSUploadDownloadVerify(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := lfsClient(t, owner)
	content, oid := lfsObject()
	objectHref := serverURL + lfsBasePath(repo) + "/basic/" + oid

	upload := lfsBatch(t, c, repo, "upload", oid, len(content)).Objects[0].Actions
	require.NotNil(t, upload.Upload)
	assert.Equal(t, objectHref, upload.Upload.Href)
	assert.Equal(t, map[string]string{"Content-Type": "application/octet-stream"}, upload.Upload.Header)
	require.NotNil(t, upload.Verify)
	assert.Equal(t, serverURL+lfsBasePath(repo)+"/basic/verify", upload.Verify.Href)

	// The object is not there before it is uploaded.
	missing := lfsBatch(t, c, repo, "download", oid, len(content)).Objects[0].Actions
	assert.Nil(t, missing.Download)
	require.NotNil(t, missing.Error)
	assert.Equal(t, http.StatusNotFound, missing.Error.Code)
	status, _, body := c.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	status, body = lfsPostJSON(t, c, lfsBasePath(repo)+"/basic/verify", map[string]any{"oid": oid, "size": len(content)})
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, _, body = c.doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+oid, "application/octet-stream", bytes.NewReader(content))
	require.Equal(t, http.StatusOK, status, "response: %s", body)

	t.Run("download", func(t *testing.T) {
		status, header, body := c.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, "application/octet-stream", header.Get("Content-Type"))
		assert.Equal(t, content, body)
	})

	t.Run("verify", func(t *testing.T) {
		status, body := lfsPostJSON(t, c, lfsBasePath(repo)+"/basic/verify", map[string]any{"oid": oid, "size": len(content)})
		assert.Equal(t, http.StatusOK, status, "response: %s", body)

		status, body = lfsPostJSON(t, c, lfsBasePath(repo)+"/basic/verify", map[string]any{"oid": oid, "size": len(content) + 1})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), "Object size mismatch")
	})

	t.Run("batch download", func(t *testing.T) {
		existing := lfsBatch(t, c, repo, "download", oid, len(content)).Objects[0].Actions
		require.NotNil(t, existing.Download)
		assert.Equal(t, objectHref, existing.Download.Href)
		assert.Nil(t, existing.Error)

		mismatch := lfsBatch(t, c, repo, "download", oid, len(content)+1).Objects[0].Actions
		assert.Nil(t, mismatch.Download)
		require.NotNil(t, mismatch.Error)
		assert.Equal(t, http.StatusUnprocessableEntity, mismatch.Error.Code)
	})

	t.Run("other repository", func(t *testing.T) {
		otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
		status, _, body := c.doRaw(http.MethodGet, lfsBasePath(otherRepo)+"/basic/"+oid, "", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})
}

func TestLFSUploadRejectsMismatchedContent(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := lfsClient(t, owner)
	_, oid := lfsObject()

	status, _, body := c.doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+oid, "application/octet-stream", bytes.NewReader([]byte("other content")))
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)

	status, _, body = c.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
	assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

	status, _, body = c.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/not-an-oid", "", nil)
	assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
	assert.Contains(t, string(body), "Invalid oid")
}

func TestLFSAccess(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	public := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	private := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	content, oid := lfsObject()

	ownerClient := lfsClient(t, owner)
	for _, repo := range []*Repository{public, private} {
		status, _, body := ownerClient.doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+oid, "application/octet-stream", bytes.NewReader(content))
		require.Equal(t, http.StatusOK, status, "response: %s", body)
	}

	t.Run("anonymous", func(t *testing.T) {
		c := lfsClient(t, nil)

		// Anyone reads objects of a public repository without signing in.
		status, _, body := c.doRaw(http.MethodGet, lfsBasePath(public)+"/basic/"+oid, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)
		download := lfsBatch(t, c, public, "download", oid, len(content)).Objects[0].Actions
		require.NotNil(t, download.Download)
		assert.Equal(t, serverURL+lfsBasePath(public)+"/basic/"+oid, download.Download.Href)

		// Anything else asks for credentials, so the Git LFS client prompts for
		// them instead of giving up.
		assertAskedCredentials := func(t *testing.T, status int, header http.Header, body []byte) {
			t.Helper()

			assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
			assert.Equal(t, `Basic realm="Git LFS"`, header.Get("Lfs-Authenticate"))
			assert.NotEqual(t, content, body)
		}
		status, header, body := c.doRaw(http.MethodGet, lfsBasePath(private)+"/basic/"+oid, "", nil)
		assertAskedCredentials(t, status, header, body)
		status, header, body = c.doRaw(http.MethodPost, lfsBasePath(private)+"/batch", lfsMediaType,
			bytes.NewReader([]byte(`{"operation":"download","objects":[{"oid":"`+oid+`","size":1}]}`)))
		assertAskedCredentials(t, status, header, body)

		otherContent, otherOID := lfsObject()
		status, header, body = c.doRaw(http.MethodPut, lfsBasePath(public)+"/basic/"+otherOID, "application/octet-stream", bytes.NewReader(otherContent))
		assertAskedCredentials(t, status, header, body)
		status, header, body = c.doRaw(http.MethodPost, lfsBasePath(public)+"/basic/verify", lfsMediaType,
			bytes.NewReader([]byte(`{"oid":"`+oid+`","size":1}`)))
		assertAskedCredentials(t, status, header, body)

		missing := lfsBatch(t, ownerClient, public, "download", otherOID, len(otherContent)).Objects[0].Actions
		require.NotNil(t, missing.Error, "object uploaded without signing in")
		assert.Equal(t, http.StatusNotFound, missing.Error.Code)

		// A repository that does not exist is not found, whether or not the user
		// signs in.
		status, _, body = c.doRaw(http.MethodGet, "/"+owner.Name+"/"+UniqueName("missing-")+".git/info/lfs/objects/basic/"+oid, "", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("token", func(t *testing.T) {
		token, err := CreateAccessToken(t.Context(), owner.ID)
		require.NoError(t, err)
		c := lfsClient(t, nil)
		c.header.Set("Authorization", "Basic "+gitBasicAuth(token, ""))
		status, _, body := c.doRaw(http.MethodGet, lfsBasePath(private)+"/basic/"+oid, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)
	})

	t.Run("user without access", func(t *testing.T) {
		c := lfsClient(t, other)

		// Any authenticated user reads objects of a public repository.
		status, _, body := c.doRaw(http.MethodGet, lfsBasePath(public)+"/basic/"+oid, "", nil)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)

		status, _, body = c.doRaw(http.MethodGet, lfsBasePath(private)+"/basic/"+oid, "", nil)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = lfsPostJSON(t, c, lfsBasePath(private)+"/batch", map[string]any{
			"operation": "download",
			"objects":   []map[string]any{{"oid": oid, "size": len(content)}},
		})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		otherContent, otherOID := lfsObject()
		status, _, body = c.doRaw(http.MethodPut, lfsBasePath(public)+"/basic/"+otherOID, "application/octet-stream", bytes.NewReader(otherContent))
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
		status, body = lfsPostJSON(t, c, lfsBasePath(public)+"/basic/verify", map[string]any{"oid": oid, "size": len(content)})
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)

		missing := lfsBatch(t, ownerClient, public, "download", otherOID, len(otherContent)).Objects[0].Actions
		require.NotNil(t, missing.Error, "object uploaded without write access")
		assert.Equal(t, http.StatusNotFound, missing.Error.Code)
	})

	t.Run("wrong headers", func(t *testing.T) {
		c := newClient(t)
		c.header.Set("Authorization", "Basic "+gitBasicAuth(owner.Name, owner.Password))
		status, _, body := c.doRaw(http.MethodPost, lfsBasePath(public)+"/batch", lfsMediaType, bytes.NewReader([]byte(`{"operation":"download","objects":[]}`)))
		assert.Equal(t, http.StatusNotAcceptable, status, "response: %s", body)

		status, body = lfsPostJSON(t, ownerClient, lfsBasePath(public)+"/batch", map[string]any{"operation": "delete"})
		assert.Equal(t, http.StatusBadRequest, status, "response: %s", body)
		assert.Contains(t, string(body), "Operation not recognized")
	})
}

// TestLFSGitClient pushes and pulls an LFS object with the Git LFS client, so
// the batch, upload, verify and download requests are the ones the client
// sends.
func TestLFSGitClient(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	other := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})

	dir := gitClone(t, gitRemoteURL(t, repo, owner.Name, owner.Password))
	gitRun(t, dir, "lfs", "install", "--local")
	gitRun(t, dir, "lfs", "track", "*.bin")
	content := make([]byte, 200<<10)
	_, err := rand.Read(content)
	require.NoError(t, err)
	sum := sha256.Sum256(content)
	oid := hex.EncodeToString(sum[:])
	require.NoError(t, os.WriteFile(filepath.Join(dir, "large.bin"), content, 0o600))
	gitRun(t, dir, "add", ".gitattributes", "large.bin")
	gitRun(t, dir, "commit", "-m", "Add large.bin")
	gitRun(t, dir, "push", "origin", "main")

	// The repository stores a pointer and the server stores the object.
	pointer := gitRun(t, t.TempDir(), "--git-dir", database.RepoPath(repo.Owner, repo.Name), "show", "main:large.bin")
	assert.Contains(t, pointer, "oid sha256:"+oid)
	status, _, body := lfsClient(t, owner).doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
	require.Equal(t, http.StatusOK, status)
	assert.Equal(t, content, body)

	// Another user pulls the object into a new clone.
	clone := gitClone(t, gitRemoteURL(t, repo, other.Name, other.Password))
	gitRun(t, clone, "lfs", "install", "--local")
	gitRun(t, clone, "lfs", "pull")
	got, err := os.ReadFile(filepath.Join(clone, "large.bin"))
	require.NoError(t, err)
	assert.Equal(t, content, got)

	t.Run("anonymous clone", func(t *testing.T) {
		// The LFS filter checks out the object during the clone, as with Git LFS
		// installed globally.
		dir := filepath.Join(t.TempDir(), "clone")
		gitRun(t, filepath.Dir(dir), "clone",
			"--config", "filter.lfs.process=git-lfs filter-process",
			"--config", "filter.lfs.required=true",
			gitRemoteURL(t, repo, "", ""), dir,
		)
		got, err := os.ReadFile(filepath.Join(dir, "large.bin"))
		require.NoError(t, err)
		assert.Equal(t, content, got)

		gitRun(t, dir, "lfs", "install", "--local")
		require.NoError(t, os.WriteFile(filepath.Join(dir, "anonymous.bin"), []byte(UniqueName("lfs-content-")), 0o600))
		gitRun(t, dir, "add", "anonymous.bin")
		gitRun(t, dir, "commit", "-m", "Add anonymous.bin")
		out, err := gitCommand(dir, "push", "origin", "main").CombinedOutput()
		require.Error(t, err, "push succeeded: %s", out)
		assert.NotEqual(t, gitRun(t, dir, "rev-parse", "HEAD"), gitMainOnDisk(t, repo))
	})

	t.Run("user without write access", func(t *testing.T) {
		gitRun(t, clone, "lfs", "track", "*.dat")
		require.NoError(t, os.WriteFile(filepath.Join(clone, "other.dat"), []byte(UniqueName("lfs-content-")), 0o600))
		gitRun(t, clone, "add", ".gitattributes", "other.dat")
		gitRun(t, clone, "commit", "-m", "Add other.dat")
		out, err := gitCommand(clone, "push", "origin", "main").CombinedOutput()
		require.Error(t, err, "push succeeded: %s", out)
		assert.NotEqual(t, gitRun(t, clone, "rev-parse", "HEAD"), gitMainOnDisk(t, repo))
	})
}

// TestLFSGitClientOverSSH pushes and pulls LFS objects with the Git LFS client
// over the builtin SSH server, which it authenticates with through
// git-lfs-authenticate.
func TestLFSGitClientOverSSH(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	reader := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	repoSettingsAddCollaborator(t, repo, reader, database.AccessModeRead)
	ownerKey := sshUserKey(t, owner.ID)
	ownerClient := lfsClient(t, owner)

	dir := ownerKey.clone(t, repo)
	gitRun(t, dir, "lfs", "install", "--local")
	gitRun(t, dir, "lfs", "track", "*.bin")
	gitRun(t, dir, "add", ".gitattributes")
	gitRun(t, dir, "commit", "-m", "Track *.bin")

	// Git LFS sends the path of each form of remote differently, e.g., with or
	// without a leading slash.
	path := repo.Owner + "/" + repo.Name + ".git"
	remotes := []string{sshRemoteURL(repo), "git@127.0.0.1:/" + path, "git@127.0.0.1:" + path}
	contents := make(map[string][]byte, len(remotes))
	for i, remote := range remotes {
		content, oid := lfsObject()
		name := fmt.Sprintf("large-%d.bin", i)
		contents[name] = content
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), content, 0o600))
		gitRun(t, dir, "add", name)
		gitRun(t, dir, "commit", "-m", "Add "+name)
		out, err := ownerKey.git(dir, "push", remote, "main")
		require.NoError(t, err, "push to %s: %s", remote, out)

		status, _, body := ownerClient.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
		require.Equal(t, http.StatusOK, status, "object pushed to %s not stored: %s", remote, body)
		assert.Equal(t, content, body)
	}

	// cloneWithLFS clones the repository with the key, with the LFS filter that
	// checks out the objects during the clone as with Git LFS installed
	// globally, and checks their content.
	cloneWithLFS := func(t *testing.T, key *sshKey) string {
		t.Helper()

		dir := filepath.Join(t.TempDir(), "clone")
		out, err := key.git(filepath.Dir(dir), "clone",
			"--config", "filter.lfs.process=git-lfs filter-process",
			"--config", "filter.lfs.required=true",
			sshRemoteURL(repo), dir,
		)
		require.NoError(t, err, "clone: %s", out)
		for name, content := range contents {
			got, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			assert.Equal(t, content, got, name)
		}
		return dir
	}

	t.Run("read collaborator", func(t *testing.T) {
		key := sshUserKey(t, reader.ID)
		dir := cloneWithLFS(t, key)

		gitRun(t, dir, "lfs", "install", "--local")
		content, oid := lfsObject()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "reader.bin"), content, 0o600))
		gitRun(t, dir, "add", "reader.bin")
		gitRun(t, dir, "commit", "-m", "Add reader.bin")
		out, err := key.git(dir, "push", "origin", "main")
		require.Error(t, err, "push succeeded: %s", out)

		status, _, body := ownerClient.doRaw(http.MethodGet, lfsBasePath(repo)+"/basic/"+oid, "", nil)
		assert.Equal(t, http.StatusNotFound, status, "object stored without write access: %s", body)
	})

	t.Run("deploy key", func(t *testing.T) {
		cloneWithLFS(t, sshDeployKey(t, repo))
	})
}

// lfsSSHCredentials is what git-lfs-authenticate writes.
type lfsSSHCredentials struct {
	Href      string            `json:"href"`
	Header    map[string]string `json:"header"`
	ExpiresIn int               `json:"expires_in"`
}

// lfsSSHAuthenticate runs git-lfs-authenticate for the repository over SSH
// with the key, as Git LFS does, and returns its output and what it wrote to
// standard error.
func lfsSSHAuthenticate(key *sshKey, repo *Repository, operation string) ([]byte, string, error) {
	args := append(key.sshArgs(), "git@127.0.0.1", "git-lfs-authenticate "+repo.Owner+"/"+repo.Name+".git "+operation)
	cmd := exec.Command("ssh", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return out, stderr.String(), err
}

// lfsSSHToken returns the bearer token in the output of git-lfs-authenticate
// for the repository, after checking the rest of the output.
func lfsSSHToken(t *testing.T, repo *Repository, out []byte) string {
	t.Helper()

	var creds lfsSSHCredentials
	require.NoError(t, json.Unmarshal(out, &creds), "output: %s", out)
	assert.Equal(t, serverURL+"/"+repo.Owner+"/"+repo.Name+".git/info/lfs", creds.Href)
	assert.Equal(t, 3600, creds.ExpiresIn)
	token, ok := strings.CutPrefix(creds.Header["Authorization"], "Bearer ")
	require.True(t, ok, "header: %v", creds.Header)
	return token
}

// lfsSSHKeyToken returns the bearer token that git-lfs-authenticate gives for
// the key and the operation on the repository.
func lfsSSHKeyToken(t *testing.T, key *sshKey, repo *Repository, operation string) string {
	t.Helper()

	out, stderr, err := lfsSSHAuthenticate(key, repo, operation)
	require.NoError(t, err, "git-lfs-authenticate: %s", stderr)
	return lfsSSHToken(t, repo, out)
}

// lfsTokenClient returns a client that speaks the LFS protocol with the
// bearer token.
func lfsTokenClient(t *testing.T, token string) *client {
	t.Helper()

	c := lfsClient(t, nil)
	c.header.Set("Authorization", "Bearer "+token)
	return c
}

func TestLFSSSHAuthenticate(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	otherRepo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true, Private: true})
	ownerKey := sshUserKey(t, owner.ID)
	content, oid := lfsObject()
	for _, r := range []*Repository{repo, otherRepo} {
		status, _, body := lfsClient(t, owner).doRaw(http.MethodPut, lfsBasePath(r)+"/basic/"+oid, "application/octet-stream", bytes.NewReader(content))
		require.Equal(t, http.StatusOK, status, "response: %s", body)
	}
	get := func(c *client, r *Repository) (int, []byte) {
		status, _, body := c.doRaw(http.MethodGet, lfsBasePath(r)+"/basic/"+oid, "", nil)
		return status, body
	}

	t.Run("download", func(t *testing.T) {
		token := lfsSSHKeyToken(t, ownerKey, repo, "download")
		c := lfsTokenClient(t, token)

		status, body := get(c, repo)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)

		// Git LFS sends the token only to the batch API, so the actions repeat it.
		download := lfsBatch(t, c, repo, "download", oid, len(content)).Objects[0].Actions
		require.NotNil(t, download.Download)
		assert.Equal(t, map[string]string{"Authorization": "Bearer " + token}, download.Download.Header)

		otherContent, otherOID := lfsObject()
		status, _, body = c.doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+otherOID, "application/octet-stream", bytes.NewReader(otherContent))
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

		// The owner reads the other repository, but the token is for one only.
		status, body = get(c, otherRepo)
		assert.Equal(t, http.StatusForbidden, status, "response: %s", body)

		tampered := []byte(token)
		tampered[0] ^= 1
		status, body = get(lfsTokenClient(t, string(tampered)), repo)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
	})

	t.Run("upload", func(t *testing.T) {
		token := lfsSSHKeyToken(t, ownerKey, repo, "upload")
		c := lfsTokenClient(t, token)
		otherContent, otherOID := lfsObject()

		upload := lfsBatch(t, c, repo, "upload", otherOID, len(otherContent)).Objects[0].Actions
		require.NotNil(t, upload.Upload)
		assert.Equal(t, map[string]string{
			"Authorization": "Bearer " + token,
			"Content-Type":  "application/octet-stream",
		}, upload.Upload.Header)
		require.NotNil(t, upload.Verify)
		assert.Equal(t, map[string]string{"Authorization": "Bearer " + token}, upload.Verify.Header)

		status, _, body := c.doRaw(http.MethodPut, lfsBasePath(repo)+"/basic/"+otherOID, "application/octet-stream", bytes.NewReader(otherContent))
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		status, body = lfsPostJSON(t, c, lfsBasePath(repo)+"/basic/verify", map[string]any{"oid": otherOID, "size": len(otherContent)})
		assert.Equal(t, http.StatusOK, status, "response: %s", body)
	})

	t.Run("read collaborator", func(t *testing.T) {
		reader := seedUser(t, CreateUserOptions{})
		repoSettingsAddCollaborator(t, repo, reader, database.AccessModeRead)
		key := sshUserKey(t, reader.ID)

		_, stderr, err := lfsSSHAuthenticate(key, repo, "upload")
		require.Error(t, err)
		assert.Contains(t, stderr, "You do not have sufficient authorization for this action")

		c := lfsTokenClient(t, lfsSSHKeyToken(t, key, repo, "download"))
		status, body := get(c, repo)
		require.Equal(t, http.StatusOK, status, "response: %s", body)

		// The token stops working with the access it was given for.
		status, body = authenticatedClient(t, owner).do(http.MethodDelete, repoSettingsAPIPath(repo, "/settings/collaborators/"+reader.Name), repoSettingsNoBody)
		require.Equal(t, http.StatusNoContent, status, "response: %s", body)
		status, body = get(c, repo)
		assert.Equal(t, http.StatusNotFound, status, "response: %s", body)
	})

	t.Run("deploy key", func(t *testing.T) {
		key := sshDeployKey(t, repo)

		_, stderr, err := lfsSSHAuthenticate(key, repo, "upload")
		require.Error(t, err)
		assert.Contains(t, stderr, "Key permission denied")

		status, body := get(lfsTokenClient(t, lfsSSHKeyToken(t, key, repo, "download")), repo)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)
	})

	t.Run("unknown operation", func(t *testing.T) {
		_, stderr, err := lfsSSHAuthenticate(ownerKey, repo, "delete")
		require.Error(t, err)
		assert.Contains(t, stderr, "Unknown LFS operation")
	})

	t.Run("gogs serv", func(t *testing.T) {
		// OpenSSH runs "gogs serv" with the command the client sends.
		cmd := exec.Command(servProgram(t, ownerKey.id), "git@localhost", "git-lfs-authenticate "+repo.Owner+"/"+repo.Name+".git download")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		require.NoError(t, err, "gogs serv: %s", stderr.String())

		status, body := get(lfsTokenClient(t, lfsSSHToken(t, repo, out)), repo)
		require.Equal(t, http.StatusOK, status, "response: %s", body)
		assert.Equal(t, content, body)
	})
}
