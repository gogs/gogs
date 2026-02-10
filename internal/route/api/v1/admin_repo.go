package v1

import (
	"gogs.io/gogs/internal/context"
)

func adminCreateRepo(c *context.APIContext, form createRepoRequest) {
	owner := getUserByParams(c)
	if c.Written() {
		return
	}

	createUserRepo(c, owner, form)
}
