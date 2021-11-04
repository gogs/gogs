// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/unknwon/paginater"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
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

	orgs, err := db.GetOrgsByUserID(puser.ID, c.IsLogged && (c.User.IsAdmin || c.User.ID == puser.ID))
	if err != nil {
		c.Error(err, "get organizations by user ID")
		return
	}

	c.Data["Orgs"] = orgs

	tab := c.Query("tab")
	c.Data["TabName"] = tab
	switch tab {
	case "activity":
		retrieveFeeds(c, puser.User, -1, true)
		if c.Written() {
			return
		}
	default:
		page := c.QueryInt("page")
		if page <= 0 {
			page = 1
		}

		showPrivate := c.IsLogged && (puser.ID == c.User.ID || c.User.IsAdmin)
		c.Data["Repos"], err = db.GetUserRepositories(&db.UserRepoOptions{
			UserID:   puser.ID,
			Private:  showPrivate,
			Page:     page,
			PageSize: conf.UI.User.RepoPagingNum,
		})
		if err != nil {
			c.Error(err, "get user repositories")
			return
		}

		count := db.CountUserRepositories(puser.ID, showPrivate)
		c.Data["Page"] = paginater.New(int(count), conf.UI.User.RepoPagingNum, page, 5)
	}

	c.Success(PROFILE)
}

func Followers(c *context.Context, puser *context.ParamsUser) {
	c.Title(puser.DisplayName())
	c.PageIs("Followers")
	c.Data["CardsTitle"] = c.Tr("user.followers")
	c.Data["Owner"] = puser
	repo.RenderUserCards(c, puser.NumFollowers, puser.GetFollowers, FOLLOWERS)
}

func Following(c *context.Context, puser *context.ParamsUser) {
	c.Title(puser.DisplayName())
	c.PageIs("Following")
	c.Data["CardsTitle"] = c.Tr("user.following")
	c.Data["Owner"] = puser
	repo.RenderUserCards(c, puser.NumFollowing, puser.GetFollowing, FOLLOWERS)
}

func Stars(c *context.Context) {

}

func Action(c *context.Context, puser *context.ParamsUser) {
	var err error
	switch c.Params(":action") {
	case "follow":
		err = db.FollowUser(c.UserID(), puser.ID)
	case "unfollow":
		err = db.UnfollowUser(c.UserID(), puser.ID)
	}

	if err != nil {
		c.Errorf(err, "action %q", c.Params(":action"))
		return
	}

	redirectTo := c.Query("redirect_to")
	if !tool.IsSameSiteURLPath(redirectTo) {
		redirectTo = puser.HomeLink()
	}
	c.Redirect(redirectTo)
}
