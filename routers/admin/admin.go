// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"strings"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Admin Dashboard"
	ctx.Data["PageIsDashboard"] = true
	ctx.Data["Stats"] = models.GetStatistic()
	ctx.HTML(200, "admin/dashboard")
}

func Users(ctx *middleware.Context) {
	ctx.Data["Title"] = "User Management"
	ctx.Data["PageIsUsers"] = true

	var err error
	ctx.Data["Users"], err = models.GetUsers(100, 0)
	if err != nil {
		ctx.Handle(200, "admin.Users", err)
		return
	}
	ctx.HTML(200, "admin/users")
}

func Repositories(ctx *middleware.Context) {
	ctx.Data["Title"] = "Repository Management"
	ctx.Data["PageIsRepos"] = true

	var err error
	ctx.Data["Repos"], err = models.GetRepos(100, 0)
	if err != nil {
		ctx.Handle(200, "admin.Repositories", err)
		return
	}
	ctx.HTML(200, "admin/repos")
}

func Config(ctx *middleware.Context) {
	ctx.Data["Title"] = "Server Configuration"
	ctx.Data["PageIsConfig"] = true

	ctx.Data["AppUrl"] = base.AppUrl
	ctx.Data["Domain"] = base.Domain
	ctx.Data["RunUser"] = base.RunUser
	ctx.Data["RunMode"] = strings.Title(martini.Env)
	ctx.Data["RepoRootPath"] = base.RepoRootPath

	ctx.Data["Service"] = base.Service

	ctx.Data["DbCfg"] = models.DbCfg

	ctx.Data["MailerEnabled"] = false
	if base.MailService != nil {
		ctx.Data["MailerEnabled"] = true
		ctx.Data["Mailer"] = base.MailService
	}

	ctx.Data["CacheAdapter"] = base.CacheAdapter
	ctx.Data["CacheConfig"] = base.CacheConfig

	ctx.Data["LogMode"] = base.LogMode
	ctx.Data["LogConfig"] = base.LogConfig

	ctx.HTML(200, "admin/config")
}
