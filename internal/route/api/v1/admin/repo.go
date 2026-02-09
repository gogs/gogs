package admin

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/route/api/v1/repo"
	"gogs.io/gogs/internal/route/api/v1/user"
)

func CreateRepo(c *context.APIContext, form repo.CreateRepoRequest) {
	owner := user.GetUserByParams(c)
	if c.Written() {
		return
	}

	repo.CreateUserRepo(c, owner, form)
}
