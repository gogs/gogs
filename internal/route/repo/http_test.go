package repo

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func TestGitHTTPActionFromPath(t *testing.T) {
	tests := []struct {
		name    string
		urlPath string
		subpath string
		owner   string
		repo    string
		want    string
	}{
		{
			name:    "info refs",
			urlPath: "/owner/repo/info/refs",
			owner:   "owner",
			repo:    "repo",
			want:    "info/refs",
		},
		{
			name:    "git suffix repo",
			urlPath: "/owner/repo.git/git-receive-pack",
			owner:   "owner",
			repo:    "repo.git",
			want:    "git-receive-pack",
		},
		{
			name:    "with subpath",
			urlPath: "/gogs/owner/repo/info/refs",
			subpath: "/gogs",
			owner:   "owner",
			repo:    "repo",
			want:    "info/refs",
		},
		{
			name:    "owner starts like the subpath",
			urlPath: "/github/repo/info/refs",
			subpath: "/git",
			owner:   "github",
			repo:    "repo",
			want:    "info/refs",
		},
		{
			name:    "owner named like the subpath",
			urlPath: "/git/repo/git-upload-pack",
			subpath: "/git",
			owner:   "git",
			repo:    "repo",
			want:    "git-upload-pack",
		},
		{
			name:    "non git suffix",
			urlPath: "/owner/repo/src/main.go",
			owner:   "owner",
			repo:    "repo",
			want:    "src/main.go",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := gitHTTPActionFromPath(tc.urlPath, tc.subpath, tc.owner, tc.repo)
			assert.Equal(t, tc.want, got)
		})
	}
}

type reverseProxyStore struct {
	Store
}

func (reverseProxyStore) GetUserByUsername(_ context.Context, username string) (*database.User, error) {
	switch username {
	case "alice":
		return &database.User{ID: 1, Name: "alice"}, nil
	case "disabled":
		return &database.User{ID: 2, Name: "disabled", ProhibitLogin: true}, nil
	}
	return nil, database.ErrUserNotExist{}
}

func (reverseProxyStore) GetRepositoryByName(context.Context, int64, string) (*database.Repository, error) {
	return &database.Repository{ID: 1, OwnerID: 1, Name: "p", IsPrivate: true}, nil
}

func (reverseProxyStore) CreateUser(context.Context, string, string, database.CreateUserOptions) (*database.User, error) {
	return nil, database.ErrNameNotAllowed{}
}

func (reverseProxyStore) AuthenticateUser(context.Context, string, string, int64) (*database.User, error) {
	return nil, auth.ErrBadCredentials{}
}

func (reverseProxyStore) GetAccessTokenBySHA1(context.Context, string) (*database.AccessToken, error) {
	return nil, database.ErrAccessTokenNotExist{}
}

func TestHTTPContexter_reverseProxy(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	// The repository is private and the Basic credentials are wrong, so a
	// response other than 401 shows the header authenticated the request. The
	// header names a disabled user to stop before checking repository
	// permissions, which need the database.
	tests := []struct {
		name             string
		remoteAddr       string
		proxyUser        string
		autoRegistration bool
		expStatusCode    int
		expBody          string
	}{
		{
			name:          "header from trusted proxy goes before Basic authentication",
			remoteAddr:    "10.0.0.1:1234",
			proxyUser:     "disabled",
			expStatusCode: http.StatusForbidden,
			expBody:       "User account is disabled",
		},
		{
			name:          "header from trusted proxy names unknown user",
			remoteAddr:    "10.0.0.1:1234",
			proxyUser:     "ghost",
			expStatusCode: http.StatusNotFound,
		},
		{
			name:             "header from trusted proxy names a user that cannot be registered",
			remoteAddr:       "10.0.0.1:1234",
			proxyUser:        "ghost",
			autoRegistration: true,
			expStatusCode:    http.StatusNotFound,
		},
		{
			name:          "header from untrusted address is ignored",
			remoteAddr:    "192.0.2.1:1234",
			proxyUser:     "disabled",
			expStatusCode: http.StatusUnauthorized,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conf.SetMockAuth(t, conf.AuthOpts{
				EnableReverseProxyAuthentication:   true,
				EnableReverseProxyAutoRegistration: test.autoRegistration,
				ReverseProxyAuthenticationHeader:   "X-WEBAUTH-USER",
				TrustedProxyCIDRs:                  []*net.IPNet{trusted},
			})

			m := flamego.New()
			m.Get("/{owner}/{repo}/info/refs", HTTPContexter(logx.NewNoopLogger(), reverseProxyStore{}), func(w http.ResponseWriter) {
				w.WriteHeader(http.StatusOK)
			})

			r := httptest.NewRequest("GET", "/alice/p.git/info/refs?service=git-upload-pack", nil)
			r.RemoteAddr = test.remoteAddr
			r.Header.Set("Authorization", "Basic dXNlcm5hbWU6cGFzc3dvcmQ=")
			r.Header.Set("X-WEBAUTH-USER", test.proxyUser)

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.Equal(t, test.expBody, string(body))
		})
	}
}
