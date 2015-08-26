// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/Unknwon/com"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTINGS_OPTIONS base.TplName = "org/settings/options"
	SETTINGS_DELETE  base.TplName = "org/settings/delete"
	SETTINGS_HOOKS   base.TplName = "org/settings/hooks"
)

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.HTML(200, SETTINGS_OPTIONS)
}

func SettingsPost(ctx *middleware.Context, form auth.UpdateOrgSettingForm) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsOptions"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_OPTIONS)
		return
	}

	org := ctx.Org.Organization

	// Check if organization name has been changed.
	if org.Name != form.OrgUserName {
		isExist, err := models.IsUserExist(org.Id, form.OrgUserName)
		if err != nil {
			ctx.Handle(500, "IsUserExist", err)
			return
		} else if isExist {
			ctx.Data["Err_UserName"] = true
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), SETTINGS_OPTIONS, &form)
			return
		} else if err = models.ChangeUserName(org, form.OrgUserName); err != nil {
			if err == models.ErrUserNameIllegal {
				ctx.Data["Err_UserName"] = true
				ctx.RenderWithErr(ctx.Tr("form.illegal_username"), SETTINGS_OPTIONS, &form)
			} else {
				ctx.Handle(500, "ChangeUserName", err)
			}
			return
		}
		log.Trace("Organization name changed: %s -> %s", org.Name, form.OrgUserName)
		org.Name = form.OrgUserName
	}

	org.FullName = form.OrgFullName
	org.Email = form.Email
	org.Description = form.Description
	org.Website = form.Website
	org.Location = form.Location
	org.Avatar = base.EncodeMd5(form.Avatar)
	org.AvatarEmail = form.Avatar
	if err := models.UpdateUser(org); err != nil {
		if models.IsErrEmailAlreadyUsed(err) {
			ctx.Data["Err_Email"] = true
			ctx.RenderWithErr(ctx.Tr("form.email_been_used"), SETTINGS_OPTIONS, &form)
		} else {
			ctx.Handle(500, "UpdateUser", err)
		}
		return
	}
	log.Trace("Organization setting updated: %s", org.Name)
	ctx.Flash.Success(ctx.Tr("org.settings.update_setting_success"))
	ctx.Redirect(setting.AppSubUrl + "/org/" + org.Name + "/settings")
}

func SettingsDelete(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsDelete"] = true

	org := ctx.Org.Organization
	if ctx.Req.Method == "POST" {
		// FIXME: validate password.
		if err := models.DeleteOrganization(org); err != nil {
			if models.IsErrUserOwnRepos(err) {
				ctx.Flash.Error(ctx.Tr("form.org_still_own_repo"))
				ctx.Redirect(setting.AppSubUrl + "/org/" + org.LowerName + "/settings/delete")
			} else {
				ctx.Handle(500, "DeleteOrganization", err)
			}
		} else {
			log.Trace("Organization deleted: %s", org.Name)
			ctx.Redirect(setting.AppSubUrl + "/")
		}
		return
	}

	ctx.HTML(200, SETTINGS_DELETE)
}

func Webhooks(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("org.settings")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["BaseLink"] = ctx.Org.OrgLink
	ctx.Data["Description"] = ctx.Tr("org.settings.hooks_desc")

	// Delete web hook.
	remove := com.StrTo(ctx.Query("remove")).MustInt64()
	if remove > 0 {
		if err := models.DeleteWebhook(remove); err != nil {
			ctx.Handle(500, "DeleteWebhook", err)
			return
		}
		ctx.Flash.Success(ctx.Tr("repo.settings.remove_hook_success"))
		ctx.Redirect(ctx.Org.OrgLink + "/settings/hooks")
		return
	}

	ws, err := models.GetWebhooksByOrgId(ctx.Org.Organization.Id)
	if err != nil {
		ctx.Handle(500, "GetWebhooksByOrgId", err)
		return
	}

	ctx.Data["Webhooks"] = ws
	ctx.HTML(200, SETTINGS_HOOKS)
}

func DeleteWebhook(ctx *middleware.Context) {
	if err := models.DeleteWebhook(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteWebhook: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.webhook_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Org.OrgLink + "/settings/hooks",
	})
}
