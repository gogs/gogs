// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package user

import (
	"encoding/json"
	"strconv"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	//"github.com/gogits/gogs/modules/oauth2"

	"code.google.com/p/goauth2/oauth"
	"github.com/martini-contrib/oauth2"
)

type SocialConnector interface {
	Identity() string
	Type() int
	Name() string
	Email() string
	Token() string
}

type SocialGithub struct {
	data struct {
		Id    int    `json:"id"`
		Name  string `json:"login"`
		Email string `json:"email"`
	}
	WebToken *oauth.Token
}

func (s *SocialGithub) Identity() string {
	return strconv.Itoa(s.data.Id)
}

func (s *SocialGithub) Type() int {
	return models.OT_GITHUB
}

func (s *SocialGithub) Name() string {
	return s.data.Name
}

func (s *SocialGithub) Email() string {
	return s.data.Email
}

func (s *SocialGithub) Token() string {
	data, _ := json.Marshal(s.WebToken)
	return string(data)
}

// Github API refer: https://developer.github.com/v3/users/
func (s *SocialGithub) Update() error {
	scope := "https://api.github.com/user"
	transport := &oauth.Transport{
		Token: s.WebToken,
	}
	log.Debug("update github info")
	r, err := transport.Client().Get(scope)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(&s.data)
}

// github && google && ...
func SocialSignIn(ctx *middleware.Context, tokens oauth2.Tokens) {
	gh := &SocialGithub{
		WebToken: &oauth.Token{
			AccessToken:  tokens.Access(),
			RefreshToken: tokens.Refresh(),
			Expiry:       tokens.ExpiryTime(),
			Extra:        tokens.ExtraData(),
		},
	}
	var err error
	var u *models.User
	if err = gh.Update(); err != nil {
		// FIXME: handle error page
		log.Error("connect with github error: %s", err)
		return
	}
	var soc SocialConnector = gh
	log.Info("login: %s", soc.Name())
	// FIXME: login here, user email to check auth, if not registe, then generate a uniq username
	if u, err = models.GetOauth2User(soc.Identity()); err != nil {
		u = &models.User{
			Name:     soc.Name(),
			Email:    soc.Email(),
			Passwd:   "123456",
			IsActive: !base.Service.RegisterEmailConfirm,
		}
		if u, err = models.RegisterUser(u); err != nil {
			log.Error("register user: %v", err)
			return
		}
		oa := &models.Oauth2{}
		oa.Uid = u.Id
		oa.Type = soc.Type()
		oa.Token = soc.Token()
		oa.Identity = soc.Identity()
		log.Info("oa: %v", oa)
		if err = models.AddOauth2(oa); err != nil {
			log.Error("add oauth2 %v", err)
			return
		}
	}
	ctx.Session.Set("userId", u.Id)
	ctx.Session.Set("userName", u.Name)
	ctx.Redirect("/")
}
