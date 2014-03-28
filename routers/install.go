// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"errors"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Install(ctx *middleware.Context) {
	if base.InstallLock {
		ctx.Handle(404, "install.Install", errors.New("Installation is prohibited"))
		return
	}

	ctx.Data["Title"] = "Install"
	ctx.Data["DbCfg"] = models.DbCfg
	ctx.Data["RepoRootPath"] = base.RepoRootPath
	ctx.Data["RunUser"] = base.RunUser
	ctx.Data["AppUrl"] = base.AppUrl
	ctx.Data["PageIsInstall"] = true

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "install")
		return
	}
}
