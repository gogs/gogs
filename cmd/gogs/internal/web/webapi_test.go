package web

import (
	stdctx "context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func TestRequireViewAccess(t *testing.T) {
	beforeSigninView, beforeConfirmation := conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation
	t.Cleanup(func() {
		conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation = beforeSigninView, beforeConfirmation
	})

	handler, err := newRoutingHandler()
	require.NoError(t, err)

	// The gate runs before any database or Git access, so these requests are
	// answered without either being set up.
	paths := []string{
		"/alice/repo/raw/main/README.md",
		"/alice/repo/commit/0123456789abcdef0123456789abcdef01234567.diff",
		"/api/web/alice/repo/header",
		"/api/web/alice/repo/commit/0123456789abcdef0123456789abcdef01234567",
		"/api/web/explore/repos",
		"/api/web/explore/users",
		"/api/web/explore/organizations",
		"/api/web/users/alice",
		"/api/web/users/alice/repos",
		"/api/web/users/alice/activity",
		"/api/web/users/alice/followers",
		"/api/web/users/alice/following",
		"/api/web/alice/repo/stargazers",
		"/api/web/alice/repo/watchers",
	}
	tests := []struct {
		name                     string
		requireSigninView        bool
		requireEmailConfirmation bool
		viewer                   *database.User
		wantCode                 int
		wantBody                 string
	}{
		{
			name:              "anonymous when sign-in is required",
			requireSigninView: true,
			wantCode:          http.StatusUnauthorized,
			wantBody:          `{"error":"authentication required"}`,
		},
		{
			name:                     "unconfirmed user when confirmation is required",
			requireEmailConfirmation: true,
			viewer:                   &database.User{ID: 2, IsActive: false},
			wantCode:                 http.StatusForbidden,
			wantBody:                 `{"error":"account not activated"}`,
		},
		{
			name:     "login prohibited user when sign-in is not required",
			viewer:   &database.User{ID: 2, IsActive: true, ProhibitLogin: true},
			wantCode: http.StatusForbidden,
			wantBody: `{"error":"login prohibited"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf.Auth.RequireSigninView = test.requireSigninView
			conf.Auth.RequireEmailConfirmation = test.requireEmailConfirmation
			for _, path := range paths {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if test.viewer != nil {
					req = req.WithContext(stdctx.WithValue(req.Context(), context.UserKey{}, test.viewer))
				}
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				assert.Equal(t, test.wantCode, resp.Code, path)
				assert.JSONEq(t, test.wantBody, resp.Body.String(), path)
			}
		})
	}
}

func TestRequireSignIn(t *testing.T) {
	beforeSigninView, beforeConfirmation := conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation
	t.Cleanup(func() {
		conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation = beforeSigninView, beforeConfirmation
	})
	// Dashboards, user settings, the people and teams of organizations, and the
	// admin panel need a signed-in user even on sites that do not require signing in to view
	// content.
	conf.Auth.RequireSigninView = false

	handler, err := newRoutingHandler()
	require.NoError(t, err)

	paths := []string{
		"/api/web/dashboard",
		"/api/web/dashboard/activity",
		"/api/web/dashboard/issues",
		"/api/web/dashboard/pulls",
		"/api/web/orgs/acme/dashboard",
		"/api/web/orgs/acme/dashboard/activity",
		"/api/web/orgs/acme/dashboard/issues",
		"/api/web/orgs/acme/dashboard/pulls",
		"/api/web/orgs/acme/settings",
		"/api/web/user/settings",
		"/api/web/user/settings/repositories",
		"/api/web/user/settings/organizations",
		"/api/web/user/settings/emails",
		"/api/web/user/settings/ssh-keys",
		"/api/web/user/settings/two-factor",
		"/api/web/user/settings/tokens",
		"/api/web/orgs/acme/members",
		"/api/web/orgs/acme/teams",
		"/api/web/orgs/acme/teams/owners",
		"/api/web/orgs/acme/teams/owners/members",
		"/api/web/orgs/acme/teams/owners/repos",
		"/api/web/admin/dashboard",
		"/api/web/admin/config",
		"/api/web/admin/monitor",
		"/api/web/admin/auths",
		"/api/web/admin/auths/1",
		"/api/web/admin/users",
		"/api/web/admin/users/1",
		"/api/web/admin/user-options",
		"/api/web/admin/orgs",
		"/api/web/admin/repos",
		"/api/web/admin/notices",
	}
	tests := []struct {
		name                     string
		requireEmailConfirmation bool
		user                     *database.User
		wantCode                 int
		wantBody                 string
	}{
		{
			name:     "anonymous",
			wantCode: http.StatusUnauthorized,
			wantBody: `{"error":"authentication required"}`,
		},
		{
			name:                     "unconfirmed user when confirmation is required",
			requireEmailConfirmation: true,
			user:                     &database.User{ID: 2, IsActive: false},
			wantCode:                 http.StatusForbidden,
			wantBody:                 `{"error":"account not activated"}`,
		},
		{
			name:     "login prohibited user",
			user:     &database.User{ID: 2, IsActive: true, ProhibitLogin: true},
			wantCode: http.StatusForbidden,
			wantBody: `{"error":"login prohibited"}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf.Auth.RequireEmailConfirmation = test.requireEmailConfirmation
			for _, path := range paths {
				req := httptest.NewRequest(http.MethodGet, path, nil)
				if test.user != nil {
					req = req.WithContext(stdctx.WithValue(req.Context(), context.UserKey{}, test.user))
				}
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				assert.Equal(t, test.wantCode, resp.Code, path)
				assert.JSONEq(t, test.wantBody, resp.Body.String(), path)
			}
		})
	}
}

func TestRequireAdmin(t *testing.T) {
	handler, err := newRoutingHandler()
	require.NoError(t, err)

	// A signed-in user who is not a site admin reaches none of the admin panel,
	// including the actions that delete data.
	requests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/web/admin/dashboard"},
		{http.MethodPost, "/api/web/admin/operations/delete-inactive-users"},
		{http.MethodGet, "/api/web/admin/config"},
		{http.MethodPost, "/api/web/admin/config/test-mail"},
		{http.MethodGet, "/api/web/admin/monitor"},
		{http.MethodGet, "/api/web/admin/auths"},
		{http.MethodPost, "/api/web/admin/auths"},
		{http.MethodGet, "/api/web/admin/auths/1"},
		{http.MethodPatch, "/api/web/admin/auths/1"},
		{http.MethodDelete, "/api/web/admin/auths/1"},
		{http.MethodGet, "/api/web/admin/users"},
		{http.MethodPost, "/api/web/admin/users"},
		{http.MethodGet, "/api/web/admin/users/1"},
		{http.MethodPatch, "/api/web/admin/users/1"},
		{http.MethodDelete, "/api/web/admin/users/1"},
		{http.MethodGet, "/api/web/admin/user-options"},
		{http.MethodGet, "/api/web/admin/orgs"},
		{http.MethodGet, "/api/web/admin/repos"},
		{http.MethodDelete, "/api/web/admin/repos/1"},
		{http.MethodGet, "/api/web/admin/notices"},
		{http.MethodDelete, "/api/web/admin/notices"},
		{http.MethodDelete, "/api/web/admin/notices/all"},
	}
	user := &database.User{ID: 2, IsActive: true}
	for _, r := range requests {
		req := httptest.NewRequest(r.method, r.path, strings.NewReader(`{"ids":[1]}`))
		req = req.WithContext(stdctx.WithValue(req.Context(), context.UserKey{}, user))
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusForbidden, resp.Code, r.method+" "+r.path)
		assert.JSONEq(t, `{"error":"admin required"}`, resp.Body.String(), r.method+" "+r.path)
	}
}
