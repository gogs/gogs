// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package user

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/process"
)

const (
	SETTING      base.TplName = "user/setting"
	SOCIAL       base.TplName = "user/social"
	PASSWORD     base.TplName = "user/password"
	PUBLICKEY    base.TplName = "user/publickey"
	NOTIFICATION base.TplName = "user/notification"
	SECURITY     base.TplName = "user/security"
)

var (
	MinimumKeySize = map[string]int{
		"(ED25519)": 256,
		"(ECDSA)":   256,
		"(NTRU)":    1087,
		"(MCE)":     1702,
		"(McE)":     1702,
		"(RSA)":     2048,
	}
)

func Setting(ctx *middleware.Context) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSetting"] = true
	ctx.Data["Owner"] = ctx.User
	ctx.HTML(200, SETTING)
}

func SettingPost(ctx *middleware.Context, form auth.UpdateProfileForm) {
	ctx.Data["Title"] = "Setting"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSetting"] = true

	if ctx.HasError() {
		ctx.HTML(200, SETTING)
		return
	}

	ctx.Data["Owner"] = ctx.User

	// Check if user name has been changed.
	if ctx.User.Name != form.UserName {
		isExist, err := models.IsUserExist(form.UserName)
		if err != nil {
			ctx.Handle(500, "user.Setting(update: check existence)", err)
			return
		} else if isExist {
			ctx.RenderWithErr("User name has been taken.", "user/setting", &form)
			return
		} else if err = models.ChangeUserName(ctx.User, form.UserName); err != nil {
			ctx.Handle(500, "user.Setting(change user name)", err)
			return
		}
		log.Trace("%s User name changed: %s -> %s", ctx.Req.RequestURI, ctx.User.Name, form.UserName)

		ctx.User.Name = form.UserName
	}

	ctx.User.FullName = form.FullName
	ctx.User.Email = form.Email
	ctx.User.Website = form.Website
	ctx.User.Location = form.Location
	ctx.User.Avatar = base.EncodeMd5(form.Avatar)
	ctx.User.AvatarEmail = form.Avatar
	if err := models.UpdateUser(ctx.User); err != nil {
		ctx.Handle(500, "setting.Setting(UpdateUser)", err)
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

	var err error
	ctx.Data["Socials"], err = models.GetOauthByUserId(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "user.SettingSocial(GetOauthByUserId)", err)
		return
	}
	ctx.HTML(200, SOCIAL)
}

func SettingPassword(ctx *middleware.Context) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingPasswd"] = true
	ctx.HTML(200, PASSWORD)
}

func SettingPasswordPost(ctx *middleware.Context, form auth.UpdatePasswdForm) {
	ctx.Data["Title"] = "Password"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingPasswd"] = true

	if ctx.HasError() {
		ctx.HTML(200, PASSWORD)
		return
	}

	tmpUser := &models.User{
		Passwd: form.OldPasswd,
		Salt:   ctx.User.Salt,
	}
	tmpUser.EncodePasswd()
	if ctx.User.Passwd != tmpUser.Passwd {
		ctx.Flash.Error("Old password is not correct.")
	} else if form.NewPasswd != form.RetypePasswd {
		ctx.Flash.Error("New password and re-type password are not same.")
	} else {
		ctx.User.Passwd = form.NewPasswd
		ctx.User.Salt = models.GetUserSalt()
		ctx.User.EncodePasswd()
		if err := models.UpdateUser(ctx.User); err != nil {
			ctx.Handle(200, "setting.SettingPassword", err)
			return
		}
		log.Trace("%s User password updated: %s", ctx.Req.RequestURI, ctx.User.LowerName)
		ctx.Flash.Success("Password is changed successfully. You can now sign in via new password.")
	}
	ctx.Redirect("/user/settings/password")
}

// Checks if the given public key string is recognized by SSH.
func CheckPublicKeyString(keyContent string) (ok bool, err error) {
	if strings.ContainsAny(keyContent, "\n\r") {
		return false, errors.New("Only a single line with a single key please")
	}

	// write the key to a file…
	tmpFile, err := ioutil.TempFile(os.TempDir(), "keytest")
	if err != nil {
		return false, err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	tmpFile.WriteString(keyContent)
	tmpFile.Close()

	// … see if ssh-keygen recognizes its contents
	stdout, stderr, err := process.Exec("CheckPublicKeyString", "ssh-keygen", "-l", "-f", tmpPath)
	if err != nil {
		return false, errors.New("ssh-keygen -l -f: " + stderr)
	} else if len(stdout) < 2 {
		return false, errors.New("ssh-keygen returned not enough output to evaluate the key")
	}
	sshKeygenOutput := strings.Split(stdout, " ")
	if len(sshKeygenOutput) < 4 {
		return false, errors.New("Not enough fields returned by ssh-keygen -l -f")
	}
	keySize, err := strconv.Atoi(sshKeygenOutput[0])
	if err != nil {
		return false, errors.New("Cannot get key size of the given key")
	}
	keyType := strings.TrimSpace(sshKeygenOutput[len(sshKeygenOutput)-1])

	if minimumKeySize := MinimumKeySize[keyType]; minimumKeySize == 0 {
		return false, errors.New("Sorry, unrecognized public key type")
	} else {
		if keySize < minimumKeySize {
			return false, fmt.Errorf("The minimum accepted size of a public key %s is %d", keyType, minimumKeySize)
		}
	}

	return true, nil
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

	var err error
	// List existed SSH keys.
	ctx.Data["Keys"], err = models.ListPublicKey(ctx.User.Id)
	if err != nil {
		ctx.Handle(500, "ssh.ListPublicKey", err)
		return
	}

	// Add new SSH key.
	if ctx.Req.Method == "POST" {
		if ctx.HasError() {
			ctx.HTML(200, "user/publickey")
			return
		}

		if ok, err := CheckPublicKeyString(form.KeyContent); !ok {
			ctx.Flash.Error(err.Error())
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

	ctx.HTML(200, PUBLICKEY)
}

func SettingNotification(ctx *middleware.Context) {
	// TODO: user setting notification
	ctx.Data["Title"] = "Notification"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingNotify"] = true
	ctx.HTML(200, NOTIFICATION)
}

func SettingSecurity(ctx *middleware.Context) {
	// TODO: user setting security
	ctx.Data["Title"] = "Security"
	ctx.Data["PageIsUserSetting"] = true
	ctx.Data["IsUserPageSettingSecurity"] = true
	ctx.HTML(200, SECURITY)
}
