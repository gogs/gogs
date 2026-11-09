package v1

import (
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func responseAPIUsers(c *apiContext, users []*database.User) {
	apiUsers := make([]*types.User, len(users))
	for i := range users {
		apiUsers[i] = toUser(users[i])
	}
	c.JSONSuccess(&apiUsers)
}

func listUserFollowers(c *apiContext, u *database.User) {
	users, err := database.Handle.Users().ListFollowers(c.Request().Context(), u.ID, c.QueryInt("page"), database.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followers")
		return
	}
	responseAPIUsers(c, users)
}

func listMyFollowers(c *apiContext) {
	listUserFollowers(c, c.User)
}

func listFollowers(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowers(c, u)
}

func listUserFollowing(c *apiContext, u *database.User) {
	users, err := database.Handle.Users().ListFollowings(c.Request().Context(), u.ID, c.QueryInt("page"), database.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followings")
		return
	}
	responseAPIUsers(c, users)
}

func listMyFollowing(c *apiContext) {
	listUserFollowing(c, c.User)
}

func listFollowing(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowing(c, u)
}

func checkUserFollowing(c *apiContext, u *database.User, followID int64) {
	if database.Handle.Users().IsFollowing(c.Request().Context(), u.ID, followID) {
		c.NoContent()
	} else {
		c.NotFound()
	}
}

func checkMyFollowing(c *apiContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	checkUserFollowing(c, c.User, target.ID)
}

func checkFollowing(c *apiContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	target := getUserByParamsName(c, "target")
	if c.Written() {
		return
	}
	checkUserFollowing(c, u, target.ID)
}

func follow(c *apiContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := database.Handle.Users().Follow(c.Request().Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "follow user")
		return
	}
	c.NoContent()
}

func unfollow(c *apiContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := database.Handle.Users().Unfollow(c.Request().Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "unfollow user")
		return
	}
	c.NoContent()
}
