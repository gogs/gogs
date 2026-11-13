package user

import (
	"strings"

	"gogs.io/gogs/internal/context"
)

func Profile(c *context.Context, puser *context.ParamsUser) {
	// Show SSH keys.
	if strings.HasSuffix(c.Params(":username"), ".keys") {
		ShowSSHKeys(c, puser.ID)
		return
	}

	c.ServeWeb()
}
