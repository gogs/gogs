package integrationtests

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

// apiV1Client returns a client that authenticates as the user with a new access
// token.
func apiV1Client(t *testing.T, u *User) *client {
	t.Helper()

	token, err := CreateAccessToken(t.Context(), u.ID)
	require.NoError(t, err)
	c := newClient(t)
	c.header.Set("Authorization", "token "+token)
	return c
}

// apiV1BasicClient returns a client that authenticates with the username and
// the password.
func apiV1BasicClient(t *testing.T, username, password string) *client {
	t.Helper()

	c := newClient(t)
	c.header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	return c
}

// apiV1RepoPath returns the API v1 path of the repository followed by the
// suffix, e.g., "/issues".
func apiV1RepoPath(repo *Repository, suffix string) string {
	return "/api/v1/repos/" + repo.Owner + "/" + repo.Name + suffix
}

// apiV1PublicKey returns a new SSH public key in the authorized_keys format.
func apiV1PublicKey(t *testing.T) string {
	t.Helper()

	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	key, err := ssh.NewPublicKey(pub)
	require.NoError(t, err)
	return strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key)))
}

type apiV1User struct {
	ID       int64  `json:"id"`
	UserName string `json:"username"`
	Login    string `json:"login"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

type apiV1Repo struct {
	ID            int64      `json:"id"`
	Owner         apiV1User  `json:"owner"`
	Name          string     `json:"name"`
	FullName      string     `json:"full_name"`
	Description   string     `json:"description"`
	Private       bool       `json:"private"`
	Fork          bool       `json:"fork"`
	Parent        *apiV1Repo `json:"parent"`
	Empty         bool       `json:"empty"`
	Mirror        bool       `json:"mirror"`
	DefaultBranch string     `json:"default_branch"`
	Permissions   *struct {
		Admin bool `json:"admin"`
		Push  bool `json:"push"`
		Pull  bool `json:"pull"`
	} `json:"permissions"`
}

type apiV1Content struct {
	Type     string `json:"type"`
	Encoding string `json:"encoding"`
	Size     int64  `json:"size"`
	Name     string `json:"name"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Sha      string `json:"sha"`
}

// apiV1PutFile creates or updates the file at the path on the branch through
// the API, and returns the SHA of the commit made.
func apiV1PutFile(t *testing.T, c *client, repo *Repository, path, content, branch string) string {
	t.Helper()

	var resp struct {
		Content apiV1Content `json:"content"`
		Commit  struct {
			SHA string `json:"sha"`
		} `json:"commit"`
	}
	c.doJSON(http.MethodPut, apiV1RepoPath(repo, "/contents/"+path), map[string]any{
		"message": "Update " + path,
		"content": base64.StdEncoding.EncodeToString([]byte(content)),
		"branch":  branch,
	}, http.StatusCreated, &resp)
	require.NotEmpty(t, resp.Commit.SHA)
	return resp.Commit.SHA
}

// apiV1CreateIssue creates an issue through the API and returns its number.
func apiV1CreateIssue(t *testing.T, c *client, repo *Repository, body map[string]any) int64 {
	t.Helper()

	var issue struct {
		Number int64 `json:"number"`
	}
	c.doJSON(http.MethodPost, apiV1RepoPath(repo, "/issues"), body, http.StatusCreated, &issue)
	return issue.Number
}

func apiV1Itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func TestAPIV1Markdown(t *testing.T) {
	c := newClient(t)

	status, _, body := c.doRaw(http.MethodPost, "/api/v1/markdown", "application/json", strings.NewReader(`{"text": "Hello **world**"}`))
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Contains(t, string(body), "<strong>world</strong>")

	status, _, body = c.doRaw(http.MethodPost, "/api/v1/markdown", "text/plain", strings.NewReader("Hello"))
	assert.Equal(t, http.StatusUnsupportedMediaType, status, "response: %s", body)

	status, _, body = c.doRaw(http.MethodPost, "/api/v1/markdown/raw", "text/plain", strings.NewReader("# Title\n\n<script>alert(1)</script>"))
	require.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Contains(t, string(body), "Title</h1>")
	assert.NotContains(t, string(body), "<script>", "raw HTML not sanitized")
}

func TestAPIV1UnknownRoutes(t *testing.T) {
	c := newClient(t)

	status, body := c.do(http.MethodOptions, "/api/v1/"+UniqueName("preflight-"), nil)
	assert.Equal(t, http.StatusOK, status, "response: %s", body)
	assert.Empty(t, body)

	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		status, body := c.do(method, "/api/v1/"+UniqueName("unknown-")+"/path", nil)
		assert.Equal(t, http.StatusNotFound, status, "%s, response: %s", method, body)
	}
}
