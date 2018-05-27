// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
)

func responseApiUsers(c *context.APIContext, users []*models.User) {
	apiUsers := make([]*api.User, len(users))
	for i := range users {
		apiUsers[i] = users[i].APIFormat()
	}
	c.JSON(200, &apiUsers)
}

func listUserFollowers(c *context.APIContext, u *models.User) {
	users, err := u.GetFollowers(c.QueryInt("page"))
	if err != nil {
		c.Error(500, "GetUserFollowers", err)
		return
	}
	responseApiUsers(c, users)
}

func ListMyFollowers(c *context.APIContext) {
	listUserFollowers(c, c.User)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#list-followers-of-a-user
func ListFollowers(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowers(c, u)
}

func listUserFollowing(c *context.APIContext, u *models.User) {
	users, err := u.GetFollowing(c.QueryInt("page"))
	if err != nil {
		c.Error(500, "GetFollowing", err)
		return
	}
	responseApiUsers(c, users)
}

func ListMyFollowing(c *context.APIContext) {
	listUserFollowing(c, c.User)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#list-users-followed-by-another-user
func ListFollowing(c *context.APIContext) {
	u := GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowing(c, u)
}

func checkUserFollowing(c *context.APIContext, u *models.User, followID int64) {
	if u.IsFollowing(followID) {
		c.Status(204)
	} else {
		c.Status(404)
	}
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#check-if-you-are-following-a-user
func CheckMyFollowing(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	checkUserFollowing(c, c.User, target.ID)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#check-if-one-user-follows-another
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

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#follow-a-user
func Follow(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := models.FollowUser(c.User.ID, target.ID); err != nil {
		c.Error(500, "FollowUser", err)
		return
	}
	c.Status(204)
}

// https://github.com/gogs/go-gogs-client/wiki/Users-Followers#unfollow-a-user
func Unfollow(c *context.APIContext) {
	target := GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := models.UnfollowUser(c.User.ID, target.ID); err != nil {
		c.Error(500, "UnfollowUser", err)
		return
	}
	c.Status(204)
}
