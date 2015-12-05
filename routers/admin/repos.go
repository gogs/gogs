// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	REPOS base.TplName = "admin/repo/list"
)

func Repos(ctx *middleware.Context) {
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

func DeleteRepo(ctx *middleware.Context) {
	repo, err := models.GetRepositoryByID(ctx.QueryInt64("id"))
	if err != nil {
		ctx.Handle(500, "GetRepositoryByID", err)
		return
	}

	if err := models.DeleteRepository(repo.MustOwner().Id, repo.ID); err != nil {
		ctx.Handle(500, "DeleteRepository", err)
		return
	}
	log.Trace("Repository deleted: %s/%s", repo.MustOwner().Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.deletion_success"))
	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubUrl + "/admin/repos",
	})
}
