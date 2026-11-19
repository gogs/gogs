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
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

// client calls the server under test like a browser that keeps its cookies.
type client struct {
	t *testing.T
	// baseURL is the URL of the server the client calls, which is serverURL
	// unless a test calls another server.
	baseURL string
	http    *http.Client
	// header is sent with every request, e.g., an access token.
	header http.Header
}

func newClient(t *testing.T) *client {
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	return &client{
		t:       t,
		baseURL: serverURL,
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

	req, err := http.NewRequestWithContext(context.Background(), method, c.baseURL+path, body)
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

	u, err := url.Parse(c.baseURL)
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

// username returns the name of the authenticated user, or an empty string when
// the client is unauthenticated.
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

// goAwayWhileLocked sends a request with the body encoded as JSON, and goes
// away while the request waits for the rows the lock query locks, e.g.,
// "SELECT id FROM issue WHERE id = ? FOR UPDATE". It then releases the rows,
// so that the server goes on with the request its client left.
func (c *client) goAwayWhileLocked(lockQuery string, lockArg any, method, path string, body any) {
	c.t.Helper()
	c.whileLocked(lockQuery, lockArg, method, path, body, func(_ *gorm.DB, goAway context.CancelFunc) {
		goAway()
		// Leave the server time to notice that the client went away.
		time.Sleep(500 * time.Millisecond)
	})
}

// lockedRequest is a request for doBothWhileLocked. Its body is sent as JSON.
type lockedRequest struct {
	method string
	path   string
	body   any
}

// doBothWhileLocked sends both requests while the lock query holds its rows,
// and releases them once both requests are blocked, so the two race. It returns
// both status codes.
func (c *client) doBothWhileLocked(lockQuery string, lockArg any, first, second lockedRequest) []int {
	c.t.Helper()

	var secondStatus int
	done := make(chan struct{})
	firstStatus, _ := c.whileLocked(lockQuery, lockArg, first.method, first.path, first.body, func(tx *gorm.DB, _ context.CancelFunc) {
		go func() {
			defer close(done)
			secondStatus, _ = c.do(second.method, second.path, second.body)
		}()
		// The second request may fail the test, so it must finish before the
		// test does.
		c.t.Cleanup(func() { <-done })

		// The second request may be blocked by the first one instead of by the
		// lock.
		var lockerPID int64
		require.NoError(c.t, tx.Raw("SELECT pg_backend_pid()").Scan(&lockerPID).Error)
		require.Eventually(c.t, func() bool {
			var blocked int64
			err := tx.Raw(`SELECT count(*) FROM pg_stat_activity a WHERE @pid = ANY(pg_blocking_pids(a.pid)) OR EXISTS (
	SELECT 1 FROM pg_stat_activity b WHERE @pid = ANY(pg_blocking_pids(b.pid)) AND b.pid = ANY(pg_blocking_pids(a.pid))
)`, map[string]any{"pid": lockerPID}).Scan(&blocked).Error
			return err == nil && blocked >= 2
		}, 30*time.Second, 50*time.Millisecond, "requests did not both wait")
	})
	<-done
	return []int{firstStatus, secondStatus}
}

// whileLocked sends a request with the body encoded as JSON while the rows the
// lock query locks are locked. Once the request waits for them, it calls
// whileWaiting with the locking transaction and the function that cancels the
// request, then commits the transaction. It returns the response, or a zero
// status when the request was cancelled.
func (c *client) whileLocked(lockQuery string, lockArg any, method, path string, body any, whileWaiting func(tx *gorm.DB, cancel context.CancelFunc)) (int, []byte) {
	c.t.Helper()

	db, err := dbx.OpenDB(conf.Database, &gorm.Config{})
	require.NoError(c.t, err)
	sqlDB, err := db.DB()
	require.NoError(c.t, err)
	defer func() { _ = sqlDB.Close() }()

	tx := db.Begin()
	require.NoError(c.t, tx.Error)
	defer tx.Rollback()
	var lockerPID int64
	require.NoError(c.t, tx.Raw("SELECT pg_backend_pid()").Scan(&lockerPID).Error)
	var locked []int64
	require.NoError(c.t, tx.Raw(lockQuery, lockArg).Scan(&locked).Error)
	require.NotEmpty(c.t, locked, "nothing locked")

	// Like the browser client, send no body rather than "null" for nil. The
	// server notices that the client went away only after it reads the body
	// to its end.
	reqBody := io.Reader(http.NoBody)
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(c.t, err)
		reqBody = bytes.NewReader(b)
	}
	ctx, cancel := context.WithCancel(c.t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reqBody)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	for k, v := range c.header {
		req.Header[k] = v
	}
	var status int
	var respBody []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		resp, err := c.http.Do(req)
		if err != nil {
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		if err == nil {
			status, respBody = resp.StatusCode, b
		}
	}()

	require.Eventually(c.t, func() bool {
		var blocked int64
		err := db.Raw("SELECT count(*) FROM pg_stat_activity WHERE ? = ANY(pg_blocking_pids(pid))", lockerPID).Scan(&blocked).Error
		return err == nil && blocked > 0
	}, 30*time.Second, 50*time.Millisecond, "request did not wait for the locked rows")
	whileWaiting(tx, cancel)
	require.NoError(c.t, tx.Commit().Error)
	<-done
	return status, respBody
}
