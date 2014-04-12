// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"code.google.com/p/goauth2/oauth"

	"github.com/go-martini/martini"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

type BasicUserInfo struct {
	Identity string
	Name     string
	Email    string
}

type SocialConnector interface {
	Type() int
	SetRedirectUrl(string)
	UserInfo(*oauth.Token, *url.URL) (*BasicUserInfo, error)

	AuthCodeURL(string) string
	Exchange(string) (*oauth.Token, error)
}

func extractPath(next string) string {
	n, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	return n.Path
}

var (
	SocialBaseUrl = "/user/login"
	SocialMap     = make(map[string]SocialConnector)
)

// github && google && ...
func SocialSignIn(params martini.Params, ctx *middleware.Context) {
	if base.OauthService == nil || !base.OauthService.GitHub.Enabled {
		ctx.Handle(404, "social login not enabled", nil)
		return
	}
	next := extractPath(ctx.Query("next"))
	name := params["name"]
	connect, ok := SocialMap[name]
	if !ok {
		ctx.Handle(404, "social login", nil)
		return
	}
	code := ctx.Query("code")
	if code == "" {
		// redirect to social login page
		connect.SetRedirectUrl(strings.TrimSuffix(base.AppUrl, "/") + ctx.Req.URL.Host + ctx.Req.URL.Path)
		ctx.Redirect(connect.AuthCodeURL(next))
		return
	}

	// handle call back
	tk, err := connect.Exchange(code) // exchange for token
	if err != nil {
		log.Error("oauth2 handle callback error: %v", err)
		ctx.Handle(500, "exchange code error", nil)
		return
	}
	next = extractPath(ctx.Query("state"))
	log.Trace("success get token")

	ui, err := connect.UserInfo(tk, ctx.Req.URL)
	if err != nil {
		ctx.Handle(500, fmt.Sprintf("get infomation from %s error: %v", name, err), nil)
		log.Error("social connect error: %s", err)
		return
	}
	log.Info("social login: %s", ui)
	oa, err := models.GetOauth2(ui.Identity)
	switch err {
	case nil:
		ctx.Session.Set("userId", oa.User.Id)
		ctx.Session.Set("userName", oa.User.Name)
	case models.ErrOauth2RecordNotExists:
		oa = &models.Oauth2{}
		raw, _ := json.Marshal(tk) // json encode
		oa.Token = string(raw)
		oa.Uid = -1
		oa.Type = connect.Type()
		oa.Identity = ui.Identity
		log.Trace("oa: %v", oa)
		if err = models.AddOauth2(oa); err != nil {
			log.Error("add oauth2 %v", err) // 501
			return
		}
	case models.ErrOauth2NotAssociatedWithUser:
		next = "/user/sign_up"
	default:
		log.Error("other error: %v", err)
		ctx.Handle(500, err.Error(), nil)
		return
	}
	ctx.Session.Set("socialId", oa.Id)
	ctx.Session.Set("socialName", ui.Name)
	ctx.Session.Set("socialEmail", ui.Email)
	log.Trace("socialId: %v", oa.Id)
	ctx.Redirect(next)
}
