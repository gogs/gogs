// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"fmt"
	"path"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers/repo"
)

const (
	FOLLOWERS base.TplName = "user/meta/followers"
	STARS     base.TplName = "user/meta/stars"
)

func GetUserByName(ctx *context.Context, name string) *models.User {
	user, err := models.GetUserByName(name)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByName", nil)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return nil
	}
	return user
}

// GetUserByParams returns user whose name is presented in URL paramenter.
func GetUserByParams(ctx *context.Context) *models.User {
	return GetUserByName(ctx, ctx.Params(":username"))
}

func Profile(ctx *context.Context) {
	uname := ctx.Params(":username")
	// Special handle for FireFox requests favicon.ico.
	if uname == "favicon.ico" {
		ctx.ServeFile(path.Join(setting.StaticRootPath, "public/img/favicon.png"))
		return
	} else if strings.HasSuffix(uname, ".png") {
		ctx.Error(404)
		return
	}

	isShowKeys := false
	if strings.HasSuffix(uname, ".keys") {
		isShowKeys = true
	}

	u := GetUserByName(ctx, strings.TrimSuffix(uname, ".keys"))
	if ctx.Written() {
		return
	}

	// Show SSH keys.
	if isShowKeys {
		ShowSSHKeys(ctx, u.Id)
		return
	}

	if u.IsOrganization() {
		showOrgProfile(ctx)
		return
	}

	ctx.Data["Title"] = u.DisplayName()
	ctx.Data["PageIsUserProfile"] = true
	ctx.Data["Owner"] = u

	orgs, err := models.GetOrgsByUserID(u.Id, ctx.IsSigned && (ctx.User.IsAdmin || ctx.User.Id == u.Id))
	if err != nil {
		ctx.Handle(500, "GetOrgsByUserIDDesc", err)
		return
	}

	ctx.Data["Orgs"] = orgs

	tab := ctx.Query("tab")
	ctx.Data["TabName"] = tab
	switch tab {
	case "activity":
		retrieveFeeds(ctx, u.Id, -1, 0, true)
		if ctx.Written() {
			return
		}
	default:
		var err error
		ctx.Data["Repos"], err = models.GetRepositories(u.Id, ctx.IsSigned && ctx.User.Id == u.Id)
		if err != nil {
			ctx.Handle(500, "GetRepositories", err)
			return
		}
	}

	ctx.HTML(200, PROFILE)
}

func Followers(ctx *context.Context) {
	u := GetUserByParams(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["Title"] = u.DisplayName()
	ctx.Data["CardsTitle"] = ctx.Tr("user.followers")
	ctx.Data["PageIsFollowers"] = true
	ctx.Data["Owner"] = u
	repo.RenderUserCards(ctx, u.NumFollowers, u.GetFollowers, FOLLOWERS)
}

func Following(ctx *context.Context) {
	u := GetUserByParams(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["Title"] = u.DisplayName()
	ctx.Data["CardsTitle"] = ctx.Tr("user.following")
	ctx.Data["PageIsFollowing"] = true
	ctx.Data["Owner"] = u
	repo.RenderUserCards(ctx, u.NumFollowing, u.GetFollowing, FOLLOWERS)
}

func Stars(ctx *context.Context) {

}

func Action(ctx *context.Context) {
	u := GetUserByParams(ctx)
	if ctx.Written() {
		return
	}

	var err error
	switch ctx.Params(":action") {
	case "follow":
		err = models.FollowUser(ctx.User.Id, u.Id)
	case "unfollow":
		err = models.UnfollowUser(ctx.User.Id, u.Id)
	}

	if err != nil {
		ctx.Handle(500, fmt.Sprintf("Action (%s)", ctx.Params(":action")), err)
		return
	}

	redirectTo := ctx.Query("redirect_to")
	if len(redirectTo) == 0 {
		redirectTo = u.HomeLink()
	}
	ctx.Redirect(redirectTo)
}
