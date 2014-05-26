// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/social"
)

func extractPath(next string) string {
	n, err := url.Parse(next)
	if err != nil {
		return "/"
	}
	return n.Path
}

func SocialSignIn(ctx *middleware.Context, params martini.Params) {
	if setting.OauthService == nil {
		ctx.Handle(404, "social.SocialSignIn(oauth service not enabled)", nil)
		return
	}

	next := extractPath(ctx.Query("next"))
	name := params["name"]
	connect, ok := social.SocialMap[name]
	if !ok {
		ctx.Handle(404, "social.SocialSignIn(social login not enabled)", errors.New(name))
		return
	}

	code := ctx.Query("code")
	if code == "" {
		// redirect to social login page
		connect.SetRedirectUrl(strings.TrimSuffix(setting.AppUrl, "/") + ctx.Req.URL.Path)
		ctx.Redirect(connect.AuthCodeURL(next))
		return
	}

	// handle call back
	tk, err := connect.Exchange(code)
	if err != nil {
		ctx.Handle(500, "social.SocialSignIn(Exchange)", err)
		return
	}
	next = extractPath(ctx.Query("state"))
	log.Trace("social.SocialSignIn(Got token)")

	ui, err := connect.UserInfo(tk, ctx.Req.URL)
	if err != nil {
		ctx.Handle(500, fmt.Sprintf("social.SocialSignIn(get info from %s)", name), err)
		return
	}
	log.Info("social.SocialSignIn(social login): %s", ui)

	oa, err := models.GetOauth2(ui.Identity)
	switch err {
	case nil:
		ctx.Session.Set("userId", oa.User.Id)
		ctx.Session.Set("userName", oa.User.Name)
	case models.ErrOauth2RecordNotExist:
		raw, _ := json.Marshal(tk)
		oa = &models.Oauth2{
			Uid:      -1,
			Type:     connect.Type(),
			Identity: ui.Identity,
			Token:    string(raw),
		}
		log.Trace("social.SocialSignIn(oa): %v", oa)
		if err = models.AddOauth2(oa); err != nil {
			log.Error("social.SocialSignIn(add oauth2): %v", err) // 501
			return
		}
	case models.ErrOauth2NotAssociated:
		next = "/user/sign_up"
	default:
		ctx.Handle(500, "social.SocialSignIn(GetOauth2)", err)
		return
	}

	ctx.Session.Set("socialId", oa.Id)
	ctx.Session.Set("socialName", ui.Name)
	ctx.Session.Set("socialEmail", ui.Email)
	log.Trace("social.SocialSignIn(social ID): %v", oa.Id)
	ctx.Redirect(next)
}
