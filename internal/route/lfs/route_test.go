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
		name                string
		header              http.Header
		mockUsersStore      *db.MockUsersStore
		mockTwoFactorsStore *db.MockTwoFactorsStore
		expStatusCode       int
		expHeader           http.Header
		expBody             string
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
					return &db.User{ID: 1}, nil
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
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db.SetMockUsersStore(t, test.mockUsersStore)
			db.SetMockTwoFactorsStore(t, test.mockTwoFactorsStore)

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
