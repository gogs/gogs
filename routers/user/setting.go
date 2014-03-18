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

// render user setting page (email, website modify)
func Setting(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true

	user := ctx.User
	ctx.Data["Owner"] = user

	if ctx.Req.Method == "GET" {
		ctx.Render.HTML(200, "user/setting", ctx.Data)
		return
	}

	// below is for POST requests
	if hasErr, ok := ctx.Data["HasError"]; ok && hasErr.(bool) {
		ctx.Render.HTML(200, "user/setting", ctx.Data)
		return
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
	ctx.Render.HTML(200, "user/setting", ctx.Data)
}

func SettingPassword(ctx *middleware.Context, form auth.UpdatePasswdForm) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true

	if ctx.Req.Method == "GET" {
		ctx.Render.HTML(200, "user/password", ctx.Data)
		return
	}

	user := ctx.User
	newUser := &models.User{Passwd: form.NewPasswd}
	if err := newUser.EncodePasswd(); err != nil {
		ctx.Handle(200, "setting.SettingPassword", err)
		return
	}

	if user.Passwd != newUser.Passwd {
		ctx.Data["HasError"] = true
		ctx.Data["ErrorMsg"] = "Old password is not correct"
	} else if form.NewPasswd != form.RetypePasswd {
		ctx.Data["HasError"] = true
		ctx.Data["ErrorMsg"] = "New password and re-type password are not same"
	} else {
		user.Passwd = newUser.Passwd
		if err := models.UpdateUser(user); err != nil {
			ctx.Handle(200, "setting.SettingPassword", err)
			return
		}
		ctx.Data["IsSuccess"] = true
	}

	ctx.Data["Owner"] = user
	ctx.Render.HTML(200, "user/password", ctx.Data)
}

func SettingSSHKeys(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = "SSH Keys"

	// Delete SSH key.
	if ctx.Req.Method == "DELETE" || ctx.Query("_method") == "DELETE" {
		id, err := strconv.ParseInt(ctx.Query("id"), 10, 64)
		if err != nil {
			ctx.Data["ErrorMsg"] = err
			log.Error("ssh.DelPublicKey: %v", err)
			ctx.Render.JSON(200, map[string]interface{}{
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
			ctx.Data["ErrorMsg"] = err
			log.Error("ssh.DelPublicKey: %v", err)
			ctx.Render.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
		} else {
			ctx.Render.JSON(200, map[string]interface{}{
				"ok": true,
			})
		}
		return
	}

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if hasErr, ok := ctx.Data["HasError"]; ok && hasErr.(bool) {
			ctx.Render.HTML(200, "user/publickey", ctx.Data)
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
	ctx.Data["Keys"] = keys
	ctx.Render.HTML(200, "user/publickey", ctx.Data)
}

func SettingNotification(ctx *middleware.Context) {
	// todo user setting notification
	ctx.Data["Title"] = "Notification"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Render.HTML(200, "user/notification", ctx.Data)
}

func SettingSecurity(ctx *middleware.Context) {
	// todo user setting security
	ctx.Data["Title"] = "Security"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Render.HTML(200, "user/security", ctx.Data)
}
