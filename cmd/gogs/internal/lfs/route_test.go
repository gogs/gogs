package lfs

import (
	"context"
	"fmt"
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
	"gogs.io/gogs/internal/lfsx"
)

func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name              string
		header            http.Header
		requireSigninView bool
		mockStore         func() *MockStore
		expStatusCode     int
		expHeader         http.Header
		expBody           string
	}{
		{
			name:          "no authorization",
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 0, Name: anonymous",
		},
		{
			name:              "no authorization when signing in is required to view",
			requireSigninView: true,
			expStatusCode:     http.StatusUnauthorized,
			expHeader: http.Header{
				"Lfs-Authenticate": []string{`Basic realm="Git LFS"`},
				"Content-Type":     []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Credentials needed"}` + "\n",
		},
		{
			name: "user has 2FA enabled",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.IsTwoFactorEnabledFunc.SetDefaultReturn(true)
				mockStore.AuthenticateUserFunc.SetDefaultReturn(&database.User{}, nil)
				return mockStore
			},
			expStatusCode: http.StatusBadRequest,
			expHeader:     http.Header{},
			expBody:       "Users with 2FA enabled are not allowed to authenticate via username and password.",
		},
		{
			name: "both user and access token do not exist",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetAccessTokenBySHA1Func.SetDefaultReturn(nil, database.ErrAccessTokenNotExist{})
				mockStore.AuthenticateUserFunc.SetDefaultReturn(nil, auth.ErrBadCredentials{})
				return mockStore
			},
			expStatusCode: http.StatusUnauthorized,
			expHeader: http.Header{
				"Lfs-Authenticate": []string{`Basic realm="Git LFS"`},
				"Content-Type":     []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Credentials needed"}` + "\n",
		},

		{
			name: "authenticated by username and password",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.IsTwoFactorEnabledFunc.SetDefaultReturn(false)
				mockStore.AuthenticateUserFunc.SetDefaultReturn(&database.User{ID: 1, Name: "unknwon"}, nil)
				return mockStore
			},
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 1, Name: unknwon",
		},
		{
			name: "authenticate by access token via username",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetAccessTokenBySHA1Func.SetDefaultReturn(&database.AccessToken{}, nil)
				mockStore.AuthenticateUserFunc.SetDefaultReturn(nil, auth.ErrBadCredentials{})
				mockStore.GetUserByIDFunc.SetDefaultReturn(&database.User{ID: 1, Name: "unknwon"}, nil)
				return mockStore
			},
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 1, Name: unknwon",
		},
		{
			name: "authenticate by access token via password",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetAccessTokenBySHA1Func.SetDefaultHook(func(_ context.Context, sha1 string) (*database.AccessToken, error) {
					if sha1 == "password" {
						return &database.AccessToken{}, nil
					}
					return nil, database.ErrAccessTokenNotExist{}
				})
				mockStore.AuthenticateUserFunc.SetDefaultReturn(nil, auth.ErrBadCredentials{})
				mockStore.GetUserByIDFunc.SetDefaultReturn(&database.User{ID: 1, Name: "unknwon"}, nil)
				return mockStore
			},
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 1, Name: unknwon",
		},
		{
			name: "user with prohibited login by username and password",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU6cGFzc3dvcmQ="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.IsTwoFactorEnabledFunc.SetDefaultReturn(false)
				mockStore.AuthenticateUserFunc.SetDefaultReturn(&database.User{ID: 1, Name: "unknwon", ProhibitLogin: true}, nil)
				return mockStore
			},
			expStatusCode: http.StatusForbidden,
			expHeader: http.Header{
				"Content-Type": []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"User account is disabled"}` + "\n",
		},
		{
			name: "user with prohibited login by access token",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU="},
			},
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetAccessTokenBySHA1Func.SetDefaultReturn(&database.AccessToken{}, nil)
				mockStore.AuthenticateUserFunc.SetDefaultReturn(nil, auth.ErrBadCredentials{})
				mockStore.GetUserByIDFunc.SetDefaultReturn(&database.User{ID: 1, Name: "unknwon", ProhibitLogin: true}, nil)
				return mockStore
			},
			expStatusCode: http.StatusForbidden,
			expHeader: http.Header{
				"Content-Type": []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"User account is disabled"}` + "\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockStore == nil {
				test.mockStore = NewMockStore
			}

			m := flamego.New()
			m.Map(conf.AuthOpts{RequireSigninView: test.requireSigninView})
			m.Get("/", authenticate(logx.NewNoopLogger(), test.mockStore()), func(w http.ResponseWriter, user *database.User) {
				_, _ = fmt.Fprintf(w, "ID: %d, Name: %s", user.ID, user.Name)
			})

			r, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header = test.header

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
			assert.Equal(t, test.expHeader, resp.Header)

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func TestAuthenticate_reverseProxy(t *testing.T) {
	_, trusted, err := net.ParseCIDR("10.0.0.0/8")
	require.NoError(t, err)

	tests := []struct {
		name              string
		remoteAddr        string
		proxyUser         string
		requireSigninView bool
		expStatusCode     int
		expBody           string
	}{
		{
			name:          "header from trusted proxy goes before Basic authentication",
			remoteAddr:    "10.0.0.1:1234",
			proxyUser:     "unknwon",
			expStatusCode: http.StatusOK,
			expBody:       "ID: 1, Name: unknwon",
		},
		{
			name:          "header from trusted proxy names unknown user",
			remoteAddr:    "10.0.0.1:1234",
			proxyUser:     "ghost",
			expStatusCode: http.StatusOK,
			expBody:       "ID: 0, Name: anonymous",
		},
		{
			name:              "header from trusted proxy names unknown user when signing in is required",
			remoteAddr:        "10.0.0.1:1234",
			proxyUser:         "ghost",
			requireSigninView: true,
			expStatusCode:     http.StatusNotFound,
		},
		{
			name:          "empty header from trusted proxy falls back to Basic authentication",
			remoteAddr:    "10.0.0.1:1234",
			proxyUser:     "",
			expStatusCode: http.StatusOK,
			expBody:       "ID: 2, Name: username",
		},
		{
			name:          "header from untrusted address is ignored",
			remoteAddr:    "192.0.2.1:1234",
			proxyUser:     "unknwon",
			expStatusCode: http.StatusOK,
			expBody:       "ID: 2, Name: username",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockStore := NewMockStore()
			mockStore.GetUserByUsernameFunc.SetDefaultHook(func(_ context.Context, username string) (*database.User, error) {
				if username == "unknwon" {
					return &database.User{ID: 1, Name: "unknwon"}, nil
				}
				return nil, database.ErrUserNotExist{}
			})
			// The Basic credentials name a different user, so the response shows
			// which of the two authenticated the request.
			mockStore.AuthenticateUserFunc.SetDefaultHook(func(_ context.Context, login, password string, _ int64) (*database.User, error) {
				if login == "username" && password == "password" {
					return &database.User{ID: 2, Name: "username"}, nil
				}
				return nil, auth.ErrBadCredentials{}
			})

			m := flamego.New()
			m.Map(conf.AuthOpts{
				EnableReverseProxyAuthentication: true,
				ReverseProxyAuthenticationHeader: "X-WEBAUTH-USER",
				TrustedProxyCIDRs:                []*net.IPNet{trusted},
				RequireSigninView:                test.requireSigninView,
			})
			m.Get("/", authenticate(logx.NewNoopLogger(), mockStore), func(w http.ResponseWriter, user *database.User) {
				_, _ = fmt.Fprintf(w, "ID: %d, Name: %s", user.ID, user.Name)
			})

			r := httptest.NewRequest("GET", "/", nil)
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

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name          string
		anonymous     bool
		proxyUser     string
		accessMode    database.AccessMode
		mockStore     func() *MockStore
		expStatusCode int
		expHeader     http.Header
		expBody       string
	}{
		{
			name:       "user does not exist",
			accessMode: database.AccessModeNone,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetUserByUsernameFunc.SetDefaultReturn(nil, database.ErrUserNotExist{})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:       "repository does not exist",
			accessMode: database.AccessModeNone,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetRepositoryByNameFunc.SetDefaultReturn(nil, database.ErrRepoNotExist{})
				mockStore.GetUserByUsernameFunc.SetDefaultHook(func(ctx context.Context, username string) (*database.User, error) {
					return &database.User{Name: username}, nil
				})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:       "actor is not authorized",
			accessMode: database.AccessModeWrite,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.AuthorizeRepositoryAccessFunc.SetDefaultHook(func(_ context.Context, _ int64, _ int64, desired database.AccessMode, _ database.AccessModeOptions) bool {
					return desired <= database.AccessModeRead
				})
				mockStore.GetRepositoryByNameFunc.SetDefaultHook(func(ctx context.Context, ownerID int64, name string) (*database.Repository, error) {
					return &database.Repository{Name: name}, nil
				})
				mockStore.GetUserByUsernameFunc.SetDefaultHook(func(ctx context.Context, username string) (*database.User, error) {
					return &database.User{Name: username}, nil
				})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:       "anonymous actor is not authorized",
			anonymous:  true,
			accessMode: database.AccessModeWrite,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.AuthorizeRepositoryAccessFunc.SetDefaultHook(func(_ context.Context, _ int64, _ int64, desired database.AccessMode, _ database.AccessModeOptions) bool {
					return desired <= database.AccessModeRead
				})
				mockStore.GetRepositoryByNameFunc.SetDefaultHook(func(ctx context.Context, ownerID int64, name string) (*database.Repository, error) {
					return &database.Repository{Name: name}, nil
				})
				mockStore.GetUserByUsernameFunc.SetDefaultHook(func(ctx context.Context, username string) (*database.User, error) {
					return &database.User{Name: username}, nil
				})
				return mockStore
			},
			expStatusCode: http.StatusUnauthorized,
			expHeader: http.Header{
				"Lfs-Authenticate": []string{`Basic realm="Git LFS"`},
				"Content-Type":     []string{"application/vnd.git-lfs+json"},
			},
			expBody: `{"message":"Credentials needed"}` + "\n",
		},
		{
			name:       "anonymous actor named by reverse proxy is not authorized",
			anonymous:  true,
			proxyUser:  "ghost",
			accessMode: database.AccessModeRead,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.GetRepositoryByNameFunc.SetDefaultHook(func(ctx context.Context, ownerID int64, name string) (*database.Repository, error) {
					return &database.Repository{Name: name, IsPrivate: true}, nil
				})
				mockStore.GetUserByUsernameFunc.SetDefaultHook(func(ctx context.Context, username string) (*database.User, error) {
					return &database.User{Name: username}, nil
				})
				return mockStore
			},
			expStatusCode: http.StatusNotFound,
		},
		{
			name:       "actor is authorized",
			accessMode: database.AccessModeRead,
			mockStore: func() *MockStore {
				mockStore := NewMockStore()
				mockStore.AuthorizeRepositoryAccessFunc.SetDefaultHook(func(_ context.Context, _ int64, _ int64, desired database.AccessMode, _ database.AccessModeOptions) bool {
					return desired <= database.AccessModeRead
				})
				mockStore.GetRepositoryByNameFunc.SetDefaultHook(func(ctx context.Context, ownerID int64, name string) (*database.Repository, error) {
					return &database.Repository{Name: name}, nil
				})
				mockStore.GetUserByUsernameFunc.SetDefaultHook(func(ctx context.Context, username string) (*database.User, error) {
					return &database.User{Name: username}, nil
				})
				return mockStore
			},
			expStatusCode: http.StatusOK,
			expBody:       "owner.Name: owner, repo.Name: repo",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mockStore := NewMockStore()
			if test.mockStore != nil {
				mockStore = test.mockStore()
			}

			var authOpts conf.AuthOpts
			if test.proxyUser != "" {
				_, trusted, err := net.ParseCIDR("10.0.0.0/8")
				require.NoError(t, err)
				authOpts = conf.AuthOpts{
					EnableReverseProxyAuthentication: true,
					ReverseProxyAuthenticationHeader: "X-WEBAUTH-USER",
					TrustedProxyCIDRs:                []*net.IPNet{trusted},
				}
			}

			m := flamego.New()
			m.Map(authOpts)
			m.Use(func(c flamego.Context) {
				if test.anonymous {
					c.Map(&database.User{})
				} else {
					c.Map(&database.User{ID: 1})
				}
			})
			m.Get(
				"/{owner}/{repo}",
				authorize(logx.NewNoopLogger(), mockStore, test.accessMode),
				func(w http.ResponseWriter, owner *database.User, repo *database.Repository) {
					_, _ = fmt.Fprintf(w, "owner.Name: %s, repo.Name: %s", owner.Name, repo.Name)
				},
			)

			r, err := http.NewRequest("GET", "/owner/repo", nil)
			if err != nil {
				t.Fatal(err)
			}

			if test.proxyUser != "" {
				r.RemoteAddr = "10.0.0.1:1234"
				r.Header.Set("X-WEBAUTH-USER", test.proxyUser)
			}

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
			if test.expHeader != nil {
				assert.Equal(t, test.expHeader, resp.Header)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func Test_verifyHeader(t *testing.T) {
	tests := []struct {
		name          string
		verifyHeader  flamego.Handler
		header        http.Header
		expStatusCode int
	}{
		{
			name:          "header not found",
			verifyHeader:  verifyHeader(logx.NewNoopLogger(), "Accept", contentType, http.StatusNotAcceptable),
			expStatusCode: http.StatusNotAcceptable,
		},

		{
			name:         "header found",
			verifyHeader: verifyHeader(logx.NewNoopLogger(), "Accept", "application/vnd.git-lfs+json", http.StatusNotAcceptable),
			header: http.Header{
				"Accept": []string{"application/vnd.git-lfs+json; charset=utf-8"},
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := flamego.New()
			m.Get("/", test.verifyHeader)

			r, err := http.NewRequest("GET", "/", nil)
			if err != nil {
				t.Fatal(err)
			}
			r.Header = test.header

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)
		})
	}
}

func Test_verifyOID(t *testing.T) {
	m := flamego.New()
	m.Get("/{oid}", verifyOID(logx.NewNoopLogger()), func(w http.ResponseWriter, oid lfsx.OID) {
		fmt.Fprintf(w, "oid: %s", oid)
	})

	tests := []struct {
		name          string
		url           string
		expStatusCode int
		expBody       string
	}{
		{
			name:          "bad oid",
			url:           "/bad_oid",
			expStatusCode: http.StatusBadRequest,
			expBody:       `{"message":"Invalid oid"}` + "\n",
		},

		{
			name:          "good oid",
			url:           "/ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
			expStatusCode: http.StatusOK,
			expBody:       "oid: ef797c8118f02dfb649607dd5d3f8c7623048c9c063d532cc95c5ed7a898a64f",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			r, err := http.NewRequest("GET", test.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			m.ServeHTTP(rr, r)

			resp := rr.Result()
			assert.Equal(t, test.expStatusCode, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}

func Test_internalServerError(t *testing.T) {
	rr := httptest.NewRecorder()
	internalServerError(logx.NewNoopLogger(), rr)

	resp := rr.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `{"message":"Internal server error"}`+"\n", string(body))
}
