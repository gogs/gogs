// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers/user"
)

const (
	HOME base.TplName = "home"
)

func Home(ctx *middleware.Context) {
	if ctx.IsSigned {
		user.Dashboard(ctx)
		return
	}

	// Check auto-login.
	userName := ctx.GetCookie(setting.CookieUserName)
	if len(userName) != 0 {
		ctx.Redirect("/user/login")
		return
	}

	ctx.Data["PageIsHome"] = true

	// Show recent updated repositories for new visitors.
	repos, err := models.GetRecentUpdatedRepositories()
	if err != nil {
		ctx.Handle(500, "dashboard.Home(GetRecentUpdatedRepositories)", err)
		return
	}

	for _, repo := range repos {
		if err = repo.GetOwner(); err != nil {
			ctx.Handle(500, "dashboard.Home(GetOwner)", err)
			return
		}
	}
	ctx.Data["Repos"] = repos
	ctx.HTML(200, HOME)
}

func NotFound(ctx *middleware.Context) {
	ctx.Data["Title"] = "Page Not Found"
	ctx.Data["PageIsNotFound"] = true
	ctx.Handle(404, "home.NotFound", nil)
}
