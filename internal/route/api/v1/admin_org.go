package v1

import (
	"gogs.io/gogs/internal/context"
)

func CreateOrg(c *context.APIContext, form CreateOrgRequest) {
	CreateOrgForUser(c, form, GetUserByParams(c))
}
