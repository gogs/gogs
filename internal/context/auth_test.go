package context

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/macaron.v1"

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

	serve := func(t *testing.T, user *database.User, path string) *httptest.ResponseRecorder {
		t.Helper()
		webHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("web app"))
		})
		m := macaron.New()
		m.Use(func(ctx *macaron.Context) {
			ctx.Locale = testLocale{}
			ctx.Map(&Context{Context: ctx, User: user, IsLogged: user != nil, webHandler: webHandler})
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

func TestContext_NotFound_passesUser(t *testing.T) {
	var got *database.User
	webHandler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got, _ = r.Context().Value(UserKey{}).(*database.User)
	})
	user := &database.User{ID: 7}
	m := macaron.New()
	m.Use(func(ctx *macaron.Context) {
		ctx.Locale = testLocale{}
		ctx.Map(&Context{Context: ctx, User: user, IsLogged: true, webHandler: webHandler})
	})
	m.Get("/alice/missing", func(c *Context) { c.NotFound() })

	m.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/alice/missing", nil))
	assert.Same(t, user, got)
}
