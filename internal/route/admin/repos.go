// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/unknwon/paginater"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
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
		repos []*db.Repository
		count int64
		err   error
	)

	keyword := c.Query("q")
	if keyword == "" {
		repos, err = db.Repositories(page, conf.UI.Admin.RepoPagingNum)
		if err != nil {
			c.Error(err, "list repositories")
			return
		}
		count = db.CountRepositories(true)
	} else {
		repos, count, err = db.SearchRepositoryByName(&db.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  "id ASC",
			Private:  true,
			Page:     page,
			PageSize: conf.UI.Admin.RepoPagingNum,
		})
		if err != nil {
			c.Error(err, "search repository by name")
			return
		}
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), conf.UI.Admin.RepoPagingNum, page, 5)

	if err = db.RepositoryList(repos).LoadAttributes(); err != nil {
		c.Error(err, "load attributes")
		return
	}
	c.Data["Repos"] = repos

	c.Success(REPOS)
}

func DeleteRepo(c *context.Context) {
	repo, err := db.GetRepositoryByID(c.QueryInt64("id"))
	if err != nil {
		c.Error(err, "get repository by ID")
		return
	}

	if err := db.DeleteRepository(repo.MustOwner().ID, repo.ID); err != nil {
		c.Error(err, "delete repository")
		return
	}
	log.Trace("Repository deleted: %s/%s", repo.MustOwner().Name, repo.Name)

	c.Flash.Success(c.Tr("repo.settings.deletion_success"))
	c.JSONSuccess(map[string]interface{}{
		"redirect": conf.Server.Subpath + "/admin/repos?page=" + c.Query("page"),
	})
}
