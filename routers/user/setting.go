// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

func Setting(ctx *middleware.Context) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSetting"] = true
	ctx.Data["Owner"] = ctx.User
	ctx.HTML(200, "user/setting")
}

func SettingPost(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSetting"] = true

	user := ctx.User
	ctx.Data["Owner"] = user

	if ctx.HasError() {
		ctx.HTML(200, "user/setting")
		return
	}

	// Check if user name has been changed.
	if user.Name != form.UserName {
		isExist, err := models.IsUserExist(form.UserName)
		if err != nil {
			ctx.Handle(500, "user.Setting(update: check existence)", err)
			return
		} else if isExist {
			ctx.RenderWithErr("User name has been taken.", "user/setting", &form)
			return
		} else if err = models.ChangeUserName(user, form.UserName); err != nil {
			ctx.Handle(500, "user.Setting(change user name)", err)
			return
		}
		log.Trace("%s User name changed: %s -> %s", ctx.Req.RequestURI, user.Name, form.UserName)

		user.Name = form.UserName
	}

	user.FullName = form.FullName
	user.Email = form.Email
	user.Website = form.Website
	user.Location = form.Location
	user.Avatar = base.EncodeMd5(form.Avatar)
	user.AvatarEmail = form.Avatar
	if err := models.UpdateUser(user); err != nil {
		ctx.Handle(500, "setting.Setting", err)
		return
	}
	log.Trace("%s User setting updated: %s", ctx.Req.RequestURI, ctx.User.LowerName)

	ctx.Flash.Success("Your profile has been successfully updated.")
	ctx.Redirect("/user/settings")
}

func SettingSocial(ctx *middleware.Context) {
	ctx.Data["Title"] = "Social Account"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingSocial"] = true

	// Unbind social account.
	remove, _ := base.StrTo(ctx.Query("remove")).Int64()
	if remove > 0 {
		if err := models.DeleteOauth2ById(remove); err != nil {
			ctx.Handle(500, "user.SettingSocial(DeleteOauth2ById)", err)
			return
		}
		ctx.Flash.Success("OAuth2 has been unbinded.")
		ctx.Redirect("/user/settings/social")
		return
	}

	socials, err := models.GetOauthByUserId(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "user.SettingSocial(GetOauthByUserId)", err)
		return
	}

	ctx.Data["Socials"] = socials
	ctx.HTML(200, "user/social")
}

func SettingPassword(ctx *middleware.Context) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingPasswd"] = true
	ctx.HTML(200, "user/password")
}

func SettingPasswordPost(ctx *middleware.Context, form auth.UpdatePasswdForm) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingPasswd"] = true

	if ctx.HasError() {
		ctx.HTML(200, "user/password")
		return
	}

	user := ctx.User
	tmpUser := &models.User{
		Passwd: form.OldPasswd,
		Salt:   user.Salt,
	}
	tmpUser.EncodePasswd()
	if user.Passwd != tmpUser.Passwd {
		ctx.Flash.Error("Old password is not correct.")
	} else if form.NewPasswd != form.RetypePasswd {
		ctx.Flash.Error("New password and re-type password are not same.")
	} else {
		user.Passwd = form.NewPasswd
		user.Salt = models.GetUserSalt()
		user.EncodePasswd()
		if err := models.UpdateUser(user); err != nil {
			ctx.Handle(200, "setting.SettingPassword", err)
			return
		}
		log.Trace("%s User password updated: %s", ctx.Req.RequestURI, ctx.User.LowerName)
		ctx.Flash.Success("Password is changed successfully. You can now sign in via new password.")
	}

	ctx.Redirect("/user/settings/password")
}

func SettingSSHKeys(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = "SSH Keys"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingSSH"] = true

	// Delete SSH key.
	if ctx.Req.Method == "DELETE" || ctx.Query("_method") == "DELETE" {
		id, err := base.StrTo(ctx.Query("id")).Int64()
		if err != nil {
			log.Error("ssh.DelPublicKey: %v", err)
			ctx.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
			return
		}

		if err = models.DeletePublicKey(&models.PublicKey{Id: id}); err != nil {
			log.Error("ssh.DelPublicKey: %v", err)
			ctx.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
		} else {
			log.Trace("%s User SSH key deleted: %s", ctx.Req.RequestURI, ctx.User.LowerName)
			ctx.JSON(200, map[string]interface{}{
				"ok": true,
			})
		}
		return
	}

	// List existed SSH keys.
	keys, err := models.ListPublicKey(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ssh.ListPublicKey", err)
		return
	}
	ctx.Data["Keys"] = keys

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if ctx.HasError() {
			ctx.HTML(200, "user/publickey")
			return
		}

		if len(form.KeyContent) < 100 || !strings.HasPrefix(form.KeyContent, "ssh-rsa") {
			ctx.Flash.Error("SSH key content is not valid.")
			ctx.Redirect("/user/settings/ssh")
			return
		}

		k := &models.PublicKey{
			OwnerId: ctx.User.Id,
			Name:    form.KeyName,
			Content: form.KeyContent,
		}

		if err := models.AddPublicKey(k); err != nil {
			if err.Error() == models.ErrKeyAlreadyExist.Error() {
				ctx.RenderWithErr("Public key name has been used", "user/publickey", &form)
				return
			}
			ctx.Handle(500, "ssh.AddPublicKey", err)
			return
		} else {
			log.Trace("%s User SSH key added: %s", ctx.Req.RequestURI, ctx.User.LowerName)
			ctx.Flash.Success("New SSH Key has been added!")
			ctx.Redirect("/user/settings/ssh")
			return
		}
	}

	ctx.HTML(200, "user/publickey")
}

func SettingNotification(ctx *middleware.Context) {
	// TODO: user setting notification
	ctx.Data["Title"] = "Notification"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingNotify"] = true
	ctx.HTML(200, "user/notification")
}

func SettingSecurity(ctx *middleware.Context) {
	// TODO: user setting security
	ctx.Data["Title"] = "Security"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingSecurity"] = true
	ctx.HTML(200, "user/security")
}
