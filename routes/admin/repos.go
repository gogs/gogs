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

func Repos(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.repositories")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminRepositories"] = true

	page := c.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	var (
		repos []*models.Repository
		count int64
		err   error
	)

	keyword := c.Query("q")
	if len(keyword) == 0 {
		repos, err = models.Repositories(page, setting.UI.Admin.RepoPagingNum)
		if err != nil {
			c.Handle(500, "Repositories", err)
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
			c.Handle(500, "SearchRepositoryByName", err)
			return
		}
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), setting.UI.Admin.RepoPagingNum, page, 5)

	if err = models.RepositoryList(repos).LoadAttributes(); err != nil {
		c.Handle(500, "LoadAttributes", err)
		return
	}
	c.Data["Repos"] = repos

	c.HTML(200, REPOS)
}

func DeleteRepo(c *context.Context) {
	repo, err := models.GetRepositoryByID(c.QueryInt64("id"))
	if err != nil {
		c.Handle(500, "GetRepositoryByID", err)
		return
	}

	if err := models.DeleteRepository(repo.MustOwner().ID, repo.ID); err != nil {
		c.Handle(500, "DeleteRepository", err)
		return
	}
	log.Trace("Repository deleted: %s/%s", repo.MustOwner().Name, repo.Name)

	c.Flash.Success(c.Tr("repo.settings.deletion_success"))
	c.JSON(200, map[string]interface{}{
		"redirect": setting.AppSubURL + "/admin/repos?page=" + c.Query("page"),
	})
}
