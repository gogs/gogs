// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/routes/api/v1/convert"
	"github.com/gogits/gogs/routes/api/v1/user"
)

// https://github.com/gogits/go-gogs-client/wiki/Administration-Organizations#create-a-new-organization
func CreateOrg(c *context.APIContext, form api.CreateOrgOption) {
	u := user.GetUserByParams(c)
	if c.Written() {
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
			c.Error(422, "", err)
		} else {
			c.Error(500, "CreateOrganization", err)
		}
		return
	}

	c.JSON(201, convert.ToOrganization(org))
}
