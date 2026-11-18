package context

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/flamego/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			f := flamego.New()
			f.Use(session.Sessioner())
			var gotUserID int64
			var gotTokenAuth bool
			f.Any("/{**}", func(r *http.Request, sess session.Session) {
				gotUserID, gotTokenAuth = authenticatedUserID(logx.NewNoopLogger(), store, r, sess)
			})
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "token "+store.token.Sha1)
			f.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.wantUserID, gotUserID)
			assert.Equal(t, tc.wantTokenAuth, gotTokenAuth)
		})
	}
}

type basicAuthStore struct {
	AuthStore
	user      *database.User
	twoFactor bool
}

func (s basicAuthStore) IsTwoFactorEnabled(context.Context, int64) bool { return s.twoFactor }

func (s basicAuthStore) AuthenticateUser(_ context.Context, login, password string, _ int64) (*database.User, error) {
	if login != s.user.Name || password != "secret" {
		return nil, auth.ErrBadCredentials{}
	}
	return s.user, nil
}

func TestAuthenticatedUserBasicAuth(t *testing.T) {
	user := &database.User{ID: 7, Name: "alice", IsActive: true}

	tests := []struct {
		name      string
		path      string
		twoFactor bool
		wantUser  bool
	}{
		{path: "/api/v1/user", wantUser: true},
		{name: "two-factor authentication enabled", path: "/api/v1/user", twoFactor: true, wantUser: false},
		{name: "two-factor authentication enabled", path: "/alice/repo/raw/main/README.md", twoFactor: true, wantUser: false},
		{path: "/alice/repo/raw/main/README.md", wantUser: true},
		{path: "/api/web/user/settings/two-factor", wantUser: false},
		{path: "/api/web/user/settings/passkeys/registration", wantUser: false},
		{path: "//api/web/user/settings/two-factor", wantUser: false},
		{path: "///api/web/user/settings/two-factor", wantUser: false},
		{path: "/%2Fapi/web/user/settings/two-factor", wantUser: false},
	}
	for _, tc := range tests {
		t.Run(strings.TrimSpace(tc.name+" "+tc.path), func(t *testing.T) {
			store := basicAuthStore{user: user, twoFactor: tc.twoFactor}
			f := flamego.New()
			f.Use(session.Sessioner())
			var got *database.User
			var gotBasicAuth bool
			f.Group("", func() {
				f.Get("/api/web/user/settings/two-factor", func() {})
				f.Get("/api/web/user/settings/passkeys/registration", func() {})
				f.Any("/{**}", func() {})
			}, func(r *http.Request, sess session.Session) {
				got, gotBasicAuth, _ = AuthenticatedUser(logx.NewNoopLogger(), store, r, sess)
			})
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.SetBasicAuth("alice", "secret")
			f.ServeHTTP(httptest.NewRecorder(), req)
			assert.Equal(t, tc.wantUser, got != nil)
			assert.Equal(t, tc.wantUser, gotBasicAuth)
		})
	}
}

// racingRegistrationStore acts as if another request registers the user
// between the lookup and CreateUser.
type racingRegistrationStore struct {
	AuthStore
	registered *bool
}

func (s racingRegistrationStore) GetUserByUsername(_ context.Context, username string) (*database.User, error) {
	if !*s.registered {
		return nil, database.ErrUserNotExist{}
	}
	return &database.User{ID: 7, Name: username}, nil
}

func (s racingRegistrationStore) CreateUser(context.Context, string, string, database.CreateUserOptions) (*database.User, error) {
	*s.registered = true
	return nil, database.ErrUserAlreadyExist{}
}

func TestReverseProxyUser_concurrentRegistration(t *testing.T) {
	_, trusted, err := net.ParseCIDR("127.0.0.0/8")
	require.NoError(t, err)
	conf.SetMockAuth(t, conf.AuthOpts{
		EnableReverseProxyAuthentication:   true,
		EnableReverseProxyAutoRegistration: true,
		ReverseProxyAuthenticationHeader:   "X-WEBAUTH-USER",
		TrustedProxyCIDRs:                  []*net.IPNet{trusted},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	req.Header.Set("X-WEBAUTH-USER", "alice")

	user, ok, err := ReverseProxyUser(racingRegistrationStore{registered: new(bool)}, req)
	require.NoError(t, err)
	assert.True(t, ok)
	require.NotNil(t, user)
	assert.Equal(t, int64(7), user.ID)
}
