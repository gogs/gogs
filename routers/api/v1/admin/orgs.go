// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers/api/v1/convert"
	"github.com/gogits/gogs/routers/api/v1/user"
)

// https://github.com/gogits/go-gogs-client/wiki/Administration-Organizations#create-a-new-organization
func CreateOrg(ctx *middleware.Context, form api.CreateOrgOption) {
	u := user.GetUserByParams(ctx)
	if ctx.Written() {
		return
	}

	org := &models.User{
		Name:        form.UserName,
		FullName:    form.FullName,
		Description: form.Description,
		Website:     form.Website,
		Location:    form.Location,
		IsActive:    true,
		Type:        models.ORGANIZATION,
	}
	if err := models.CreateOrganization(org, u); err != nil {
		if models.IsErrUserAlreadyExist(err) ||
			models.IsErrNameReserved(err) ||
			models.IsErrNamePatternNotAllowed(err) {
			ctx.APIError(422, "CreateOrganization", err)
		} else {
			ctx.APIError(500, "CreateOrganization", err)
		}
		return
	}

	ctx.JSON(201, convert.ToApiOrganization(org))
}
func DeleteOrg(ctx *middleware.Context) {
	if ctx.Written() {
		return
	}
	org := user.GetUserByParamsName(ctx, ":orgname")
	err := models.DeleteOrganization(org)

	if err != nil {
		ctx.APIError(500, "", err)
	}
	ctx.Status(204)
}

func AddOrganizationUser(ctx *middleware.Context, form api.AddUserOption) {
	u, err := models.GetUserByName(form.UserName)

	if err != nil {
		ctx.APIError(404, "user does not exist", err)
		return
	}

	org := user.GetUserByParamsName(ctx, ":orgname")
	err = models.AddOrgUser(org.Id, u.Id)

	if ctx.Written() {
		return
	}
	if err != nil {
		ctx.APIError(500, "", err)
	}
	ctx.Status(201)
}

func RemoveOrganizationUser(ctx *middleware.Context) {

	u := user.GetUserByParamsName(ctx, ":user")
	org := user.GetUserByParamsName(ctx, ":orgname")
	err := models.RemoveOrgUser(org.Id, u.Id)

	if err != nil {
		ctx.APIError(404, "user does not exist", err)
		return
	}

	if ctx.Written() {
		return
	}

	if err != nil {
		ctx.APIError(500, "", err)
	}
	ctx.Status(204)
}
