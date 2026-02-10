package v1

import (
	"gogs.io/gogs/internal/context"
)

func AdminCreateRepo(c *context.APIContext, form CreateRepoRequest) {
	owner := GetUserByParams(c)
	if c.Written() {
		return
	}

	CreateUserRepo(c, owner, form)
}
