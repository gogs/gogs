// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers/user"
)

const (
	NEW base.TplName = "org/new"
)

func Organization(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"]
	ctx.HTML(200, "org/org")
}

func Members(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Members"
	ctx.HTML(200, "org/members")
}

func Teams(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Teams"
	ctx.HTML(200, "org/teams")
}

func New(ctx *middleware.Context) {
	ctx.Data["Title"] = "Create An Organization"
	ctx.HTML(200, NEW)
}

func NewPost(ctx *middleware.Context, form auth.CreateOrganizationForm) {
	ctx.Data["Title"] = "Create An Organization"

	if ctx.HasError() {
		ctx.HTML(200, NEW)
		return
	}

	org := &models.User{
		Name:     form.OrgName,
		Email:    form.Email,
		IsActive: true, // NOTE: may need to set false when require e-mail confirmation.
		Type:     models.ORGANIZATION,
	}

	var err error
	if org, err = models.CreateOrganization(org, ctx.User); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr("Organization name has been already taken", NEW, &form)
		case models.ErrEmailAlreadyUsed:
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr("E-mail address has been already used", NEW, &form)
		case models.ErrUserNameIllegal:
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(models.ErrRepoNameIllegal.Error(), NEW, &form)
		default:
			ctx.Handle(500, "user.NewPost(CreateUser)", err)
		}
		return
	}
	log.Trace("%s Organization created: %s", ctx.Req.RequestURI, org.Name)

	ctx.Redirect("/org/" + form.OrgName + "/dashboard")
}

func Dashboard(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Dashboard"
	ctx.Data["PageIsUserDashboard"] = true
	ctx.Data["PageIsOrgDashboard"] = true

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.Dashboard(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.Dashboard(GetUserByName)", err)
		}
		return
	}

	if err := ctx.User.GetOrganizations(); err != nil {
		ctx.Handle(500, "home.Dashboard(GetOrganizations)", err)
		return
	}
	ctx.Data["Orgs"] = ctx.User.Orgs
	ctx.Data["ContextUser"] = org

	ctx.Data["MyRepos"], err = models.GetRepositories(org.Id, true)
	if err != nil {
		ctx.Handle(500, "org.Dashboard(GetRepositories)", err)
		return
	}

	actions, err := models.GetFeeds(org.Id, 0, false)
	if err != nil {
		ctx.Handle(500, "org.Dashboard(GetFeeds)", err)
		return
	}
	ctx.Data["Feeds"] = actions

	ctx.HTML(200, user.DASHBOARD)
}

func Setting(ctx *middleware.Context, param martini.Params) {
	ctx.Data["Title"] = "Setting"
	ctx.HTML(200, "org/setting")
}
