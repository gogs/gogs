// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/routers/api/v1/convert"
	"github.com/gogits/gogs/routers/api/v1/user"
)

func listUserOrgs(ctx *context.APIContext, u *models.User, all bool) {
	if err := u.GetOrganizations(all); err != nil {
		ctx.Error(500, "GetOrganizations", err)
		return
	}

	apiOrgs := make([]*api.Organization, len(u.Orgs))
	for i := range u.Orgs {
		apiOrgs[i] = convert.ToOrganization(u.Orgs[i])
	}
	ctx.JSON(200, &apiOrgs)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#list-your-organizations
func ListMyOrgs(ctx *context.APIContext) {
	listUserOrgs(ctx, ctx.User, true)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#list-user-organizations
func ListUserOrgs(ctx *context.APIContext) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}
	listUserOrgs(ctx, u, false)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#get-an-organization
func Get(ctx *context.APIContext) {
	org := user.GetUserByParamsName(ctx, ":orgname")
	if ctx.Written() {
		return
	}
	ctx.JSON(200, convert.ToOrganization(org))
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#edit-an-organization
func Edit(ctx *context.APIContext, form api.EditOrgOption) {
	org := user.GetUserByParamsName(ctx, ":orgname")
	if ctx.Written() {
		return
	}

	if !org.IsOwnedBy(ctx.User.Id) {
		ctx.Status(403)
		return
	}

	org.FullName = form.FullName
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	if err := models.UpdateUser(org); err != nil {
		ctx.Error(500, "UpdateUser", err)
		return
	}

	ctx.JSON(200, convert.ToOrganization(org))
}
