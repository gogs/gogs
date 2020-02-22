// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package route

import (
	"github.com/unknwon/paginater"
	user2 "gogs.io/gogs/internal/route/user"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

const (
	HOME                  = "home"
	EXPLORE_REPOS         = "explore/repos"
	EXPLORE_USERS         = "explore/users"
	EXPLORE_ORGANIZATIONS = "explore/organizations"
)

func Home(c *context.Context) {
	if c.IsLogged {
		if !c.User.IsActive && conf.Service.RegisterEmailConfirm {
			c.Data["Title"] = c.Tr("auth.active_your_account")
			c.Success(user2.ACTIVATE)
		} else {
			user2.Dashboard(c)
		}
		return
	}

	// Check auto-login.
	uname := c.GetCookie(conf.Security.CookieUsername)
	if len(uname) != 0 {
		c.Redirect(conf.Server.Subpath + "/user/login")
		return
	}

	c.Data["PageIsHome"] = true
	c.Success(HOME)
}

func ExploreRepos(c *context.Context) {
	c.Data["Title"] = c.Tr("explore")
	c.Data["PageIsExplore"] = true
	c.Data["PageIsExploreRepositories"] = true

	page := c.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	keyword := c.Query("q")
	repos, count, err := db.SearchRepositoryByName(&db.SearchRepoOptions{
		Keyword:  keyword,
		UserID:   c.UserID(),
		OrderBy:  "updated_unix DESC",
		Page:     page,
		PageSize: conf.UI.ExplorePagingNum,
	})
	if err != nil {
		c.ServerError("SearchRepositoryByName", err)
		return
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), conf.UI.ExplorePagingNum, page, 5)

	if err = db.RepositoryList(repos).LoadAttributes(); err != nil {
		c.ServerError("RepositoryList.LoadAttributes", err)
		return
	}
	c.Data["Repos"] = repos

	c.Success(EXPLORE_REPOS)
}

type UserSearchOptions struct {
	Type     db.UserType
	Counter  func() int64
	Ranger   func(int, int) ([]*db.User, error)
	PageSize int
	OrderBy  string
	TplName  string
}

func RenderUserSearch(c *context.Context, opts *UserSearchOptions) {
	page := c.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var (
		users []*db.User
		count int64
		err   error
	)

	keyword := c.Query("q")
	if len(keyword) == 0 {
		users, err = opts.Ranger(page, opts.PageSize)
		if err != nil {
			c.ServerError("Ranger", err)
			return
		}
		count = opts.Counter()
	} else {
		users, count, err = db.SearchUserByName(&db.SearchUserOptions{
			Keyword:  keyword,
			Type:     opts.Type,
			OrderBy:  opts.OrderBy,
			Page:     page,
			PageSize: opts.PageSize,
		})
		if err != nil {
			c.ServerError("SearchUserByName", err)
			return
		}
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), opts.PageSize, page, 5)
	c.Data["Users"] = users

	c.Success(opts.TplName)
}

func ExploreUsers(c *context.Context) {
	c.Data["Title"] = c.Tr("explore")
	c.Data["PageIsExplore"] = true
	c.Data["PageIsExploreUsers"] = true

	RenderUserSearch(c, &UserSearchOptions{
		Type:     db.USER_TYPE_INDIVIDUAL,
		Counter:  db.CountUsers,
		Ranger:   db.Users,
		PageSize: conf.UI.ExplorePagingNum,
		OrderBy:  "updated_unix DESC",
		TplName:  EXPLORE_USERS,
	})
}

func ExploreOrganizations(c *context.Context) {
	c.Data["Title"] = c.Tr("explore")
	c.Data["PageIsExplore"] = true
	c.Data["PageIsExploreOrganizations"] = true

	RenderUserSearch(c, &UserSearchOptions{
		Type:     db.USER_TYPE_ORGANIZATION,
		Counter:  db.CountOrganizations,
		Ranger:   db.Organizations,
		PageSize: conf.UI.ExplorePagingNum,
		OrderBy:  "updated_unix DESC",
		TplName:  EXPLORE_ORGANIZATIONS,
	})
}

func NotFound(c *context.Context) {
	c.Data["Title"] = "Page Not Found"
	c.NotFound()
}
