// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	HOME   base.TplName = "org/home"
	CREATE base.TplName = "org/create"
)

func Home(ctx *middleware.Context) {
	org := ctx.Org.Organization
	ctx.Data["Title"] = org.FullName

	repos, err := models.GetRepositories(org.Id, ctx.IsSigned && org.IsOrgMember(ctx.User.Id))
	if err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}
	ctx.Data["Repos"] = repos

	if err = org.GetMembers(); err != nil {
		ctx.Handle(500, "GetMembers", err)
		return
	}
	ctx.Data["Members"] = org.Members

	if err = org.GetTeams(); err != nil {
		ctx.Handle(500, "GetTeams", err)
		return
	}
	ctx.Data["Teams"] = org.Teams

	ctx.HTML(200, HOME)
}

func Create(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("new_org")
	ctx.HTML(200, CREATE)
}

func CreatePost(ctx *middleware.Context, form auth.CreateOrgForm) {
	ctx.Data["Title"] = ctx.Tr("new_org")

	if ctx.HasError() {
		ctx.HTML(200, CREATE)
		return
	}

	org := &models.User{
		Name:     form.OrgName,
		Email:    form.Email,
		IsActive: true,
		Type:     models.ORGANIZATION,
	}

	var err error
	if err = models.CreateOrganization(org, ctx.User); err != nil {
		switch {
		case models.IsErrUserAlreadyExist(err):
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("form.org_name_been_taken"), CREATE, &form)
		case models.IsErrEmailAlreadyUsed(err):
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), CREATE, &form)
		case models.IsErrNameReserved(err):
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("org.form.name_reserved", err.(models.ErrNameReserved).Name), CREATE, &form)
		case models.IsErrNamePatternNotAllowed(err):
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("org.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), CREATE, &form)
		default:
			ctx.Handle(500, "CreateOrganization", err)
		}
		return
	}
	log.Trace("Organization created: %s", org.Name)

	ctx.Redirect(setting.AppSubUrl + "/org/" + form.OrgName + "/dashboard")
}
