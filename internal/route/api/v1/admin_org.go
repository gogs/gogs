package v1

import (
	"gogs.io/gogs/internal/context"
)

func adminCreateOrg(c *context.APIContext, form createOrgRequest) {
	createOrgForUser(c, form, getUserByParams(c))
}
