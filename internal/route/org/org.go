// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	log "gopkg.in/clog.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/setting"
)

const (
	CREATE = "org/create"
)

func Create(c *context.Context) {
	c.Data["Title"] = c.Tr("new_org")
	c.HTML(200, CREATE)
}

func CreatePost(c *context.Context, f form.CreateOrg) {
	c.Data["Title"] = c.Tr("new_org")

	if c.HasError() {
		c.HTML(200, CREATE)
		return
	}

	org := &db.User{
		Name:     f.OrgName,
		IsActive: true,
		Type:     db.USER_TYPE_ORGANIZATION,
	}

	if err := db.CreateOrganization(org, c.User); err != nil {
		c.Data["Err_OrgName"] = true
		switch {
		case db.IsErrUserAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.org_name_been_taken"), CREATE, &f)
		case db.IsErrNameReserved(err):
			c.RenderWithErr(c.Tr("org.form.name_reserved", err.(db.ErrNameReserved).Name), CREATE, &f)
		case db.IsErrNamePatternNotAllowed(err):
			c.RenderWithErr(c.Tr("org.form.name_pattern_not_allowed", err.(db.ErrNamePatternNotAllowed).Pattern), CREATE, &f)
		default:
			c.Handle(500, "CreateOrganization", err)
		}
		return
	}
	log.Trace("Organization created: %s", org.Name)

	c.Redirect(setting.AppSubURL + "/org/" + f.OrgName + "/dashboard")
}
