package web

import (
	stdctx "context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	macaronsession "github.com/go-macaron/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

// memorySession is a session store that keeps its values in memory.
type memorySession struct {
	macaronsession.Store
	values map[any]any
}

func (s *memorySession) Set(k, v any) error { s.values[k] = v; return nil }
func (s *memorySession) Get(k any) any      { return s.values[k] }
func (s *memorySession) Delete(k any) error { delete(s.values, k); return nil }
func (s *memorySession) ID() string         { return "test" }

func TestRequireRecentAuth(t *testing.T) {
	handler, err := newRoutingHandler()
	require.NoError(t, err)

	// The gate runs before any database access on these routes.
	routes := []struct{ method, path string }{
		{http.MethodPost, "/api/web/user/settings/emails"},
		{http.MethodDelete, "/api/web/user/settings/emails/alice@example.com"},
		{http.MethodPut, "/api/web/user/settings/emails/alice@example.com/primary"},
		{http.MethodPost, "/api/web/user/settings/ssh-keys"},
		{http.MethodDelete, "/api/web/user/settings/ssh-keys/1"},
		{http.MethodPost, "/api/web/user/settings/two-factor"},
		{http.MethodDelete, "/api/web/user/settings/two-factor"},
		{http.MethodGet, "/api/web/user/settings/two-factor/recovery-codes"},
		{http.MethodPost, "/api/web/user/settings/two-factor/recovery-codes"},
		{http.MethodPost, "/api/web/user/settings/passkeys"},
		{http.MethodPost, "/api/web/user/settings/passkeys/registration"},
		{http.MethodDelete, "/api/web/user/settings/passkeys/1"},
		{http.MethodPost, "/api/web/user/settings/tokens"},
		{http.MethodDelete, "/api/web/user/settings/tokens/1"},
		{http.MethodPost, "/api/web/admin/users"},
		{http.MethodPost, "/api/web/admin/auths"},
	}
	for _, test := range []struct {
		name        string
		recentUntil any
	}{
		{name: "never confirmed"},
		{name: "confirmation expired", recentUntil: time.Now().Add(-time.Second).Unix()},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, route := range routes {
				sess := &memorySession{values: map[any]any{}}
				if test.recentUntil != nil {
					sess.values[recentAuthSessionKey] = test.recentUntil
				}
				req := httptest.NewRequest(route.method, route.path, strings.NewReader(`{}`))
				ctx := stdctx.WithValue(req.Context(), context.UserKey{}, &database.User{ID: 1, Name: "alice", IsActive: true, IsAdmin: true})
				ctx = stdctx.WithValue(ctx, webAPISessionKey{}, macaronsession.Store(sess))
				ctx = stdctx.WithValue(ctx, webAPIMacaronKey{}, &macaron.Context{})
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req.WithContext(ctx))
				assert.Equal(t, http.StatusForbidden, resp.Code, "%s %s", route.method, route.path)
				assert.Contains(t, resp.Body.String(), `"reauthRequired":true`, "%s %s", route.method, route.path)
			}
		})
	}
}

func TestRecentAuth(t *testing.T) {
	sess := flamegoSessionAdapter{sess: &memorySession{values: map[any]any{}}}
	assert.False(t, hasRecentAuth(sess))

	grantRecentAuth(sess)
	assert.True(t, hasRecentAuth(sess))

	sess.Set(recentAuthSessionKey, time.Now().Unix())
	assert.False(t, hasRecentAuth(sess), "a confirmation ends at its expiry time")
}
