package user

import (
	"strings"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/repo"
	"gogs.io/gogs/internal/urlx"
)

const (
	FOLLOWERS = "user/meta/followers"
	STARS     = "user/meta/stars"
)

func Profile(c *context.Context, puser *context.ParamsUser) {
	// Show SSH keys.
	if strings.HasSuffix(c.Params(":username"), ".keys") {
		ShowSSHKeys(c, puser.ID)
		return
	}

	if puser.IsOrganization() {
		showOrgProfile(c)
		return
	}

	c.ServeWeb()
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
	if !urlx.IsSameSite(redirectTo) {
		redirectTo = puser.HomeURLPath()
	}
	c.Redirect(redirectTo)
}
