package admin

import (
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/route/api/v1/org"
	"gogs.io/gogs/internal/route/api/v1/user"
)

func CreateOrg(c *context.APIContext, form org.CreateOrgRequest) {
	org.CreateOrgForUser(c, form, user.GetUserByParams(c))
}
