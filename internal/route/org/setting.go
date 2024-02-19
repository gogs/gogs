// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/route/user"
)

const (
	SETTINGS_OPTIONS = "org/settings/options"
	SETTINGS_DELETE  = "org/settings/delete"
)

func Settings(c *context.Context) {
	c.Title("org.settings")
	c.Data["PageIsSettingsOptions"] = true
	c.Success(SETTINGS_OPTIONS)
}

func SettingsPost(c *context.Context, f form.UpdateOrgSetting) {
	c.Title("org.settings")
	c.Data["PageIsSettingsOptions"] = true

	if c.HasError() {
		c.Success(SETTINGS_OPTIONS)
		return
	}

	org := c.Org.Organization

	// Check if the organization username (including cases) had been changed
	if org.Name != f.Name {
		err := database.Users.ChangeUsername(c.Req.Context(), c.Org.Organization.ID, f.Name)
		if err != nil {
			c.Data["OrgName"] = true
			var msg string
			switch {
			case database.IsErrUserAlreadyExist(err):
				msg = c.Tr("form.username_been_taken")
			case database.IsErrNameNotAllowed(err):
				msg = c.Tr("user.form.name_not_allowed", err.(database.ErrNameNotAllowed).Value())
			default:
				c.Error(err, "change organization name")
				return
			}

			c.RenderWithErr(msg, SETTINGS_OPTIONS, &f)
			return
		}

		// reset c.org.OrgLink with new name
		c.Org.OrgLink = conf.Server.Subpath + "/org/" + f.Name
		log.Trace("Organization name changed: %s -> %s", org.Name, f.Name)
	}

	opts := database.UpdateUserOptions{
		FullName:    &f.FullName,
		Website:     &f.Website,
		Location:    &f.Location,
		Description: &f.Description,
	}
	if c.User.IsAdmin {
		opts.MaxRepoCreation = &f.MaxRepoCreation
	}
	err := database.Users.Update(c.Req.Context(), c.Org.Organization.ID, opts)
	if err != nil {
		c.Error(err, "update organization")
		return
	}

	c.Flash.Success(c.Tr("org.settings.update_setting_success"))
	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsAvatar(c *context.Context, f form.Avatar) {
	f.Source = form.AvatarLocal
	if err := user.UpdateAvatarSetting(c, f, c.Org.Organization); err != nil {
		c.Flash.Error(err.Error())
	} else {
		c.Flash.Success(c.Tr("org.settings.update_avatar_success"))
	}

	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsDeleteAvatar(c *context.Context) {
	if err := database.Users.DeleteCustomAvatar(c.Req.Context(), c.Org.Organization.ID); err != nil {
		c.Flash.Error(err.Error())
	}

	c.Redirect(c.Org.OrgLink + "/settings")
}

func SettingsDelete(c *context.Context) {
	c.Title("org.settings")
	c.PageIs("SettingsDelete")

	org := c.Org.Organization
	if c.Req.Method == "POST" {
		if _, err := database.Users.Authenticate(c.Req.Context(), c.User.Name, c.Query("password"), c.User.LoginSource); err != nil {
			if auth.IsErrBadCredentials(err) {
				c.RenderWithErr(c.Tr("form.enterred_invalid_password"), SETTINGS_DELETE, nil)
			} else {
				c.Error(err, "authenticate user")
			}
			return
		}

		if err := database.DeleteOrganization(org); err != nil {
			if database.IsErrUserOwnRepos(err) {
				c.Flash.Error(c.Tr("form.org_still_own_repo"))
				c.Redirect(c.Org.OrgLink + "/settings/delete")
			} else {
				c.Error(err, "delete organization")
			}
		} else {
			log.Trace("Organization deleted: %s", org.Name)
			c.Redirect(conf.Server.Subpath + "/")
		}
		return
	}

	c.Success(SETTINGS_DELETE)
}
