package v1

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

func responseAPIUsers(c *context.APIContext, users []*database.User) {
	apiUsers := make([]*types.User, len(users))
	for i := range users {
		apiUsers[i] = toUser(users[i])
	}
	c.JSONSuccess(&apiUsers)
}

func listUserFollowers(c *context.APIContext, u *database.User) {
	users, err := database.Handle.Users().ListFollowers(c.Req.Context(), u.ID, c.QueryInt("page"), database.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followers")
		return
	}
	responseAPIUsers(c, users)
}

func listMyFollowers(c *context.APIContext) {
	listUserFollowers(c, c.User)
}

func listFollowers(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowers(c, u)
}

func listUserFollowing(c *context.APIContext, u *database.User) {
	users, err := database.Handle.Users().ListFollowings(c.Req.Context(), u.ID, c.QueryInt("page"), database.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followings")
		return
	}
	responseAPIUsers(c, users)
}

func listMyFollowing(c *context.APIContext) {
	listUserFollowing(c, c.User)
}

func listFollowing(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	listUserFollowing(c, u)
}

func checkUserFollowing(c *context.APIContext, u *database.User, followID int64) {
	if database.Handle.Users().IsFollowing(c.Req.Context(), u.ID, followID) {
		c.NoContent()
	} else {
		c.NotFound()
	}
}

func checkMyFollowing(c *context.APIContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	checkUserFollowing(c, c.User, target.ID)
}

func checkFollowing(c *context.APIContext) {
	u := getUserByParams(c)
	if c.Written() {
		return
	}
	target := getUserByParamsName(c, ":target")
	if c.Written() {
		return
	}
	checkUserFollowing(c, u, target.ID)
}

func follow(c *context.APIContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := database.Handle.Users().Follow(c.Req.Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "follow user")
		return
	}
	c.NoContent()
}

func unfollow(c *context.APIContext) {
	target := getUserByParams(c)
	if c.Written() {
		return
	}
	if err := database.Handle.Users().Unfollow(c.Req.Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "unfollow user")
		return
	}
	c.NoContent()
}
