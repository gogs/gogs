// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/routes/api/v1/convert"
	"github.com/gogs/gogs/routes/api/v1/user"
)

func CreateOrgForUser(c *context.APIContext, apiForm api.CreateOrgOption, user *models.User) {
	if c.Written() {
		return
	}

	org := &models.User{
		Name:        apiForm.UserName,
		FullName:    apiForm.FullName,
		Description: apiForm.Description,
		Website:     apiForm.Website,
		Location:    apiForm.Location,
		IsActive:    true,
		Type:        models.USER_TYPE_ORGANIZATION,
	}
	if err := models.CreateOrganization(org, user); err != nil {
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

func listUserOrgs(c *context.APIContext, u *models.User, all bool) {
	if err := u.GetOrganizations(all); err != nil {
		c.Error(500, "GetOrganizations", err)
		return
	}

	apiOrgs := make([]*api.Organization, len(u.Orgs))
	for i := range u.Orgs {
		apiOrgs[i] = convert.ToOrganization(u.Orgs[i])
	}
	c.JSON(200, &apiOrgs)
}

// https://github.com/gogs/go-gogs-client/wiki/Organizations#list-your-organizations
func ListMyOrgs(c *context.APIContext) {
	listUserOrgs(c, c.User, true)
}

// https://github.com/gogs/go-gogs-client/wiki/Organizations#create-your-organization
func CreateMyOrg(c *context.APIContext, apiForm api.CreateOrgOption) {
	CreateOrgForUser(c, apiForm, c.User)
}

// https://github.com/gogs/go-gogs-client/wiki/Organizations#list-user-organizations
func ListUserOrgs(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserOrgs(c, u, false)
}

// https://github.com/gogs/go-gogs-client/wiki/Organizations#get-an-organization
func Get(c *context.APIContext) {
	c.JSON(200, convert.ToOrganization(c.Org.Organization))
}

// https://github.com/gogs/go-gogs-client/wiki/Organizations#edit-an-organization
func Edit(c *context.APIContext, form api.EditOrgOption) {
	org := c.Org.Organization
	if !org.IsOwnedBy(c.User.ID) {
		c.Status(403)
		return
	}

	org.FullName = form.FullName
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	if err := models.UpdateUser(org); err != nil {
		c.Error(500, "UpdateUser", err)
		return
	}

	c.JSON(200, convert.ToOrganization(org))
}
