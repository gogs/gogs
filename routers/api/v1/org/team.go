// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gigforks/go-gogs-client"

	"github.com/gigforks/gogs/models"
	"github.com/gigforks/gogs/routers/api/v1/user"
	"github.com/gigforks/gogs/modules/context"
	"github.com/gigforks/gogs/routers/api/v1/convert"
)

func ListTeams(ctx *context.APIContext) {
	org := ctx.Org.Organization
	if err := org.GetTeams(); err != nil {
		ctx.Error(500, "GetTeams", err)
		return
	}

	apiTeams := make([]*api.Team, len(org.Teams))
	for i := range org.Teams {
		apiTeams[i] = convert.ToTeam(org.Teams[i])
	}
	ctx.JSON(200, apiTeams)
}

func AddOrganizationUser(ctx *context.APIContext, form api.AddUserOption) {
	u, err := models.GetUserByName(form.UserName)

	if err != nil {
		ctx.Error(404, "user does not exist", err)
		return
	}
	
	org := user.GetUserByParamsName(ctx, ":orgname")
	err = models.AddOrgUser(org.Id, u.Id)

	if ctx.Written() {
		return
	}
	if err != nil {
		ctx.Error(500, "", err)
	}
	ctx.Status(201)
}

func RemoveOrganizationUser(ctx *context.APIContext) {

	u := user.GetUserByParamsName(ctx, ":user")

	if ctx.Written() {
		return
	}

	org := user.GetUserByParamsName(ctx, ":orgname")

	if ctx.Written() {
		return
	}

	err := models.RemoveOrgUser(org.Id, u.Id)

	if err != nil {
		ctx.Error(500, "", err)
	}
	ctx.Status(204)
}