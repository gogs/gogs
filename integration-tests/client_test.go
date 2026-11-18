package integrationtests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

// client calls the server under test like a browser that keeps its cookies.
type client struct {
	t    *testing.T
	http *http.Client
	// header is sent with every request, e.g., an access token.
	header http.Header
}

func newClient(t *testing.T) *client {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &client{
		t: t,
		http: &http.Client{
			Jar: jar,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		header: make(http.Header),
	}
}

// do sends a request with the body encoded as JSON, unless it is nil, and
// returns the status code and the response body.
func (c *client) do(method, path string, body any) (int, []byte) {
	c.t.Helper()

	if body == nil {
		status, _, respBody := c.doRaw(method, path, "", nil)
		return status, respBody
	}
	b, err := json.Marshal(body)
	require.NoError(c.t, err)
	status, _, respBody := c.doRaw(method, path, "application/json", bytes.NewReader(b))
	return status, respBody
}

// doRaw sends a request with the body of the content type, unless it is nil,
// e.g., a multipart form, and returns the status code, the headers and the
// body of the response.
func (c *client) doRaw(method, path, contentType string, body io.Reader) (int, http.Header, []byte) {
	c.t.Helper()

	req, err := http.NewRequestWithContext(context.Background(), method, serverURL+path, body)
	require.NoError(c.t, err)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range c.header {
		req.Header[k] = v
	}

	resp, err := c.http.Do(req)
	require.NoError(c.t, err)
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(c.t, err)
	return resp.StatusCode, resp.Header, respBody
}

// doJSON is like do, and decodes the response body into v after checking the
// status code.
func (c *client) doJSON(method, path string, body any, wantStatus int, v any) {
	c.t.Helper()

	status, respBody := c.do(method, path, body)
	require.Equal(c.t, wantStatus, status, "response: %s", respBody)
	require.NoError(c.t, json.Unmarshal(respBody, v), "response: %s", respBody)
}

// sessionID returns the ID of the session the server keeps for this client.
func (c *client) sessionID() string {
	c.t.Helper()

	u, err := url.Parse(serverURL)
	require.NoError(c.t, err)
	for _, cookie := range c.http.Jar.Cookies(u) {
		if cookie.Name == conf.Session.CookieName {
			return cookie.Value
		}
	}
	c.t.Fatal("no session cookie")
	return ""
}

// sessionValue returns the value the server stores in the session of this
// client under the key.
func (c *client) sessionValue(key string) any {
	c.t.Helper()

	store, err := sessions.Read(c.t.Context(), c.sessionID())
	require.NoError(c.t, err)
	return store.Get(key)
}

// authenticatedClient returns a new client authenticated as the user, who must
// not have two-factor authentication enabled.
func authenticatedClient(t *testing.T, u *User) *client {
	t.Helper()

	c := newClient(t)
	c.signIn(u)
	return c
}

// signIn signs in as the user, who must not have two-factor authentication
// enabled.
func (c *client) signIn(u *User) {
	c.t.Helper()

	var resp struct {
		MFA bool `json:"mfa"`
	}
	c.doJSON(http.MethodPost, "/api/web/user/sign-in", map[string]any{
		"username": u.Name,
		"password": u.Password,
	}, http.StatusOK, &resp)
	require.False(c.t, resp.MFA)
}

// username returns the name of the signed-in user, or an empty string when the
// client is not signed in.
func (c *client) username() string {
	c.t.Helper()

	status, body := c.do(http.MethodGet, "/api/web/user/info", nil)
	if status == http.StatusNoContent {
		return ""
	}
	require.Equal(c.t, http.StatusOK, status, "response: %s", body)
	var info struct {
		Username string `json:"username"`
	}
	require.NoError(c.t, json.Unmarshal(body, &info))
	return info.Username
}

func seedUser(t *testing.T, opts CreateUserOptions) *User {
	t.Helper()

	u, err := CreateUser(t.Context(), opts)
	require.NoError(t, err)
	return u
}

func seedRepository(t *testing.T, ownerID int64, opts CreateRepositoryOptions) *Repository {
	t.Helper()

	repo, err := CreateRepository(t.Context(), logger, ownerID, opts)
	require.NoError(t, err)
	return repo
}
