// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package route

import (
	gocontext "context"
	"fmt"
	"net/http"

	"github.com/go-macaron/i18n"
	"github.com/unknwon/paginater"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/user"
)

const (
	HOME                  = "home"
	EXPLORE_REPOS         = "explore/repos"
	EXPLORE_USERS         = "explore/users"
	EXPLORE_ORGANIZATIONS = "explore/organizations"
)

func Home(c *context.Context) {
	if c.IsLogged {
		if !c.User.IsActive && conf.Auth.RequireEmailConfirmation {
			c.Data["Title"] = c.Tr("auth.active_your_account")
			c.Success(user.ACTIVATE)
		} else {
			user.Dashboard(c)
		}
		return
	}

	// Check auto-login.
	uname := c.GetCookie(conf.Security.CookieUsername)
	if uname != "" {
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
	repos, count, err := database.SearchRepositoryByName(&database.SearchRepoOptions{
		Keyword:  keyword,
		UserID:   c.UserID(),
		OrderBy:  "updated_unix DESC",
		Page:     page,
		PageSize: conf.UI.ExplorePagingNum,
	})
	if err != nil {
		c.Error(err, "search repository by name")
		return
	}
	c.Data["Keyword"] = keyword
	c.Data["Total"] = count
	c.Data["Page"] = paginater.New(int(count), conf.UI.ExplorePagingNum, page, 5)

	if err = database.RepositoryList(repos).LoadAttributes(); err != nil {
		c.Error(err, "load attributes")
		return
	}
	c.Data["Repos"] = repos

	c.Success(EXPLORE_REPOS)
}

type UserSearchOptions struct {
	Type     database.UserType
	Counter  func(ctx gocontext.Context) int64
	Ranger   func(ctx gocontext.Context, page, pageSize int) ([]*database.User, error)
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
		users []*database.User
		count int64
		err   error
	)

	keyword := c.Query("q")
	if keyword == "" {
		users, err = opts.Ranger(c.Req.Context(), page, opts.PageSize)
		if err != nil {
			c.Error(err, "ranger")
			return
		}
		count = opts.Counter(c.Req.Context())
	} else {
		search := database.Users.SearchByName
		if opts.Type == database.UserTypeOrganization {
			search = database.Handle.Organizations().SearchByName
		}
		users, count, err = search(c.Req.Context(), keyword, page, opts.PageSize, opts.OrderBy)
		if err != nil {
			c.Error(err, "search by name")
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
		Type:     database.UserTypeIndividual,
		Counter:  database.Users.Count,
		Ranger:   database.Users.List,
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
		Type: database.UserTypeOrganization,
		Counter: func(gocontext.Context) int64 {
			return database.CountOrganizations()
		},
		Ranger: func(_ gocontext.Context, page, pageSize int) ([]*database.User, error) {
			return database.Organizations(page, pageSize)
		},
		PageSize: conf.UI.ExplorePagingNum,
		OrderBy:  "updated_unix DESC",
		TplName:  EXPLORE_ORGANIZATIONS,
	})
}

func NotFound(c *macaron.Context, l i18n.Locale) {
	c.Data["Title"] = l.Tr("status.page_not_found")
	c.HTML(http.StatusNotFound, fmt.Sprintf("status/%d", http.StatusNotFound))
}
