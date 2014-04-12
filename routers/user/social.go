// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"code.google.com/p/goauth2/oauth"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

type SocialConnector interface {
	Identity() string
	Name() string
	Email() string
	TokenString() string
}

type SocialGithub struct {
	data struct {
		Id    int    `json:"id"`
		Name  string `json:"login"`
		Email string `json:"email"`
	}
	Token *oauth.Token
}

func (s *SocialGithub) Identity() string {
	return strconv.Itoa(s.data.Id)
}

func (s *SocialGithub) Name() string {
	return s.data.Name
}

func (s *SocialGithub) Email() string {
	return s.data.Email
}

func (s *SocialGithub) TokenString() string {
	data, _ := json.Marshal(s.Token)
	return string(data)
}

// Github API refer: https://developer.github.com/v3/users/
func (s *SocialGithub) Update() error {
	scope := "https://api.github.com/user"
	transport := &oauth.Transport{
		Token: s.Token,
	}
	log.Debug("update github info")
	r, err := transport.Client().Get(scope)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(&s.data)
}

func extractPath(next string) string {
	n, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	return n.Path
}

// github && google && ...
func SocialSignIn(ctx *middleware.Context) {
	//if base.OauthService != nil && base.OauthService.GitHub.Enabled {
	//}

	var socid int64
	var ok bool
	next := extractPath(ctx.Query("next"))
	log.Debug("social signed check %s", next)
	if socid, ok = ctx.Session.Get("socialId").(int64); ok && socid != 0 {
		// already login
		ctx.Redirect(next)
		log.Info("login soc id: %v", socid)
		return
	}

	config := &oauth.Config{
		ClientId:     base.OauthService.GitHub.ClientId,
		ClientSecret: base.OauthService.GitHub.ClientSecret,
		RedirectURL:  strings.TrimSuffix(base.AppUrl, "/") + ctx.Req.URL.RequestURI(),
		Scope:        base.OauthService.GitHub.Scopes,
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
	}
	transport := &oauth.Transport{
		Config:    config,
		Transport: http.DefaultTransport,
	}
	code := ctx.Query("code")
	if code == "" {
		// redirect to social login page
		ctx.Redirect(config.AuthCodeURL(next))
		return
	}

	// handle call back
	tk, err := transport.Exchange(code)
	if err != nil {
		log.Error("oauth2 handle callback error: %v", err)
		return // FIXME, need error page 501
	}
	next = extractPath(ctx.Query("state"))
	log.Debug("success token: %v", tk)

	gh := &SocialGithub{Token: tk}
	if err = gh.Update(); err != nil {
		// FIXME: handle error page 501
		log.Error("connect with github error: %s", err)
		return
	}
	var soc SocialConnector = gh
	log.Info("login: %s", soc.Name())
	oa, err := models.GetOauth2(soc.Identity())
	switch err {
	case nil:
		ctx.Session.Set("userId", oa.User.Id)
		ctx.Session.Set("userName", oa.User.Name)
	case models.ErrOauth2RecordNotExists:
		oa = &models.Oauth2{}
		oa.Uid = -1
		oa.Type = models.OT_GITHUB
		oa.Token = soc.TokenString()
		oa.Identity = soc.Identity()
		log.Debug("oa: %v", oa)
		if err = models.AddOauth2(oa); err != nil {
			log.Error("add oauth2 %v", err) // 501
			return
		}
	case models.ErrOauth2NotAssociatedWithUser:
		ctx.Session.Set("socialId", oa.Id)
		ctx.Session.Set("socialName", soc.Name())
		ctx.Session.Set("socialEmail", soc.Email())
		ctx.Redirect("/user/sign_up")
		return
	default:
		log.Error(err.Error()) // FIXME: handle error page
		return
	}
	ctx.Session.Set("socialId", oa.Id)
	log.Debug("socialId: %v", oa.Id)
	ctx.Redirect(next)
}
