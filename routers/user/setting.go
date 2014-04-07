// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"strconv"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

// Render user setting page (email, website modify)
func Setting(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true // For navbar arrow.
	ctx.Data["IsUserPageSetting"] = true // For setting nav highlight.

	user := ctx.User
	ctx.Data["Owner"] = user

	if ctx.Req.Method == "GET" || ctx.HasError() {
		ctx.HTML(200, "user/setting")
		return
	}

	// Check if user name has been changed.
	if user.Name != form.UserName {
		isExist, err := models.IsUserExist(form.UserName)
		if err != nil {
			ctx.Handle(404, "user.Setting(update: check existence)", err)
			return
		} else if isExist {
			ctx.RenderWithErr("User name has been taken.", "user/setting", &form)
			return
		} else if err = models.ChangeUserName(user, form.UserName); err != nil {
			ctx.Handle(404, "user.Setting(change user name)", err)
			return
		}
		log.Trace("%s User name changed: %s -> %s", ctx.Req.RequestURI, user.Name, form.UserName)

		user.Name = form.UserName
	}

	user.Email = form.Email
	user.Website = form.Website
	user.Location = form.Location
	user.Avatar = base.EncodeMd5(form.Avatar)
	user.AvatarEmail = form.Avatar
	if err := models.UpdateUser(user); err != nil {
		ctx.Handle(200, "setting.Setting", err)
		return
	}

	ctx.Data["IsSuccess"] = true
	ctx.HTML(200, "user/setting")
	log.Trace("%s User setting updated: %s", ctx.Req.RequestURI, ctx.User.LowerName)
}

func SettingPassword(ctx *middleware.Context, form auth.UpdatePasswdForm) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingPasswd"] = true

	if ctx.Req.Method == "GET" {
		ctx.HTML(200, "user/password")
		return
	}

	user := ctx.User
	newUser := &models.User{Passwd: form.NewPasswd}
	newUser.EncodePasswd()
	if user.Passwd != newUser.Passwd {
		ctx.Data["HasError"] = true
		ctx.Data["ErrorMsg"] = "Old password is not correct"
	} else if form.NewPasswd != form.RetypePasswd {
		ctx.Data["HasError"] = true
		ctx.Data["ErrorMsg"] = "New password and re-type password are not same"
	} else {
		newUser.Salt = models.GetUserSalt()
		user.Passwd = newUser.Passwd
		if err := models.UpdateUser(user); err != nil {
			ctx.Handle(200, "setting.SettingPassword", err)
			return
		}
		ctx.Data["IsSuccess"] = true
	}

	ctx.Data["Owner"] = user
	ctx.HTML(200, "user/password")
	log.Trace("%s User password updated: %s", ctx.Req.RequestURI, ctx.User.LowerName)
}

func SettingSSHKeys(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = "SSH Keys"

	// Delete SSH key.
	if ctx.Req.Method == "DELETE" || ctx.Query("_method") == "DELETE" {
		id, err := strconv.ParseInt(ctx.Query("id"), 10, 64)
		if err != nil {
			log.Error("ssh.DelPublicKey: %v", err)
			ctx.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
			return
		}
		k := &models.PublicKey{
			Id:      id,
			OwnerId: ctx.User.Id,
		}

		if err = models.DeletePublicKey(k); err != nil {
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

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if hasErr, ok := ctx.Data["HasError"]; ok && hasErr.(bool) {
			ctx.HTML(200, "user/publickey")
			return
		}

		k := &models.PublicKey{OwnerId: ctx.User.Id,
			Name:    form.KeyName,
			Content: form.KeyContent,
		}

		if err := models.AddPublicKey(k); err != nil {
			if err.Error() == models.ErrKeyAlreadyExist.Error() {
				ctx.RenderWithErr("Public key name has been used", "user/publickey", &form)
				return
			}
			ctx.Handle(200, "ssh.AddPublicKey", err)
			log.Trace("%s User SSH key added: %s", ctx.Req.RequestURI, ctx.User.LowerName)
			return
		} else {
			ctx.Data["AddSSHKeySuccess"] = true
		}
	}

	// List existed SSH keys.
	keys, err := models.ListPublicKey(ctx.User.Id)
	if err != nil {
		ctx.Handle(200, "ssh.ListPublicKey", err)
		return
	}

	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingSSH"] = true
	ctx.Data["Keys"] = keys
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
