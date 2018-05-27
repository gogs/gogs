// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"strings"

	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/setting"
	"github.com/gogs/gogs/routes/user"
)

const (
	SETTINGS_OPTIONS  = "org/settings/options"
	SETTINGS_DELETE   = "org/settings/delete"
	SETTINGS_WEBHOOKS = "org/settings/webhooks"
)

func Settings(c *context.Context) {
	c.Data["Title"] = c.Tr("org.settings")
	c.Data["PageIsSettingsOptions"] = true
	c.HTML(200, SETTINGS_OPTIONS)
}

func SettingsPost(c *context.Context, f form.UpdateOrgSetting) {
	c.Data["Title"] = c.Tr("org.settings")
	c.Data["PageIsSettingsOptions"] = true

	if c.HasError() {
		c.HTML(200, SETTINGS_OPTIONS)
		return
	}

	org := c.Org.Organization

	// Check if organization name has been changed.
	if org.LowerName != strings.ToLower(f.Name) {
		isExist, err := models.IsUserExist(org.ID, f.Name)
		if err != nil {
			c.Handle(500, "IsUserExist", err)
			return
		} else if isExist {
			c.Data["OrgName"] = true
			c.RenderWithErr(c.Tr("form.username_been_taken"), SETTINGS_OPTIONS, &f)
			return
		} else if err = models.ChangeUserName(org, f.Name); err != nil {
			c.Data["OrgName"] = true
			switch {
			case models.IsErrNameReserved(err):
				c.RenderWithErr(c.Tr("user.form.name_reserved"), SETTINGS_OPTIONS, &f)
			case models.IsErrNamePatternNotAllowed(err):
				c.RenderWithErr(c.Tr("user.form.name_pattern_not_allowed"), SETTINGS_OPTIONS, &f)
			default:
				c.Handle(500, "ChangeUserName", err)
			}
			return
		}
		// reset c.org.OrgLink with new name
		c.Org.OrgLink = setting.AppSubURL + "/org/" + f.Name
		log.Trace("Organization name changed: %s -> %s", org.Name, f.Name)
	}
	// In case it's just a case change.
	org.Name = f.Name
	org.LowerName = strings.ToLower(f.Name)

	if c.User.IsAdmin {
		org.MaxRepoCreation = f.MaxRepoCreation
	}

	org.FullName = f.FullName
	org.Description = f.Description
	org.Website = f.Website
	org.Location = f.Location
	if err := models.UpdateUser(org); err != nil {
		c.Handle(500, "UpdateUser", err)
		return
	}
	log.Trace("Organization setting updated: %s", org.Name)
	c.Flash.Success(c.Tr("org.settings.update_setting_success"))
	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsAvatar(c *context.Context, f form.Avatar) {
	f.Source = form.AVATAR_LOCAL
	if err := user.UpdateAvatarSetting(c, f, c.Org.Organization); err != nil {
		c.Flash.Error(err.Error())
	} else {
		c.Flash.Success(c.Tr("org.settings.update_avatar_success"))
	}

	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsDeleteAvatar(c *context.Context) {
	if err := c.Org.Organization.DeleteAvatar(); err != nil {
		c.Flash.Error(err.Error())
	}

	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsDelete(c *context.Context) {
	c.Title("org.settings")
	c.PageIs("SettingsDelete")

	org := c.Org.Organization
	if c.Req.Method == "POST" {
		if _, err := models.UserLogin(c.User.Name, c.Query("password"), c.User.LoginSource); err != nil {
			if errors.IsUserNotExist(err) {
				c.RenderWithErr(c.Tr("form.enterred_invalid_password"), SETTINGS_DELETE, nil)
			} else {
				c.ServerError("UserLogin", err)
			}
			return
		}

		if err := models.DeleteOrganization(org); err != nil {
			if models.IsErrUserOwnRepos(err) {
				c.Flash.Error(c.Tr("form.org_still_own_repo"))
				c.Redirect(c.Org.OrgLink + "/settings/delete")
			} else {
				c.ServerError("DeleteOrganization", err)
			}
		} else {
			log.Trace("Organization deleted: %s", org.Name)
			c.Redirect(setting.AppSubURL + "/")
		}
		return
	}

	c.Success(SETTINGS_DELETE)
}

func Webhooks(c *context.Context) {
	c.Data["Title"] = c.Tr("org.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["BaseLink"] = c.Org.OrgLink
	c.Data["Description"] = c.Tr("org.settings.hooks_desc")
	c.Data["Types"] = setting.Webhook.Types

	ws, err := models.GetWebhooksByOrgID(c.Org.Organization.ID)
	if err != nil {
		c.Handle(500, "GetWebhooksByOrgId", err)
		return
	}

	c.Data["Webhooks"] = ws
	c.HTML(200, SETTINGS_WEBHOOKS)
}

func DeleteWebhook(c *context.Context) {
	if err := models.DeleteWebhookOfOrgByID(c.Org.Organization.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteWebhookByOrgID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.webhook_deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Org.OrgLink + "/settings/hooks",
	})
}
