package user

import (
	"strings"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/urlx"
)

func Profile(c *context.Context, puser *context.ParamsUser) {
	// Show SSH keys.
	if strings.HasSuffix(c.Params(":username"), ".keys") {
		ShowSSHKeys(c, puser.ID)
		return
	}

	c.ServeWeb()
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
