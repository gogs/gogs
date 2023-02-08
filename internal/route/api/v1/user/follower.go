// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func responseApiUsers(c *context.APIContext, users []*db.User) {
	apiUsers := make([]*api.User, len(users))
	for i := range users {
		apiUsers[i] = users[i].APIFormat()
	}
	c.JSONSuccess(&apiUsers)
}

func listUserFollowers(c *context.APIContext, u *db.User) {
	users, err := db.Users.ListFollowers(c.Req.Context(), u.ID, c.QueryInt("page"), db.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followers")
		return
	}
	responseApiUsers(c, users)
}

func ListMyFollowers(c *context.APIContext) {
	listUserFollowers(c, c.User)
}

func ListFollowers(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowers(c, u)
}

func listUserFollowing(c *context.APIContext, u *db.User) {
	users, err := db.Users.ListFollowings(c.Req.Context(), u.ID, c.QueryInt("page"), db.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followings")
		return
	}
	responseApiUsers(c, users)
}

func ListMyFollowing(c *context.APIContext) {
	listUserFollowing(c, c.User)
}

func ListFollowing(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowing(c, u)
}

func checkUserFollowing(c *context.APIContext, u *db.User, followID int64) {
	if db.Users.IsFollowing(c.Req.Context(), u.ID, followID) {
		c.NoContent()
	} else {
		c.NotFound()
	}
}

func CheckMyFollowing(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	checkUserFollowing(c, c.User, target.ID)
}

func CheckFollowing(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	target := GetUserByParamsName(c, ":target")
	if c.Written() {
		return
	}
	checkUserFollowing(c, u, target.ID)
}

func Follow(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := db.Users.Follow(c.Req.Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "follow user")
		return
	}
	c.NoContent()
}

func Unfollow(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := db.Users.Unfollow(c.Req.Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "unfollow user")
		return
	}
	c.NoContent()
}
