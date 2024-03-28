// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/unknwon/paginater"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/tool"
)

const (
	FOLLOWERS = "user/meta/followers"
	STARS     = "user/meta/stars"
)

func Profile(c *context.Context, puser *context.ParamsUser) {
	isShowKeys := false
	if strings.HasSuffix(c.Params(":username"), ".keys") {
		isShowKeys = true
	}

	// Show SSH keys.
	if isShowKeys {
		ShowSSHKeys(c, puser.ID)
		return
	}

	if puser.IsOrganization() {
		showOrgProfile(c)
		return
	}

	c.Title(puser.DisplayName())
	c.PageIs("UserProfile")
	c.Data["Owner"] = puser

	orgs, err := database.GetOrgsByUserID(puser.ID, c.IsLogged && (c.User.IsAdmin || c.User.ID == puser.ID))
	if err != nil {
		c.Error(err, "get organizations by user ID")
		return
	}

	c.Data["Orgs"] = orgs

	tab := c.Query("tab")
	c.Data["TabName"] = tab
	switch tab {
	case "activity":
		retrieveFeeds(c, puser.User, c.UserID(), true)
		if c.Written() {
			return
		}
	default:
		page := c.QueryInt("page")
		if page <= 0 {
			page = 1
		}

		showPrivate := c.IsLogged && (puser.ID == c.User.ID || c.User.IsAdmin)
		c.Data["Repos"], err = database.GetUserRepositories(&database.UserRepoOptions{
			UserID:   puser.ID,
			Private:  showPrivate,
			Page:     page,
			PageSize: conf.UI.User.RepoPagingNum,
		})
		if err != nil {
			c.Error(err, "get user repositories")
			return
		}

		count := database.CountUserRepositories(puser.ID, showPrivate)
		c.Data["Page"] = paginater.New(int(count), conf.UI.User.RepoPagingNum, page, 5)
	}

	c.Success(PROFILE)
}

func Followers(c *context.Context, puser *context.ParamsUser) {
	c.Title(puser.DisplayName())
	c.PageIs("Followers")
	c.Data["CardsTitle"] = c.Tr("user.followers")
	c.Data["Owner"] = puser
	repo.RenderUserCards(
		c,
		puser.NumFollowers,
		func(page int) ([]*database.User, error) {
			return database.Handle.Users().ListFollowers(c.Req.Context(), puser.ID, page, database.ItemsPerPage)
		},
		FOLLOWERS,
	)
}

func Following(c *context.Context, puser *context.ParamsUser) {
	c.Title(puser.DisplayName())
	c.PageIs("Following")
	c.Data["CardsTitle"] = c.Tr("user.following")
	c.Data["Owner"] = puser
	repo.RenderUserCards(
		c,
		puser.NumFollowing,
		func(page int) ([]*database.User, error) {
			return database.Handle.Users().ListFollowings(c.Req.Context(), puser.ID, page, database.ItemsPerPage)
		},
		FOLLOWERS,
	)
}

func Stars(_ *context.Context) {
}

func Action(c *context.Context, puser *context.ParamsUser) {
	var err error
	switch c.Params(":action") {
	case "follow":
		err = database.Handle.Users().Follow(c.Req.Context(), c.UserID(), puser.ID)
	case "unfollow":
		err = database.Handle.Users().Unfollow(c.Req.Context(), c.UserID(), puser.ID)
	}

	if err != nil {
		c.Errorf(err, "action %q", c.Params(":action"))
		return
	}

	redirectTo := c.Query("redirect_to")
	if !tool.IsSameSiteURLPath(redirectTo) {
		redirectTo = puser.HomeURLPath()
	}
	c.Redirect(redirectTo)
}
