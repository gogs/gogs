// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/pkg/context"
	"gogs.io/gogs/routes/api/v1/org"
	"gogs.io/gogs/routes/api/v1/user"
)

func CreateOrg(c *context.APIContext, form api.CreateOrgOption) {
	org.CreateOrgForUser(c, form, user.GetUserByParams(c))
}
