// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/route/api/v1/convert"
	"gogs.io/gogs/internal/route/api/v1/user"
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
		Type:        db.UserTypeOrganization,
	}
	if err := db.CreateOrganization(org, user); err != nil {
		if db.IsErrUserAlreadyExist(err) ||
			db.IsErrNameNotAllowed(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "create organization")
		}
		return
	}

	c.JSON(201, convert.ToOrganization(org))
}

func listUserOrgs(c *context.APIContext, u *db.User, all bool) {
	if err := u.GetOrganizations(all); err != nil {
		c.Error(err, "get organization")
		return
	}

	apiOrgs := make([]*api.Organization, len(u.Orgs))
	for i := range u.Orgs {
		apiOrgs[i] = convert.ToOrganization(u.Orgs[i])
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
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}
	listUserOrgs(c, u, false)
}

func Get(c *context.APIContext) {
	c.JSONSuccess(convert.ToOrganization(c.Org.Organization))
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
		c.Error(err, "update user")
		return
	}

	c.JSONSuccess(convert.ToOrganization(org))
}
