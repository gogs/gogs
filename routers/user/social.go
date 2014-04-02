// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package user

import (
	"encoding/json"

	"code.google.com/p/goauth2/oauth"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/oauth2"
)

// github && google && ...
func SocialSignIn(tokens oauth2.Tokens) {
	transport := &oauth.Transport{}
	transport.Token = &oauth.Token{
		AccessToken:  tokens.Access(),
		RefreshToken: tokens.Refresh(),
		Expiry:       tokens.ExpiryTime(),
		Extra:        tokens.ExtraData(),
	}

	// Github API refer: https://developer.github.com/v3/users/
	// FIXME: need to judge url
	type GithubUser struct {
		Id    int    `json:"id"`
		Name  string `json:"login"`
		Email string `json:"email"`
	}

	// Make the request.
	scope := "https://api.github.com/user"
	r, err := transport.Client().Get(scope)
	if err != nil {
		log.Error("connect with github error: %s", err)
		// FIXME: handle error page
		return
	}
	defer r.Body.Close()

	user := &GithubUser{}
	err = json.NewDecoder(r.Body).Decode(user)
	if err != nil {
		log.Error("Get: %s", err)
	}
	log.Info("login: %s", user.Name)
	// FIXME: login here, user email to check auth, if not registe, then generate a uniq username
}
