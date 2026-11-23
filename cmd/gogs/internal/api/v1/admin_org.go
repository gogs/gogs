package v1

import (
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

func adminCreateOrg(c *apiContext, links database.LinkOptions, repository conf.RepositoryOpts, form createOrgRequest) {
	createOrgForUser(c, links, repository.Root, form, getUserByParams(c))
}
