// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
)

const (
	CREATE = "org/create"
)

func Create(c *context.Context) {
	c.Title("new_org")
	c.Success(CREATE)
}

func CreatePost(c *context.Context, f form.CreateOrg) {
	c.Title("new_org")

	if c.HasError() {
		c.Success(CREATE)
		return
	}

	org := &database.User{
		Name:     f.OrgName,
		IsActive: true,
		Type:     database.UserTypeOrganization,
	}

	if err := database.CreateOrganization(org, c.User); err != nil {
		c.Data["Err_OrgName"] = true
		switch {
		case database.IsErrUserAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.org_name_been_taken"), CREATE, &f)
		case database.IsErrNameNotAllowed(err):
			c.RenderWithErr(c.Tr("org.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value()), CREATE, &f)
		default:
			c.Error(err, "create organization")
		}
		return
	}
	log.Trace("Organization created: %s", org.Name)

	c.RedirectSubpath("/org/" + f.OrgName + "/dashboard")
}
