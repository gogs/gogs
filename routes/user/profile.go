// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"path"
	"strings"

	"github.com/Unknwon/paginater"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/routes/repo"
)

const (
	FOLLOWERS = "user/meta/followers"
	STARS     = "user/meta/stars"
)

func GetUserByName(c *context.Context, name string) *models.User {
	user, err := models.GetUserByName(name)
	if err != nil {
		c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
		return nil
	}
	return user
}

// GetUserByParams returns user whose name is presented in URL paramenter.
func GetUserByParams(c *context.Context) *models.User {
	return GetUserByName(c, c.Params(":username"))
}

func Profile(c *context.Context) {
	uname := c.Params(":username")
	// Special handle for FireFox requests favicon.ico.
	if uname == "favicon.ico" {
		c.ServeFile(path.Join(setting.StaticRootPath, "public/img/favicon.png"))
		return
	} else if strings.HasSuffix(uname, ".png") {
		c.Error(404)
		return
	}

	isShowKeys := false
	if strings.HasSuffix(uname, ".keys") {
		isShowKeys = true
	}

	ctxUser := GetUserByName(c, strings.TrimSuffix(uname, ".keys"))
	if c.Written() {
		return
	}

	// Show SSH keys.
	if isShowKeys {
		ShowSSHKeys(c, ctxUser.ID)
		return
	}

	if ctxUser.IsOrganization() {
		showOrgProfile(c)
		return
	}

	c.Data["Title"] = ctxUser.DisplayName()
	c.Data["PageIsUserProfile"] = true
	c.Data["Owner"] = ctxUser

	orgs, err := models.GetOrgsByUserID(ctxUser.ID, c.IsLogged && (c.User.IsAdmin || c.User.ID == ctxUser.ID))
	if err != nil {
		c.Handle(500, "GetOrgsByUserIDDesc", err)
		return
	}

	c.Data["Orgs"] = orgs

	tab := c.Query("tab")
	c.Data["TabName"] = tab
	switch tab {
	case "activity":
		retrieveFeeds(c, ctxUser, -1, true)
		if c.Written() {
			return
		}
	default:
		page := c.QueryInt("page")
		if page <= 0 {
			page = 1
		}

		showPrivate := c.IsLogged && (ctxUser.ID == c.User.ID || c.User.IsAdmin)
		c.Data["Repos"], err = models.GetUserRepositories(&models.UserRepoOptions{
			UserID:   ctxUser.ID,
			Private:  showPrivate,
			Page:     page,
			PageSize: setting.UI.User.RepoPagingNum,
		})
		if err != nil {
			c.Handle(500, "GetRepositories", err)
			return
		}

		count := models.CountUserRepositories(ctxUser.ID, showPrivate)
		c.Data["Page"] = paginater.New(int(count), setting.UI.User.RepoPagingNum, page, 5)
	}

	c.HTML(200, PROFILE)
}

func Followers(c *context.Context) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	c.Data["Title"] = u.DisplayName()
	c.Data["CardsTitle"] = c.Tr("user.followers")
	c.Data["PageIsFollowers"] = true
	c.Data["Owner"] = u
	repo.RenderUserCards(c, u.NumFollowers, u.GetFollowers, FOLLOWERS)
}

func Following(c *context.Context) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	c.Data["Title"] = u.DisplayName()
	c.Data["CardsTitle"] = c.Tr("user.following")
	c.Data["PageIsFollowing"] = true
	c.Data["Owner"] = u
	repo.RenderUserCards(c, u.NumFollowing, u.GetFollowing, FOLLOWERS)
}

func Stars(c *context.Context) {

}

func Action(c *context.Context) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}

	var err error
	switch c.Params(":action") {
	case "follow":
		err = models.FollowUser(c.User.ID, u.ID)
	case "unfollow":
		err = models.UnfollowUser(c.User.ID, u.ID)
	}

	if err != nil {
		c.Handle(500, fmt.Sprintf("Action (%s)", c.Params(":action")), err)
		return
	}

	redirectTo := c.Query("redirect_to")
	if len(redirectTo) == 0 {
		redirectTo = u.HomeLink()
	}
	c.Redirect(redirectTo)
}
