// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/paginater"
	log "gopkg.in/clog.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/setting"
)

const (
	REPOS = "admin/repo/list"
)

func Repos(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.repositories")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminRepositories"] = true

	page := ctx.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	var (
		repos []*models.Repository
		count int64
		err   error
	)

	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		repos, err = models.Repositories(page, setting.UI.Admin.RepoPagingNum)
		if err != nil {
			ctx.Handle(500, "Repositories", err)
			return
		}
		count = models.CountRepositories(true)
	} else {
		repos, count, err = models.SearchRepositoryByName(&models.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  "id ASC",
			Private:  true,
			Page:     page,
			PageSize: setting.UI.Admin.RepoPagingNum,
		})
		if err != nil {
			ctx.Handle(500, "SearchRepositoryByName", err)
			return
		}
	}
	ctx.Data["Keyword"] = keyword
	ctx.Data["Total"] = count
	ctx.Data["Page"] = paginater.New(int(count), setting.UI.Admin.RepoPagingNum, page, 5)

	if err = models.RepositoryList(repos).LoadAttributes(); err != nil {
		ctx.Handle(500, "LoadAttributes", err)
		return
	}
	ctx.Data["Repos"] = repos

	ctx.HTML(200, REPOS)
}

func DeleteRepo(ctx *context.Context) {
	repo, err := models.GetRepositoryByID(ctx.QueryInt64("id"))
	if err != nil {
		ctx.Handle(500, "GetRepositoryByID", err)
		return
	}

	if err := models.DeleteRepository(repo.MustOwner().ID, repo.ID); err != nil {
		ctx.Handle(500, "DeleteRepository", err)
		return
	}
	log.Trace("Repository deleted: %s/%s", repo.MustOwner().Name, repo.Name)

	ctx.Flash.Success(ctx.Tr("repo.settings.deletion_success"))
	ctx.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubURL + "/admin/repos?page=" + ctx.Query("page"),
	})
}
