// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	SETTINGS_PROFILE  base.TplName = "user/settings/profile"
	SETTINGS_PASSWORD base.TplName = "user/settings/password"
	SETTINGS_SSH_KEYS base.TplName = "user/settings/sshkeys"
	SETTINGS_SOCIAL   base.TplName = "user/settings/social"
	SETTINGS_ORGS     base.TplName = "user/settings/orgs"
	SETTINGS_DELETE   base.TplName = "user/settings/delete"
	NOTIFICATION      base.TplName = "user/notification"
	SECURITY          base.TplName = "user/security"
)

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsProfile"] = true
	ctx.HTML(200, SETTINGS_PROFILE)
}

func SettingsPost(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsProfile"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_PROFILE)
		return
	}

	// Check if user name has been changed.
	if ctx.User.Name != form.UserName {
		isExist, err := models.IsUserExist(form.UserName)
		if err != nil {
			ctx.Handle(500, "IsUserExist", err)
			return
		} else if isExist {
			ctx.RenderWithErr(ctx.Tr("form.username_been_taken"), SETTINGS_PROFILE, &form)
			return
		} else if err = models.ChangeUserName(ctx.User, form.UserName); err != nil {
			if err == models.ErrUserNameIllegal {
				ctx.Flash.Error(ctx.Tr("form.illegal_username"))
				ctx.Redirect("/user/settings")
				return
			} else {
				ctx.Handle(500, "ChangeUserName", err)
			}
			return
		}
		log.Trace("User name changed: %s -> %s", ctx.User.Name, form.UserName)
		ctx.User.Name = form.UserName
	}

	ctx.User.FullName = form.FullName
	ctx.User.Email = form.Email
	ctx.User.Website = form.Website
	ctx.User.Location = form.Location
	ctx.User.Avatar = base.EncodeMd5(form.Avatar)
	ctx.User.AvatarEmail = form.Avatar
	if err := models.UpdateUser(ctx.User); err != nil {
		ctx.Handle(500, "UpdateUser", err)
		return
	}
	log.Trace("User setting updated: %s", ctx.User.Name)
	ctx.Flash.Success(ctx.Tr("settings.update_profile_success"))
	ctx.Redirect("/user/settings")
}

func SettingsPassword(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsPassword"] = true
	ctx.HTML(200, SETTINGS_PASSWORD)
}

func SettingsPasswordPost(ctx *middleware.Context, form auth.ChangePasswordForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsPassword"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_PASSWORD)
		return
	}

	tmpUser := &models.User{
		Passwd: form.OldPassword,
		Salt:   ctx.User.Salt,
	}
	tmpUser.EncodePasswd()
	if ctx.User.Passwd != tmpUser.Passwd {
		ctx.Flash.Error(ctx.Tr("settings.password_incorrect"))
	} else if form.Password != form.Retype {
		ctx.Flash.Error(ctx.Tr("form.password_not_match"))
	} else {
		ctx.User.Passwd = form.Password
		ctx.User.Salt = models.GetUserSalt()
		ctx.User.EncodePasswd()
		if err := models.UpdateUser(ctx.User); err != nil {
			ctx.Handle(500, "UpdateUser", err)
			return
		}
		log.Trace("User password updated: %s", ctx.User.Name)
		ctx.Flash.Success(ctx.Tr("settings.change_password_success"))
	}

	ctx.Redirect("/user/settings/password")
}

func SettingsSSHKeys(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsSSHKeys"] = true

	var err error
	ctx.Data["Keys"], err = models.ListPublicKey(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ssh.ListPublicKey", err)
		return
	}

	ctx.HTML(200, SETTINGS_SSH_KEYS)
}

func SettingsSSHKeysPost(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsSSHKeys"] = true

	var err error
	ctx.Data["Keys"], err = models.ListPublicKey(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ssh.ListPublicKey", err)
		return
	}

	// Delete SSH key.
	if ctx.Query("_method") == "DELETE" {
		id := com.StrTo(ctx.Query("id")).MustInt64()
		if id <= 0 {
			return
		}

		if err = models.DeletePublicKey(&models.PublicKey{Id: id}); err != nil {
			ctx.Handle(500, "DeletePublicKey", err)
		} else {
			log.Trace("SSH key deleted: %s", ctx.User.Name)
			ctx.Redirect("/user/settings/ssh")
		}
		return
	}

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_SSH_KEYS)
			return
		}

		// Remove newline characters from form.KeyContent
		cleanKeyContent := strings.Replace(form.KeyContent, "\n", "", -1)

		if ok, err := models.CheckPublicKeyString(cleanKeyContent); !ok {
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
			ctx.Redirect("/user/settings/ssh")
			return
		}

		k := &models.PublicKey{
			OwnerId: ctx.User.Id,
			Name:    form.SSHTitle,
			Content: cleanKeyContent,
		}
		if err := models.AddPublicKey(k); err != nil {
			if err == models.ErrKeyAlreadyExist {
				ctx.RenderWithErr(ctx.Tr("form.ssh_key_been_used"), SETTINGS_SSH_KEYS, &form)
				return
			}
			ctx.Handle(500, "ssh.AddPublicKey", err)
			return
		} else {
			log.Trace("SSH key added: %s", ctx.User.Name)
			ctx.Flash.Success(ctx.Tr("settings.add_key_success"))
			ctx.Redirect("/user/settings/ssh")
			return
		}
	}

	ctx.HTML(200, SETTINGS_SSH_KEYS)
}

func SettingsSocial(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsSocial"] = true

	// Unbind social account.
	remove, _ := com.StrTo(ctx.Query("remove")).Int64()
	if remove > 0 {
		if err := models.DeleteOauth2ById(remove); err != nil {
			ctx.Handle(500, "DeleteOauth2ById", err)
			return
		}
		ctx.Flash.Success(ctx.Tr("settings.unbind_success"))
		ctx.Redirect("/user/settings/social")
		return
	}

	socials, err := models.GetOauthByUserId(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "GetOauthByUserId", err)
		return
	}
	ctx.Data["Socials"] = socials
	ctx.HTML(200, SETTINGS_SOCIAL)
}

func SettingsOrgs(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsOrgs"] = true
	ctx.HTML(200, SETTINGS_ORGS)
}

func SettingsDelete(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsDelete"] = true

	if ctx.Req.Method == "POST" {
		// tmpUser := models.User{
		// 	Passwd: ctx.Query("password"),
		// 	Salt:   ctx.User.Salt,
		// }
		// tmpUser.EncodePasswd()
		// if tmpUser.Passwd != ctx.User.Passwd {
		// 	ctx.Flash.Error("Password is not correct. Make sure you are owner of this account.")
		// } else {
		if err := models.DeleteUser(ctx.User); err != nil {
			switch err {
			case models.ErrUserOwnRepos:
				ctx.Flash.Error(ctx.Tr("form.still_own_repo"))
				ctx.Redirect("/user/settings/delete")
			default:
				ctx.Handle(500, "DeleteUser", err)
			}
		} else {
			log.Trace("Account deleted: %s", ctx.User.Name)
			ctx.Redirect("/")
		}
		return
	}

	ctx.HTML(200, SETTINGS_DELETE)
}
