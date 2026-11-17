package integrationtest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
				Href string `json:"href"`
			} `json:"download"`
			Upload *struct {
				Href   string            `json:"href"`
				Header map[string]string `json:"header"`
			} `json:"upload"`
			Verify *struct {
				Href string `json:"href"`
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
		status, header, body := c.doRaw(http.MethodGet, lfsBasePath(public)+"/basic/"+oid, "", nil)
		assert.Equal(t, http.StatusUnauthorized, status, "response: %s", body)
		assert.Equal(t, `Basic realm="Git LFS"`, header.Get("Lfs-Authenticate"))
		assert.NotEqual(t, content, body)
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

		// Anyone signed in reads objects of a public repository.
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
