package web

import (
	stdctx "context"
	"net/http"
	"net/http/httptest"
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
		"/api/web/dashboard",
		"/api/web/dashboard/activity",
		"/api/web/dashboard/issues",
		"/api/web/dashboard/pulls",
		"/api/web/orgs/acme/dashboard",
		"/api/web/orgs/acme/dashboard/activity",
		"/api/web/orgs/acme/dashboard/issues",
		"/api/web/orgs/acme/dashboard/pulls",
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

func TestRequireAuthenticated(t *testing.T) {
	beforeSigninView := conf.Auth.RequireSigninView
	t.Cleanup(func() {
		conf.Auth.RequireSigninView = beforeSigninView
	})
	// Dashboards, user settings, and the people and teams of organizations need
	// an authenticated viewer even on sites that do not require signing in to view
	// content.
	conf.Auth.RequireSigninView = false

	handler, err := newRoutingHandler()
	require.NoError(t, err)

	for _, path := range []string{
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
		"/api/web/orgs/acme/members",
		"/api/web/orgs/acme/teams",
		"/api/web/orgs/acme/teams/owners",
		"/api/web/orgs/acme/teams/owners/members",
		"/api/web/orgs/acme/teams/owners/repos",
	} {
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusUnauthorized, resp.Code, path)
		assert.JSONEq(t, `{"error":"authentication required"}`, resp.Body.String(), path)
	}
}
