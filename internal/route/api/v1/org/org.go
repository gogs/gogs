// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	convert2 "gogs.io/gogs/internal/route/api/v1/convert"
	user2 "gogs.io/gogs/internal/route/api/v1/user"
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
)

func CreateOrgForUser(c *context.APIContext, apiForm api.CreateOrgOption, user *db.User) {
	if c.Written() {
		return
	}

	org := &db.User{
		Name:        apiForm.UserName,
		FullName:    apiForm.FullName,
		Description: apiForm.Description,
		Website:     apiForm.Website,
		Location:    apiForm.Location,
		IsActive:    true,
		Type:        db.USER_TYPE_ORGANIZATION,
	}
	if err := db.CreateOrganization(org, user); err != nil {
		if db.IsErrUserAlreadyExist(err) ||
			db.IsErrNameReserved(err) ||
			db.IsErrNamePatternNotAllowed(err) {
			c.Error(http.StatusUnprocessableEntity, "", err)
		} else {
			c.ServerError("CreateOrganization", err)
		}
		return
	}

	c.JSON(201, convert2.ToOrganization(org))
}

func listUserOrgs(c *context.APIContext, u *db.User, all bool) {
	if err := u.GetOrganizations(all); err != nil {
		c.ServerError("GetOrganizations", err)
		return
	}

	apiOrgs := make([]*api.Organization, len(u.Orgs))
	for i := range u.Orgs {
		apiOrgs[i] = convert2.ToOrganization(u.Orgs[i])
	}
	c.JSONSuccess(&apiOrgs)
}

func ListMyOrgs(c *context.APIContext) {
	listUserOrgs(c, c.User, true)
}

func CreateMyOrg(c *context.APIContext, apiForm api.CreateOrgOption) {
	CreateOrgForUser(c, apiForm, c.User)
}

func ListUserOrgs(c *context.APIContext) {
	u := user2.GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserOrgs(c, u, false)
}

func Get(c *context.APIContext) {
	c.JSONSuccess(convert2.ToOrganization(c.Org.Organization))
}

func Edit(c *context.APIContext, form api.EditOrgOption) {
	org := c.Org.Organization
	if !org.IsOwnedBy(c.User.ID) {
		c.Status(http.StatusForbidden)
		return
	}

	org.FullName = form.FullName
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	if err := db.UpdateUser(org); err != nil {
		c.ServerError("UpdateUser", err)
		return
	}

	c.JSONSuccess(convert2.ToOrganization(org))
}
