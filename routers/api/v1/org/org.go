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

func listUserOrgs(ctx *context.Context, u *models.User, all bool) {
	if err := u.GetOrganizations(all); err != nil {
		ctx.APIError(500, "GetOrganizations", err)
		return
	}

	apiOrgs := make([]*api.Organization, len(u.Orgs))
	for i := range u.Orgs {
		apiOrgs[i] = convert.ToApiOrganization(u.Orgs[i])
	}
	ctx.JSON(200, &apiOrgs)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#list-your-organizations
func ListMyOrgs(ctx *context.Context) {
	listUserOrgs(ctx, ctx.User, true)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#list-user-organizations
func ListUserOrgs(ctx *context.Context) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}
	listUserOrgs(ctx, u, false)
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#get-an-organization
func Get(ctx *context.Context) {
	org := user.GetUserByParamsName(ctx, ":orgname")
	if ctx.Written() {
		return
	}
	ctx.JSON(200, convert.ToApiOrganization(org))
}

// https://github.com/gogits/go-gogs-client/wiki/Organizations#edit-an-organization
func Edit(ctx *context.Context, form api.EditOrgOption) {
	org := user.GetUserByParamsName(ctx, ":orgname")
	if ctx.Written() {
		return
	}

	if !org.IsOwnedBy(ctx.User.Id) {
		ctx.Error(403)
		return
	}

	org.FullName = form.FullName
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	if err := models.UpdateUser(org); err != nil {
		ctx.APIError(500, "UpdateUser", err)
		return
	}

	ctx.JSON(200, convert.ToApiOrganization(org))
}
