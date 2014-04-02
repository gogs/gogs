// Copyright 2014 Google Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package oauth2

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-martini/martini"
	"github.com/martini-contrib/sessions"
)

func Test_LoginRedirect(t *testing.T) {
	recorder := httptest.NewRecorder()
	m := martini.New()
	m.Use(sessions.Sessions("my_session", sessions.NewCookieStore([]byte("secret123"))))
	m.Use(Google(&Options{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		RedirectURL:  "refresh_url",
		Scopes:       []string{"x", "y"},
	}))

	r, _ := http.NewRequest("GET", "/login", nil)
	m.ServeHTTP(recorder, r)

	location := recorder.HeaderMap["Location"][0]
	if recorder.Code != 302 {
		t.Errorf("Not being redirected to the auth page.")
	}
	if location != "https://accounts.google.com/o/oauth2/auth?access_type=&approval_prompt=&client_id=client_id&redirect_uri=refresh_url&response_type=code&scope=x+y&state=" {
		t.Errorf("Not being redirected to the right page, %v found", location)
	}
}

func Test_LoginRedirectAfterLoginRequired(t *testing.T) {
	recorder := httptest.NewRecorder()
	m := martini.Classic()
	m.Use(sessions.Sessions("my_session", sessions.NewCookieStore([]byte("secret123"))))
	m.Use(Google(&Options{
		ClientId:     "client_id",
		ClientSecret: "client_secret",
		RedirectURL:  "refresh_url",
		Scopes:       []string{"x", "y"},
	}))

	m.Get("/login-required", LoginRequired, func(tokens Tokens) (int, string) {
		return 200, tokens.Access()
	})

	r, _ := http.NewRequest("GET", "/login-required?key=value", nil)
	m.ServeHTTP(recorder, r)

	location := recorder.HeaderMap["Location"][0]
	if recorder.Code != 302 {
		t.Errorf("Not being redirected to the auth page.")
	}
	if location != "/login?next=%2Flogin-required%3Fkey%3Dvalue" {
		t.Errorf("Not being redirected to the right page, %v found", location)
	}
}

func Test_Logout(t *testing.T) {
	recorder := httptest.NewRecorder()
	s := sessions.NewCookieStore([]byte("secret123"))

	m := martini.Classic()
	m.Use(sessions.Sessions("my_session", s))
	m.Use(Google(&Options{
	// no need to configure
	}))

	m.Get("/", func(s sessions.Session) {
		s.Set(keyToken, "dummy token")
	})

	m.Get("/get", func(s sessions.Session) {
		if s.Get(keyToken) != nil {
			t.Errorf("User credentials are still kept in the session.")
		}
	})

	logout, _ := http.NewRequest("GET", "/logout", nil)
	index, _ := http.NewRequest("GET", "/", nil)

	m.ServeHTTP(httptest.NewRecorder(), index)
	m.ServeHTTP(recorder, logout)

	if recorder.Code != 302 {
		t.Errorf("Not being redirected to the next page.")
	}
}

func Test_LogoutOnAccessTokenExpiration(t *testing.T) {
	recorder := httptest.NewRecorder()
	s := sessions.NewCookieStore([]byte("secret123"))

	m := martini.Classic()
	m.Use(sessions.Sessions("my_session", s))
	m.Use(Google(&Options{
	// no need to configure
	}))

	m.Get("/addtoken", func(s sessions.Session) {
		s.Set(keyToken, "dummy token")
	})

	m.Get("/", func(s sessions.Session) {
		if s.Get(keyToken) != nil {
			t.Errorf("User not logged out although access token is expired.")
		}
	})

	addtoken, _ := http.NewRequest("GET", "/addtoken", nil)
	index, _ := http.NewRequest("GET", "/", nil)
	m.ServeHTTP(recorder, addtoken)
	m.ServeHTTP(recorder, index)
}

func Test_InjectedTokens(t *testing.T) {
	recorder := httptest.NewRecorder()
	m := martini.Classic()
	m.Use(sessions.Sessions("my_session", sessions.NewCookieStore([]byte("secret123"))))
	m.Use(Google(&Options{
	// no need to configure
	}))
	m.Get("/", func(tokens Tokens) string {
		return "Hello world!"
	})
	r, _ := http.NewRequest("GET", "/", nil)
	m.ServeHTTP(recorder, r)
}

func Test_LoginRequired(t *testing.T) {
	recorder := httptest.NewRecorder()
	m := martini.Classic()
	m.Use(sessions.Sessions("my_session", sessions.NewCookieStore([]byte("secret123"))))
	m.Use(Google(&Options{
	// no need to configure
	}))
	m.Get("/", LoginRequired, func(tokens Tokens) string {
		return "Hello world!"
	})
	r, _ := http.NewRequest("GET", "/", nil)
	m.ServeHTTP(recorder, r)
	if recorder.Code != 302 {
		t.Errorf("Not being redirected to the auth page although user is not logged in.")
	}
}
