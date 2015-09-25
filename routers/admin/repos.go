// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	REPOS base.TplName = "admin/repo/list"
)

func Repositories(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.repositories")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminRepositories"] = true

	total := models.CountRepositories()
	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}
	ctx.Data["Page"] = paginater.New(int(total), setting.AdminRepoPagingNum, page, 5)

	repos, err := models.RepositoriesWithUsers(page, setting.AdminRepoPagingNum)
	if err != nil {
		ctx.Handle(500, "RepositoriesWithUsers", err)
		return
	}
	ctx.Data["Repos"] = repos

	ctx.Data["Total"] = total
	ctx.HTML(200, REPOS)
}
