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
)

const (
	HOME     base.TplName = "org/home"
	CREATE   base.TplName = "org/create"
	SETTINGS base.TplName = "org/settings"
)

func Home(ctx *middleware.Context) {
	ctx.Data["Title"] = "Organization " + ctx.Params(":org")

	org, err := models.GetUserByName(ctx.Params(":org"))
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
	if org, err = models.CreateOrganization(org, ctx.User); err != nil {
		switch err {
		case models.ErrUserAlreadyExist:
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("form.org_name_been_taken"), CREATE, &form)
		case models.ErrEmailAlreadyUsed:
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), CREATE, &form)
		case models.ErrUserNameIllegal:
			ctx.Data["Err_OrgName"] = true
			ctx.RenderWithErr(ctx.Tr("form.illegal_org_name"), CREATE, &form)
		default:
			ctx.Handle(500, "CreateUser", err)
		}
		return
	}
	log.Trace("Organization created: %s", org.Name)

	ctx.Redirect("/org/" + form.OrgName + "/dashboard")
}

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(ctx.Params(":org"))
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

func SettingsPost(ctx *middleware.Context, form auth.OrgSettingForm) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(ctx.Params(":org"))
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

func DeletePost(ctx *middleware.Context) {
	ctx.Data["Title"] = "Settings"

	org, err := models.GetUserByName(ctx.Params(":org"))
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
