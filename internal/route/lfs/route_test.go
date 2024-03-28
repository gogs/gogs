// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsutil"
)

func TestAuthenticate(t *testing.T) {
	tests := []struct {
		name          string
		header        http.Header
		mockStore     func() *MockStore
		expStatusCode int
		expHeader     http.Header
		expBody       string
	}{
		{
			name:          "no authorization",
			expStatusCode: http.StatusUnauthorized,
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mockStore == nil {
				test.mockStore = NewMockStore
			}

			m := macaron.New()
			m.Use(macaron.Renderer())
			m.Get("/", authenticate(test.mockStore()), func(w http.ResponseWriter, user *database.User) {
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

func TestAuthorize(t *testing.T) {
	tests := []struct {
		name          string
		accessMode    database.AccessMode
		mockStore     func() *MockStore
		expStatusCode int
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

			m := macaron.New()
			m.Use(macaron.Renderer())
			m.Use(func(c *macaron.Context) {
				c.Map(&database.User{})
			})
			m.Get(
				"/:username/:reponame",
				authorize(mockStore, test.accessMode),
				func(w http.ResponseWriter, owner *database.User, repo *database.Repository) {
					_, _ = fmt.Fprintf(w, "owner.Name: %s, repo.Name: %s", owner.Name, repo.Name)
				},
			)

			r, err := http.NewRequest("GET", "/owner/repo", nil)
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

func Test_verifyHeader(t *testing.T) {
	tests := []struct {
		name          string
		verifyHeader  macaron.Handler
		header        http.Header
		expStatusCode int
	}{
		{
			name:          "header not found",
			verifyHeader:  verifyHeader("Accept", contentType, http.StatusNotAcceptable),
			expStatusCode: http.StatusNotAcceptable,
		},

		{
			name:         "header found",
			verifyHeader: verifyHeader("Accept", "application/vnd.git-lfs+json", http.StatusNotAcceptable),
			header: http.Header{
				"Accept": []string{"application/vnd.git-lfs+json; charset=utf-8"},
			},
			expStatusCode: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			m := macaron.New()
			m.Use(macaron.Renderer())
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
	m := macaron.New()
	m.Get("/:oid", verifyOID(), func(w http.ResponseWriter, oid lfsutil.OID) {
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
	internalServerError(rr)

	resp := rr.Result()
	assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, `{"message":"Internal server error"}`+"\n", string(body))
}
