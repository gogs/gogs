// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"fmt"

	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers/user"
)

const (
	HOME          base.TplName = "home"
	EXPLORE_REPOS base.TplName = "explore/repos"
	EXPLORE_USERS base.TplName = "explore/users"
)

func Home(ctx *context.Context) {
	if ctx.IsSigned {
		if !ctx.User.IsActive && setting.Service.RegisterEmailConfirm {
			ctx.Data["Title"] = ctx.Tr("auth.active_your_account")
			ctx.HTML(200, user.ACTIVATE)
		} else {
			user.Dashboard(ctx)
		}
		return
	}

	// Check auto-login.
	uname := ctx.GetCookie(setting.CookieUserName)
	if len(uname) != 0 {
		ctx.Redirect(setting.AppSubUrl + "/user/login")
		return
	}

	ctx.Data["PageIsHome"] = true
	ctx.HTML(200, HOME)
}

func RenderRepoSearch(ctx *context.Context,
	counter func() int64, ranger func(int, int) ([]*models.Repository, error),
	pagingNum int, orderBy string, tplName base.TplName) {
	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var (
		repos []*models.Repository
		count int64
		err   error
	)

	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		repos, err = ranger(page, pagingNum)
		if err != nil {
			ctx.Handle(500, "ranger", err)
			return
		}
		count = counter()
	} else {
		repos, count, err = models.SearchRepositoryByName(&models.SearchRepoOptions{
			Keyword:  keyword,
			OrderBy:  orderBy,
			Page:     page,
			PageSize: pagingNum,
		})
		if err != nil {
			ctx.Handle(500, "SearchRepositoryByName", err)
			return
		}
	}
	ctx.Data["Keyword"] = keyword
	ctx.Data["Total"] = count
	ctx.Data["Page"] = paginater.New(int(count), pagingNum, page, 5)

	for _, repo := range repos {
		if err = repo.GetOwner(); err != nil {
			ctx.Handle(500, "GetOwner", fmt.Errorf("%d: %v", repo.ID, err))
			return
		}
	}
	ctx.Data["Repos"] = repos

	ctx.HTML(200, tplName)
}

func ExploreRepos(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("explore")
	ctx.Data["PageIsExplore"] = true
	ctx.Data["PageIsExploreRepositories"] = true

	RenderRepoSearch(ctx, models.CountPublicRepositories, models.GetRecentUpdatedRepositories,
		setting.ExplorePagingNum, "updated_unix DESC", EXPLORE_REPOS)
}

func RenderUserSearch(ctx *context.Context, userType models.UserType,
	counter func() int64, ranger func(int, int) ([]*models.User, error),
	pagingNum int, orderBy string, tplName base.TplName) {
	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}

	var (
		users []*models.User
		count int64
		err   error
	)

	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		users, err = ranger(page, pagingNum)
		if err != nil {
			ctx.Handle(500, "ranger", err)
			return
		}
		count = counter()
	} else {
		users, count, err = models.SearchUserByName(&models.SearchUserOptions{
			Keyword:  keyword,
			Type:     userType,
			OrderBy:  orderBy,
			Page:     page,
			PageSize: pagingNum,
		})
		if err != nil {
			ctx.Handle(500, "SearchUserByName", err)
			return
		}
	}
	ctx.Data["Keyword"] = keyword
	ctx.Data["Total"] = count
	ctx.Data["Page"] = paginater.New(int(count), pagingNum, page, 5)
	ctx.Data["Users"] = users

	ctx.HTML(200, tplName)
}

func ExploreUsers(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("explore")
	ctx.Data["PageIsExplore"] = true
	ctx.Data["PageIsExploreUsers"] = true

	RenderUserSearch(ctx, models.USER_TYPE_INDIVIDUAL, models.CountUsers, models.Users,
		setting.ExplorePagingNum, "updated_unix DESC", EXPLORE_USERS)
}

func NotFound(ctx *context.Context) {
	ctx.Data["Title"] = "Page Not Found"
	ctx.Handle(404, "home.NotFound", nil)
}
