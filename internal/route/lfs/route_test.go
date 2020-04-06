// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfs

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/db"
)

func Test_authenticate(t *testing.T) {
	m := macaron.New()
	m.Use(macaron.Renderer())
	m.Get("/", authenticate(), func(w http.ResponseWriter, user *db.User) {
		fmt.Fprintf(w, "ID: %d, Name: %s", user.ID, user.Name)
	})

	tests := []struct {
		name                  string
		header                http.Header
		mockUsersStore        *db.MockUsersStore
		mockTwoFactorsStore   *db.MockTwoFactorsStore
		mockAccessTokensStore *db.MockAccessTokensStore
		expStatusCode         int
		expHeader             http.Header
		expBody               string
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
			mockUsersStore: &db.MockUsersStore{
				MockAuthenticate: func(username, password string, loginSourceID int64) (*db.User, error) {
					return &db.User{}, nil
				},
			},
			mockTwoFactorsStore: &db.MockTwoFactorsStore{
				MockIsUserEnabled: func(userID int64) bool {
					return true
				},
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
			mockUsersStore: &db.MockUsersStore{
				MockAuthenticate: func(username, password string, loginSourceID int64) (*db.User, error) {
					return nil, db.ErrUserNotExist{}
				},
			},
			mockAccessTokensStore: &db.MockAccessTokensStore{
				MockGetBySHA: func(sha string) (*db.AccessToken, error) {
					return nil, db.ErrAccessTokenNotExist{}
				},
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
			mockUsersStore: &db.MockUsersStore{
				MockAuthenticate: func(username, password string, loginSourceID int64) (*db.User, error) {
					return &db.User{ID: 1, Name: "unknwon"}, nil
				},
			},
			mockTwoFactorsStore: &db.MockTwoFactorsStore{
				MockIsUserEnabled: func(userID int64) bool {
					return false
				},
			},
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 1, Name: unknwon",
		},
		{
			name: "authenticate by access token",
			header: http.Header{
				"Authorization": []string{"Basic dXNlcm5hbWU="},
			},
			mockUsersStore: &db.MockUsersStore{
				MockAuthenticate: func(username, password string, loginSourceID int64) (*db.User, error) {
					return nil, db.ErrUserNotExist{}
				},
				MockGetByID: func(id int64) (*db.User, error) {
					return &db.User{ID: 1, Name: "unknwon"}, nil
				},
			},
			mockAccessTokensStore: &db.MockAccessTokensStore{
				MockGetBySHA: func(sha string) (*db.AccessToken, error) {
					return &db.AccessToken{}, nil
				},
				MockSave: func(t *db.AccessToken) error {
					if t.Updated.IsZero() {
						return errors.New("Updated is zero")
					}
					return nil
				},
			},
			expStatusCode: http.StatusOK,
			expHeader:     http.Header{},
			expBody:       "ID: 1, Name: unknwon",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db.SetMockUsersStore(t, test.mockUsersStore)
			db.SetMockTwoFactorsStore(t, test.mockTwoFactorsStore)
			db.SetMockAccessTokensStore(t, test.mockAccessTokensStore)

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

			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			assert.Equal(t, test.expBody, string(body))
		})
	}
}
