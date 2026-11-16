package web

import (
	stdctx "context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func TestRequireViewAccess(t *testing.T) {
	beforeSigninView, beforeConfirmation := conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation
	t.Cleanup(func() {
		conf.Auth.RequireSigninView, conf.Auth.RequireEmailConfirmation = beforeSigninView, beforeConfirmation
	})

	handler, err := newRoutingHandler(logx.NewNoopLogger())
	require.NoError(t, err)

	// The gate runs before any database or Git access, so these requests are
	// answered without either being set up.
	paths := []string{
		"/alice/repo/raw/main/README.md",
		"/alice/repo/commit/0123456789abcdef0123456789abcdef01234567.diff",
		"/alice/repo/compare/0123456789abcdef0123456789abcdef01234567...89abcdef0123456789abcdef0123456789abcdef.diff",
		"/alice/repo/pulls/1.diff",
		"/api/web/alice/repo/header",
		"/api/web/alice/repo/src",
		"/api/web/alice/repo/src/main/README.md",
		"/api/web/alice/repo/commit/0123456789abcdef0123456789abcdef01234567",
		"/api/web/alice/repo/compare/0123456789abcdef0123456789abcdef01234567...89abcdef0123456789abcdef0123456789abcdef",
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
		"/api/web/alice/repo/issues",
		"/api/web/alice/repo/issues/1",
		"/api/web/alice/repo/issues/new",
		"/api/web/alice/repo/pulls",
		"/api/web/alice/repo/pulls/1",
		"/api/web/alice/repo/pulls/1/commits",
		"/api/web/alice/repo/forks",
		"/api/web/alice/repo/releases",
		"/api/web/alice/repo/branches",
		"/api/web/alice/repo/commits/main/README.md",
		"/api/web/alice/repo/wiki",
		"/api/web/alice/repo/wiki/pages",
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
	// Dashboards, user settings, the people and teams of organizations, the
	// settings of repositories, and the admin panel need an authenticated viewer even on
	// sites that do not require signing in to view content.
	conf.Auth.RequireSigninView = false

	handler, err := newRoutingHandler(logx.NewNoopLogger())
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
		"/api/web/repo/new",
		"/api/web/repo/fork/1",
		"/api/web/acme/widgets/settings",
		"/api/web/acme/widgets/settings/collaborators",
		"/api/web/acme/widgets/settings/keys",
		"/api/web/acme/widgets/settings/hooks",
		"/api/web/acme/widgets/settings/hooks/1",
		"/api/web/orgs/acme/settings/hooks",
		"/api/web/orgs/acme/settings/hooks/1",
		"/api/web/acme/widgets/milestones/1",
		"/api/web/acme/widgets/wiki/source",
		"/api/web/acme/widgets/editor/master/README.md",
		"/api/web/acme/widgets/editor-indent",
		"/api/web/acme/widgets/suggestions/mentions",
		"/api/web/acme/widgets/suggestions/issues",
		"/api/web/acme/widgets/pulls/new/master...feature/login",
	} {
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		assert.Equal(t, http.StatusUnauthorized, resp.Code, path)
		assert.JSONEq(t, `{"error":"authentication required"}`, resp.Body.String(), path)
	}
}

func TestRequireAdmin(t *testing.T) {
	handler, err := newRoutingHandler(logx.NewNoopLogger())
	require.NoError(t, err)

	// An authenticated user who is not a site admin reaches none of the admin panel,
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
		req.Header.Set("Content-Type", "application/json")
		req = req.WithContext(stdctx.WithValue(req.Context(), context.UserKey{}, user))
		resp := httptest.NewRecorder()
		handler.ServeHTTP(resp, req)
		assert.Equal(t, http.StatusForbidden, resp.Code, r.method+" "+r.path)
		assert.JSONEq(t, `{"error":"admin required"}`, resp.Body.String(), r.method+" "+r.path)
	}
}

func TestRequireJSON(t *testing.T) {
	handler, err := newRoutingHandler(logx.NewNoopLogger())
	require.NoError(t, err)

	// Requests that pass the check reach the sign-in gate of each route, which
	// answers anonymous visitors without a database.
	paths := []string{
		"/api/web/user/settings/ssh-keys",
		"/api/web/admin/operations/delete-inactive-users",
		"/api/web/alice/repo/settings/collaborators",
		"/api/web/alice/repo/issues",
	}
	tests := []struct {
		name        string
		contentType string
		wantCode    int
	}{
		{name: "form posted as text/plain", contentType: "text/plain", wantCode: http.StatusUnsupportedMediaType},
		{name: "URL-encoded form", contentType: "application/x-www-form-urlencoded", wantCode: http.StatusUnsupportedMediaType},
		{name: "multipart form", contentType: "multipart/form-data; boundary=x", wantCode: http.StatusUnsupportedMediaType},
		{name: "no content type", wantCode: http.StatusUnsupportedMediaType},
		{name: "JSON with a charset", contentType: "application/json; charset=utf-8", wantCode: http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for _, path := range paths {
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"key":"x"}`))
				if tc.contentType != "" {
					req.Header.Set("Content-Type", tc.contentType)
				}
				resp := httptest.NewRecorder()
				handler.ServeHTTP(resp, req)
				assert.Equal(t, tc.wantCode, resp.Code, path)
				if tc.wantCode == http.StatusUnsupportedMediaType {
					// Nothing after the check writes to the response.
					assert.JSONEq(t, `{"error":"content type must be application/json"}`, resp.Body.String(), path)
				}
			}
		})
	}

	t.Run("file uploads stay forms", func(t *testing.T) {
		for _, path := range []string{
			"/api/web/alice/repo/issues/attachments",
			"/api/web/alice/repo/releases/attachments",
			"/api/web/alice/repo/editor/uploads",
		} {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("--x--\r\n"))
			req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
			resp := httptest.NewRecorder()
			handler.ServeHTTP(resp, req)
			assert.Equal(t, http.StatusUnauthorized, resp.Code, path)
		}
	})
}

func TestCrossOriginProtection(t *testing.T) {
	beforeURL := conf.Server.URL
	t.Cleanup(func() { conf.Server.URL = beforeURL })
	conf.Server.URL = &url.URL{Scheme: "https", Host: "git.example.com"}

	crossOrigin, err := newCrossOriginProtection()
	require.NoError(t, err)
	var reached bool
	m := macaron.New()
	m.Use(crossOrigin)
	m.Any("/*", func(w http.ResponseWriter) {
		reached = true
		w.WriteHeader(http.StatusNoContent)
	})

	tests := []struct {
		name     string
		method   string
		path     string
		header   map[string]string
		wantCode int
	}{
		{
			name:     "same-origin JSON request",
			method:   http.MethodPost,
			path:     "/api/web/alice/repo/settings/collaborators",
			header:   map[string]string{"Sec-Fetch-Site": "same-origin"},
			wantCode: http.StatusNoContent,
		},
		{
			name:     "form posted from another site",
			method:   http.MethodPost,
			path:     "/api/web/alice/repo/settings/collaborators",
			header:   map[string]string{"Sec-Fetch-Site": "cross-site", "Content-Type": "text/plain"},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "form posted from a sibling subdomain",
			method:   http.MethodPost,
			path:     "/api/web/user/settings/ssh-keys",
			header:   map[string]string{"Sec-Fetch-Site": "same-site"},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "legacy form post from another site",
			method:   http.MethodPost,
			path:     "/alice/repo/action/watch",
			header:   map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "older browser posting from another origin",
			method:   http.MethodPost,
			path:     "/api/web/admin/auths",
			header:   map[string]string{"Origin": "https://evil.example.com"},
			wantCode: http.StatusForbidden,
		},
		{
			name:     "older browser behind a proxy that rewrites Host",
			method:   http.MethodPost,
			path:     "/api/web/admin/auths",
			header:   map[string]string{"Origin": "https://git.example.com"},
			wantCode: http.StatusNoContent,
		},
		{
			name:     "cross-site page load",
			method:   http.MethodGet,
			path:     "/alice/repo",
			header:   map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantCode: http.StatusNoContent,
		},
		{
			name:     "API v1 called from an allowed origin",
			method:   http.MethodPost,
			path:     "/api/v1/user/repos",
			header:   map[string]string{"Sec-Fetch-Site": "cross-site"},
			wantCode: http.StatusNoContent,
		},
		{
			name:     "client that is not a browser",
			method:   http.MethodPost,
			path:     "/api/web/user/sign-in",
			wantCode: http.StatusNoContent,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reached = false
			req := httptest.NewRequest(tc.method, tc.path, nil)
			req.Host = "127.0.0.1:3000"
			for k, v := range tc.header {
				req.Header.Set(k, v)
			}
			resp := httptest.NewRecorder()
			m.ServeHTTP(resp, req)
			assert.Equal(t, tc.wantCode, resp.Code)
			assert.Equal(t, tc.wantCode != http.StatusForbidden, reached, "route reached")
		})
	}
}
