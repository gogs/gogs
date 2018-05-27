// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
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

	org := &models.User{
		Name:     f.OrgName,
		IsActive: true,
		Type:     models.USER_TYPE_ORGANIZATION,
	}

	if err := models.CreateOrganization(org, c.User); err != nil {
		c.Data["Err_OrgName"] = true
		switch {
		case models.IsErrUserAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.org_name_been_taken"), CREATE, &f)
		case models.IsErrNameReserved(err):
			c.RenderWithErr(c.Tr("org.form.name_reserved", err.(models.ErrNameReserved).Name), CREATE, &f)
		case models.IsErrNamePatternNotAllowed(err):
			c.RenderWithErr(c.Tr("org.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), CREATE, &f)
		default:
			c.Handle(500, "CreateOrganization", err)
		}
		return
	}
	log.Trace("Organization created: %s", org.Name)

	c.Redirect(setting.AppSubURL + "/org/" + f.OrgName + "/dashboard")
}
