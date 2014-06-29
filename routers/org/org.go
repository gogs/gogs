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
	HOME     base.TplName = "org/home"
	NEW      base.TplName = "org/new"
	SETTINGS base.TplName = "org/settings"
)

func Home(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"]

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.Home(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.Home(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	ctx.Data["Repos"], err = models.GetRepositories(org.Id,
		ctx.IsSigned && org.IsOrgMember(ctx.User.Id))
	if err != nil {
		ctx.Handle(500, "org.Home(GetRepositories)", err)
		return
	}

	if err = org.GetMembers(); err != nil {
		ctx.Handle(500, "org.Home(GetMembers)", err)
		return
	}
	ctx.Data["Members"] = org.Members

	if err = org.GetTeams(); err != nil {
		ctx.Handle(500, "org.Home(GetTeams)", err)
		return
	}
	ctx.Data["Teams"] = org.Teams

	ctx.HTML(200, HOME)
}

func New(ctx *middleware.Context) {
	ctx.Data["Title"] = "Create An Organization"
	ctx.HTML(200, NEW)
}

func NewPost(ctx *middleware.Context, form auth.CreateOrgForm) {
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
			ctx.Handle(500, "org.NewPost(CreateUser)", err)
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

func Settings(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.Settings(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.Settings(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	ctx.HTML(200, SETTINGS)
}

func SettingsPost(ctx *middleware.Context, params martini.Params, form auth.OrgSettingForm) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.SettingsPost(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.SettingsPost(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS)
		return
	}

	org.FullName = form.DisplayName
	org.Email = form.Email
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	if err = models.UpdateUser(org); err != nil {
		ctx.Handle(500, "org.SettingsPost(UpdateUser)", err)
		return
	}
	log.Trace("%s Organization setting updated: %s", ctx.Req.RequestURI, org.LowerName)
	ctx.Flash.Success("Organization profile has been successfully updated.")
	ctx.Redirect("/org/" + org.Name + "/settings")
}

func DeletePost(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.DeletePost(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.DeletePost(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	if !org.IsOrgOwner(ctx.User.Id) {
		ctx.Error(403)
		return
	}

	tmpUser := models.User{
		Passwd: ctx.Query("password"),
		Salt:   ctx.User.Salt,
	}
	tmpUser.EncodePasswd()
	if tmpUser.Passwd != ctx.User.Passwd {
		ctx.Flash.Error("Password is not correct. Make sure you are owner of this account.")
	} else {
		if err := models.DeleteOrganization(org); err != nil {
			switch err {
			case models.ErrUserOwnRepos:
				ctx.Flash.Error("This organization still have ownership of repository, you have to delete or transfer them first.")
			default:
				ctx.Handle(500, "org.DeletePost(DeleteOrganization)", err)
				return
			}
		} else {
			ctx.Redirect("/")
			return
		}
	}

	ctx.Redirect("/org/" + org.Name + "/settings")
}
