// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gigforks/go-gogs-client"

	"github.com/gigforks/gogs/models"
	"github.com/gigforks/gogs/modules/context"
	"github.com/gigforks/gogs/routers/api/v1/convert"
	"github.com/gigforks/gogs/routers/api/v1/user"
)

// https://github.com/gigforks/go-gogs-client/wiki/Administration-Organizations#create-a-new-organization
func CreateOrg(ctx *context.APIContext, form api.CreateOrgOption) {
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
		Type:        models.USER_TYPE_ORGANIZATION,
	}
	if err := models.CreateOrganization(org, u); err != nil {
		if models.IsErrUserAlreadyExist(err) ||
			models.IsErrNameReserved(err) ||
			models.IsErrNamePatternNotAllowed(err) {
			ctx.Error(422, "", err)
		} else {
			ctx.Error(500, "CreateOrganization", err)
		}
		return
	}

	ctx.JSON(201, convert.ToOrganization(org))
}

func DeleteOrg(ctx *context.APIContext) {
	org := user.GetUserByParamsName(ctx, ":orgname")

	if ctx.Written() {
		return
	}

	err := models.DeleteOrganization(org)

	if err != nil {
		ctx.Error(500, "", err)
	}
	ctx.Status(204)
}
