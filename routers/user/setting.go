// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"io/ioutil"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTINGS_PROFILE      base.TplName = "user/settings/profile"
	SETTINGS_PASSWORD     base.TplName = "user/settings/password"
	SETTINGS_EMAILS       base.TplName = "user/settings/email"
	SETTINGS_SSH_KEYS     base.TplName = "user/settings/sshkeys"
	SETTINGS_SOCIAL       base.TplName = "user/settings/social"
	SETTINGS_APPLICATIONS base.TplName = "user/settings/applications"
	SETTINGS_DELETE       base.TplName = "user/settings/delete"
	NOTIFICATION          base.TplName = "user/notification"
	SECURITY              base.TplName = "user/security"
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
				ctx.Redirect(setting.AppSubUrl + "/user/settings")
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
	ctx.Redirect(setting.AppSubUrl + "/user/settings")
}

// FIXME: limit size.
func SettingsAvatar(ctx *middleware.Context, form auth.UploadAvatarForm) {
	defer ctx.Redirect(setting.AppSubUrl + "/user/settings")

	ctx.User.UseCustomAvatar = form.Enable

	if form.Avatar != nil {
		fr, err := form.Avatar.Open()
		if err != nil {
			ctx.Flash.Error(err.Error())
			return
		}

		data, err := ioutil.ReadAll(fr)
		if err != nil {
			ctx.Flash.Error(err.Error())
			return
		}
		if _, ok := base.IsImageFile(data); !ok {
			ctx.Flash.Error(ctx.Tr("settings.uploaded_avatar_not_a_image"))
			return
		}
		if err = ctx.User.UploadAvatar(data); err != nil {
			ctx.Flash.Error(err.Error())
			return
		}
	} else {
		// In case no avatar at all.
		if form.Enable && !com.IsFile(ctx.User.CustomAvatarPath()) {
			ctx.Flash.Error(ctx.Tr("settings.no_custom_avatar_available"))
			return
		}
	}

	if err := models.UpdateUser(ctx.User); err != nil {
		ctx.Flash.Error(err.Error())
		return
	}

	ctx.Flash.Success(ctx.Tr("settings.update_avatar_success"))
}

func SettingsEmails(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsEmails"] = true

	var err error
	ctx.Data["Emails"], err = models.GetEmailAddresses(ctx.User.Id)

	if err != nil {
		ctx.Handle(500, "email.GetEmailAddresses", err)
		return
	}

	ctx.HTML(200, SETTINGS_EMAILS)
}

func SettingsEmailPost(ctx *middleware.Context, form auth.AddEmailForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsEmails"] = true

	var err error
	ctx.Data["Emails"], err = models.GetEmailAddresses(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "email.GetEmailAddresses", err)
		return
	}

	// Delete Email address.
	if ctx.Query("_method") == "DELETE" {
		id := com.StrTo(ctx.Query("id")).MustInt64()
		if id <= 0 {
			return
		}

		if err = models.DeleteEmailAddress(&models.EmailAddress{Id: id}); err != nil {
			ctx.Handle(500, "DeleteEmail", err)
		} else {
			log.Trace("Email address deleted: %s", ctx.User.Name)
			ctx.Redirect(setting.AppSubUrl + "/user/settings/email")
		}
		return
	}

	// Make emailaddress primary.
	if ctx.Query("_method") == "PRIMARY" {
		id := com.StrTo(ctx.Query("id")).MustInt64()
		if id <= 0 {
			return
		}

		if err = models.MakeEmailPrimary(&models.EmailAddress{Id: id}); err != nil {
			ctx.Handle(500, "MakeEmailPrimary", err)
		} else {
			log.Trace("Email made primary: %s", ctx.User.Name)
			ctx.Redirect(setting.AppSubUrl + "/user/settings/email")
		}
		return
	}

	// Add Email address.
	if ctx.Req.Method == "POST" {
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_EMAILS)
			return
		}

		cleanEmail := strings.Replace(form.Email, "\n", "", -1)
		e := &models.EmailAddress{
			Uid:         ctx.User.Id,
			Email:       cleanEmail,
			IsActivated: !setting.Service.RegisterEmailConfirm,
		}

		if err := models.AddEmailAddress(e); err != nil {
			if err == models.ErrEmailAlreadyUsed {
				ctx.RenderWithErr(ctx.Tr("form.email_has_been_used"), SETTINGS_EMAILS, &form)
				return
			}
			ctx.Handle(500, "email.AddEmailAddress", err)
			return
		} else {

			// Send confirmation e-mail
			if setting.Service.RegisterEmailConfirm {
				mailer.SendActivateEmail(ctx.Render, ctx.User, e)

				if err := ctx.Cache.Put("MailResendLimit_"+ctx.User.LowerName, ctx.User.LowerName, 180); err != nil {
					log.Error(4, "Set cache(MailResendLimit) fail: %v", err)
				}
				ctx.Flash.Success(ctx.Tr("settings.add_email_success_confirmation_email_sent"))
			} else {
				ctx.Flash.Success(ctx.Tr("settings.add_email_success"))
			}

			log.Trace("Email address added: %s", e.Email)

			ctx.Redirect(setting.AppSubUrl + "/user/settings/email")
			return
		}

	}

	ctx.HTML(200, SETTINGS_EMAILS)
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

	ctx.Redirect(setting.AppSubUrl + "/user/settings/password")
}

func SettingsSSHKeys(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsSSHKeys"] = true

	var err error
	ctx.Data["Keys"], err = models.ListPublicKeys(ctx.User.Id)
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
	ctx.Data["Keys"], err = models.ListPublicKeys(ctx.User.Id)
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
			ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
		}
		return
	}

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_SSH_KEYS)
			return
		}

		// Parse openssh style string from form content
		content, err := models.ParseKeyString(form.Content)
		if err != nil {
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
			ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
			return
		}

		if ok, err := models.CheckPublicKeyString(content); !ok {
			if err == models.ErrKeyUnableVerify {
				ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
			} else {
				ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
				return
			}
		}

		k := &models.PublicKey{
			OwnerId: ctx.User.Id,
			Name:    form.SSHTitle,
			Content: content,
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
			ctx.Redirect(setting.AppSubUrl + "/user/settings/ssh")
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
		ctx.Redirect(setting.AppSubUrl + "/user/settings/social")
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

func SettingsApplications(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsApplications"] = true

	// Delete access token.
	remove, _ := com.StrTo(ctx.Query("remove")).Int64()
	if remove > 0 {
		if err := models.DeleteAccessTokenById(remove); err != nil {
			ctx.Handle(500, "DeleteAccessTokenById", err)
			return
		}
		ctx.Flash.Success(ctx.Tr("settings.delete_token_success"))
		ctx.Redirect(setting.AppSubUrl + "/user/settings/applications")
		return
	}

	tokens, err := models.ListAccessTokens(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ListAccessTokens", err)
		return
	}
	ctx.Data["Tokens"] = tokens

	ctx.HTML(200, SETTINGS_APPLICATIONS)
}

// FIXME: split to two different functions and pages to handle access token and oauth2
func SettingsApplicationsPost(ctx *middleware.Context, form auth.NewAccessTokenForm) {
	ctx.Data["Title"] = ctx.Tr("settings")
	ctx.Data["PageIsUserSettings"] = true
	ctx.Data["PageIsSettingsApplications"] = true

	switch ctx.Query("type") {
	case "token":
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_APPLICATIONS)
			return
		}

		t := &models.AccessToken{
			Uid:  ctx.User.Id,
			Name: form.Name,
		}
		if err := models.NewAccessToken(t); err != nil {
			ctx.Handle(500, "NewAccessToken", err)
			return
		}

		ctx.Flash.Success(ctx.Tr("settings.generate_token_succees"))
		ctx.Flash.Info(t.Sha1)
	}

	ctx.Redirect(setting.AppSubUrl + "/user/settings/applications")
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
				ctx.Redirect(setting.AppSubUrl + "/user/settings/delete")
			case models.ErrUserHasOrgs:
				ctx.Flash.Error(ctx.Tr("form.still_has_org"))
				ctx.Redirect(setting.AppSubUrl + "/user/settings/delete")
			default:
				ctx.Handle(500, "DeleteUser", err)
			}
		} else {
			log.Trace("Account deleted: %s", ctx.User.Name)
			ctx.Redirect(setting.AppSubUrl + "/")
		}
		return
	}

	ctx.HTML(200, SETTINGS_DELETE)
}
