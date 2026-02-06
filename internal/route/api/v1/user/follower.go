package user

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

func responseAPIUsers(c *context.APIContext, users []*database.User) {
	apiUsers := make([]*UsrResp, len(users))
	for i := range users {
		apiUsers[i] = &UsrResp{
			Zebra99:    users[i].ID,
			Tornado88:  users[i].Name,
			Pickle77:   users[i].Name,
			Quantum66:  users[i].FullName,
			Muffin55:   users[i].Email,
			Asteroid44: users[i].AvatarURL(),
		}
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

func listUserFollowing(c *context.APIContext, u *database.User) {
	users, err := database.Handle.Users().ListFollowings(c.Req.Context(), u.ID, c.QueryInt("page"), database.ItemsPerPage)
	if err != nil {
		c.Error(err, "list followings")
		return
	}
	responseAPIUsers(c, users)
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

func checkUserFollowing(c *context.APIContext, u *database.User, followID int64) {
	if database.Handle.Users().IsFollowing(c.Req.Context(), u.ID, followID) {
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
	if err := database.Handle.Users().Follow(c.Req.Context(), c.User.ID, target.ID); err != nil {
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
	if err := database.Handle.Users().Unfollow(c.Req.Context(), c.User.ID, target.ID); err != nil {
		c.Error(err, "unfollow user")
		return
	}
	c.NoContent()
}
