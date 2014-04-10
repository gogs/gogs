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

// Package oauth2 contains Martini handlers to provide
// user login via an OAuth 2.0 backend.
package oauth2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"code.google.com/p/goauth2/oauth"
	"github.com/go-martini/martini"

	"github.com/gogits/session"

	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	keyToken    = "oauth2_token"
	keyNextPage = "next"
)

var (
	// Path to handle OAuth 2.0 logins.
	PathLogin = "/login"
	// Path to handle OAuth 2.0 logouts.
	PathLogout = "/logout"
	// Path to handle callback from OAuth 2.0 backend
	// to exchange credentials.
	PathCallback = "/oauth2callback"
	// Path to handle error cases.
	PathError = "/oauth2error"
)

// Represents OAuth2 backend options.
type Options struct {
	ClientId     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string

	AuthUrl  string
	TokenUrl string
}

// Represents a container that contains
// user's OAuth 2.0 access and refresh tokens.
type Tokens interface {
	Access() string
	Refresh() string
	IsExpired() bool
	ExpiryTime() time.Time
	ExtraData() map[string]string
}

type token struct {
	oauth.Token
}

func (t *token) ExtraData() map[string]string {
	return t.Extra
}

// Returns the access token.
func (t *token) Access() string {
	return t.AccessToken
}

// Returns the refresh token.
func (t *token) Refresh() string {
	return t.RefreshToken
}

// Returns whether the access token is
// expired or not.
func (t *token) IsExpired() bool {
	if t == nil {
		return true
	}
	return t.Expired()
}

// Returns the expiry time of the user's
// access token.
func (t *token) ExpiryTime() time.Time {
	return t.Expiry
}

// Formats tokens into string.
func (t *token) String() string {
	return fmt.Sprintf("tokens: %v", t)
}

// Returns a new Google OAuth 2.0 backend endpoint.
func Google(opts *Options) martini.Handler {
	opts.AuthUrl = "https://accounts.google.com/o/oauth2/auth"
	opts.TokenUrl = "https://accounts.google.com/o/oauth2/token"
	return NewOAuth2Provider(opts)
}

// Returns a new Github OAuth 2.0 backend endpoint.
func Github(opts *Options) martini.Handler {
	opts.AuthUrl = "https://github.com/login/oauth/authorize"
	opts.TokenUrl = "https://github.com/login/oauth/access_token"
	return NewOAuth2Provider(opts)
}

func Facebook(opts *Options) martini.Handler {
	opts.AuthUrl = "https://www.facebook.com/dialog/oauth"
	opts.TokenUrl = "https://graph.facebook.com/oauth/access_token"
	return NewOAuth2Provider(opts)
}

// Returns a generic OAuth 2.0 backend endpoint.
func NewOAuth2Provider(opts *Options) martini.Handler {
	config := &oauth.Config{
		ClientId:     opts.ClientId,
		ClientSecret: opts.ClientSecret,
		RedirectURL:  opts.RedirectURL,
		Scope:        strings.Join(opts.Scopes, " "),
		AuthURL:      opts.AuthUrl,
		TokenURL:     opts.TokenUrl,
	}

	transport := &oauth.Transport{
		Config:    config,
		Transport: http.DefaultTransport,
	}

	return func(c martini.Context, ctx *middleware.Context) {
		if ctx.Req.Method == "GET" {
			switch ctx.Req.URL.Path {
			case PathLogin:
				login(transport, ctx)
			case PathLogout:
				logout(transport, ctx)
			case PathCallback:
				handleOAuth2Callback(transport, ctx)
			}
		}

		tk := unmarshallToken(ctx.Session)
		if tk != nil {
			// check if the access token is expired
			if tk.IsExpired() && tk.Refresh() == "" {
				ctx.Session.Delete(keyToken)
				tk = nil
			}
		}
		// Inject tokens.
		c.MapTo(tk, (*Tokens)(nil))
	}
}

// Handler that redirects user to the login page
// if user is not logged in.
// Sample usage:
// m.Get("/login-required", oauth2.LoginRequired, func() ... {})
var LoginRequired martini.Handler = func() martini.Handler {
	return func(c martini.Context, ctx *middleware.Context) {
		token := unmarshallToken(ctx.Session)
		if token == nil || token.IsExpired() {
			next := url.QueryEscape(ctx.Req.URL.RequestURI())
			ctx.Redirect(PathLogin + "?next=" + next)
			return
		}
	}
}()

func login(t *oauth.Transport, ctx *middleware.Context) {
	next := extractPath(ctx.Query(keyNextPage))
	if ctx.Session.Get(keyToken) == nil {
		// User is not logged in.
		ctx.Redirect(t.Config.AuthCodeURL(next))
		return
	}
	// No need to login, redirect to the next page.
	ctx.Redirect(next)
}

func logout(t *oauth.Transport, ctx *middleware.Context) {
	next := extractPath(ctx.Query(keyNextPage))
	ctx.Session.Delete(keyToken)
	ctx.Redirect(next)
}

func handleOAuth2Callback(t *oauth.Transport, ctx *middleware.Context) {
	if errMsg := ctx.Query("error_description"); len(errMsg) > 0 {
		log.Error("oauth2.handleOAuth2Callback: %s", errMsg)
		return
	}

	next := extractPath(ctx.Query("state"))
	code := ctx.Query("code")
	tk, err := t.Exchange(code)
	if err != nil {
		// Pass the error message, or allow dev to provide its own
		// error handler.
		log.Error("oauth2.handleOAuth2Callback(token.Exchange): %v", err)
		// ctx.Redirect(PathError)
		return
	}
	// Store the credentials in the session.
	val, _ := json.Marshal(tk)
	ctx.Session.Set(keyToken, val)
	ctx.Redirect(next)
}

func unmarshallToken(s session.SessionStore) (t *token) {
	if s.Get(keyToken) == nil {
		return
	}
	data := s.Get(keyToken).([]byte)
	var tk oauth.Token
	json.Unmarshal(data, &tk)
	return &token{tk}
}

func extractPath(next string) string {
	n, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	return n.Path
}
