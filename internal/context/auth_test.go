package context

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-macaron/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func TestIsRequestFromTrustedProxy(t *testing.T) {
	mustCIDR := func(s string) *net.IPNet {
		_, n, err := net.ParseCIDR(s)
		require.NoError(t, err)
		return n
	}

	original := conf.Auth.TrustedProxyCIDRs
	t.Cleanup(func() { conf.Auth.TrustedProxyCIDRs = original })
	conf.Auth.TrustedProxyCIDRs = []*net.IPNet{
		mustCIDR("127.0.0.0/8"),
		mustCIDR("::1/128"),
		mustCIDR("10.1.0.0/16"),
	}

	tests := []struct {
		name       string
		remoteAddr string
		want       bool
	}{
		{name: "loopback IPv4 with port", remoteAddr: "127.0.0.1:54321", want: true},
		{name: "loopback IPv6 with port", remoteAddr: "[::1]:54321", want: true},
		{name: "within configured CIDR", remoteAddr: "10.1.2.3:8080", want: true},
		{name: "outside configured CIDR", remoteAddr: "203.0.113.5:443", want: false},
		{name: "IPv4-mapped IPv6 matches IPv4 CIDR", remoteAddr: "[::ffff:127.0.0.1]:54321", want: true},
		{name: "remote without port", remoteAddr: "127.0.0.1", want: false},
		{name: "unparseable remote", remoteAddr: "not-an-ip", want: false},
		{name: "empty remote", remoteAddr: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{RemoteAddr: tc.remoteAddr}
			require.Equal(t, tc.want, isRequestFromTrustedProxy(req))
		})
	}
}

type tokenAuthStore struct {
	AuthStore
	token *database.AccessToken
}

func (s tokenAuthStore) GetAccessTokenBySHA1(_ context.Context, sha1 string) (*database.AccessToken, error) {
	if sha1 != s.token.Sha1 {
		return nil, database.ErrAccessTokenNotExist{}
	}
	return s.token, nil
}

func (tokenAuthStore) TouchAccessTokenByID(context.Context, int64) error { return nil }

func TestAuthenticatedUserIDAccessToken(t *testing.T) {
	store := tokenAuthStore{token: &database.AccessToken{ID: 1, UserID: 7, Sha1: "0123456789abcdef"}}

	tests := []struct {
		path          string
		wantUserID    int64
		wantTokenAuth bool
	}{
		{path: "/api/v1/user", wantUserID: 7, wantTokenAuth: true},
		{path: "/api/web/user/settings/tokens", wantUserID: 0, wantTokenAuth: false},
		{path: "/api/web/user/settings/two-factor/recovery-codes", wantUserID: 0, wantTokenAuth: false},
		{path: "/api/web/admin/auths", wantUserID: 0, wantTokenAuth: false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			m := macaron.New()
			m.Use(session.Sessioner())
			var gotUserID int64
			var gotTokenAuth bool
			m.Any("/*", func(c *macaron.Context, sess session.Store) {
				gotUserID, gotTokenAuth = authenticatedUserID(logx.NewNoopLogger(), store, c, sess)
			})
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "token "+store.token.Sha1)
			m.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.wantUserID, gotUserID)
			assert.Equal(t, tc.wantTokenAuth, gotTokenAuth)
		})
	}
}

type basicAuthStore struct {
	AuthStore
	user *database.User
}

func (s basicAuthStore) AuthenticateUser(_ context.Context, login, password string, _ int64) (*database.User, error) {
	if login != s.user.Name || password != "secret" {
		return nil, auth.ErrBadCredentials{}
	}
	return s.user, nil
}

func TestAuthenticatedUserBasicAuth(t *testing.T) {
	store := basicAuthStore{user: &database.User{ID: 7, Name: "alice", IsActive: true}}

	tests := []struct {
		path     string
		wantUser bool
	}{
		{path: "/api/v1/user", wantUser: true},
		{path: "/alice/repo/raw/main/README.md", wantUser: true},
		{path: "/api/web/user/settings/two-factor", wantUser: false},
		{path: "/api/web/user/settings/passkeys/registration", wantUser: false},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			m := macaron.New()
			m.Use(session.Sessioner())
			var got *database.User
			var gotBasicAuth bool
			m.Any("/*", func(c *macaron.Context, sess session.Store) {
				got, gotBasicAuth, _ = authenticatedUser(logx.NewNoopLogger(), store, c, sess)
			})
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.SetBasicAuth("alice", "secret")
			m.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.wantUser, got != nil)
			assert.Equal(t, tc.wantUser, gotBasicAuth)
		})
	}
}

type testLocale struct{}

func (testLocale) Language() string               { return "en-US" }
func (testLocale) Tr(key string, _ ...any) string { return key }

func TestToggle(t *testing.T) {
	originalSubpath, originalConfirmation := conf.Server.Subpath, conf.Auth.RequireEmailConfirmation
	t.Cleanup(func() {
		conf.Server.Subpath, conf.Auth.RequireEmailConfirmation = originalSubpath, originalConfirmation
	})
	conf.Server.Subpath = "/gogs"
	conf.Auth.RequireEmailConfirmation = true

	serve := func(t *testing.T, viewer *database.User, path string) *httptest.ResponseRecorder {
		t.Helper()
		webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("web app"))
		})
		m := macaron.New()
		m.Use(func(ctx *macaron.Context) {
			ctx.Locale = testLocale{}
			ctx.Map(&Context{Context: ctx, User: viewer, IsLogged: viewer != nil, webHandler: webHandler})
		})
		m.Get("/:username/:reponame", Toggle(&ToggleOptions{SignInRequired: true}), func(ctx *macaron.Context) {
			_, _ = ctx.Resp.Write([]byte("page"))
		})
		resp := httptest.NewRecorder()
		m.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, path, nil))
		return resp
	}

	t.Run("anonymous visitor is sent to sign in and back", func(t *testing.T) {
		resp := serve(t, nil, "/alice/repo?type=open&page=2")
		assert.Equal(t, http.StatusFound, resp.Code)
		assert.Equal(t, "/gogs/user/sign-in?redirect_to=%2Fgogs%2Falice%2Frepo%3Ftype%3Dopen%26page%3D2", resp.Header().Get("Location"))
	})

	// Frontend assets match "/:username/:reponame" too, and the activation page
	// cannot render without them.
	t.Run("frontend assets are served to an unconfirmed user", func(t *testing.T) {
		for _, path := range []string{"/assets/index.js", "/src/main.tsx"} {
			resp := serve(t, &database.User{ID: 1, IsActive: false}, path)
			assert.Equal(t, http.StatusOK, resp.Code, path)
			assert.Equal(t, "web app", resp.Body.String(), path)
		}
	})

	// A user or organization named "src" is allowed, so its repositories must stay
	// reachable for viewers who may see them, and a missing asset must stay a 404
	// from the normal routes.
	t.Run("asset paths go through the normal routes for viewers who may see them", func(t *testing.T) {
		for _, path := range []string{"/src/app", "/assets/stale.js"} {
			resp := serve(t, &database.User{ID: 1, IsActive: true}, path)
			assert.Equal(t, http.StatusOK, resp.Code, path)
			assert.Equal(t, "page", resp.Body.String(), path)
		}
	})
}

func TestContextNotFoundPassesUser(t *testing.T) {
	var got *database.User
	webHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = r.Context().Value(UserKey{}).(*database.User)
	})
	viewer := &database.User{ID: 7}
	m := macaron.New()
	m.Use(func(ctx *macaron.Context) {
		ctx.Locale = testLocale{}
		ctx.Map(&Context{Context: ctx, User: viewer, IsLogged: true, webHandler: webHandler})
	})
	m.Get("/alice/missing", func(c *Context) { c.NotFound() })

	m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/alice/missing", nil))
	assert.Same(t, viewer, got)
}
