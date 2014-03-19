// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"encoding/hex"
	"fmt"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

// Create New mail message use MailFrom and MailUser
func NewMailMessage(To []string, subject, body string) Message {
	msg := NewHtmlMessage(To, base.MailService.User, subject, body)
	msg.User = base.MailService.User
	return msg
}

func GetMailTmplData(user *models.User) map[interface{}]interface{} {
	data := make(map[interface{}]interface{}, 10)
	data["AppName"] = base.AppName
	data["AppVer"] = base.AppVer
	data["AppUrl"] = base.AppUrl
	data["AppLogo"] = base.AppLogo
	data["ActiveCodeLives"] = base.Service.ActiveCodeLives / 60
	data["ResetPwdCodeLives"] = base.Service.ResetPwdCodeLives / 60
	if user != nil {
		data["User"] = user
	}
	return data
}

// create a time limit code for user active
func CreateUserActiveCode(user *models.User, startInf interface{}) string {
	hours := base.Service.ActiveCodeLives / 60
	data := base.ToStr(user.Id) + user.Email + user.LowerName + user.Passwd + user.Rands
	code := base.CreateTimeLimitCode(data, hours, startInf)

	// add tail hex username
	code += hex.EncodeToString([]byte(user.LowerName))
	return code
}

// Send user register mail with active code
func SendRegisterMail(r *middleware.Render, user *models.User) {
	code := CreateUserActiveCode(user, nil)
	subject := "Register success, Welcome"

	data := GetMailTmplData(user)
	data["Code"] = code
	body, err := r.HTMLString("mail/auth/register_success", data)
	if err != nil {
		log.Error("mail.SendRegisterMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{user.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send register mail", user.Id)

	// async send mail
	SendAsync(msg)
}

// Send email verify active email.
func SendActiveMail(r *middleware.Render, user *models.User) {
	code := CreateUserActiveCode(user, nil)

	subject := "Verify your email address"

	data := GetMailTmplData(user)
	data["Code"] = code
	body, err := r.HTMLString("mail/auth/active_email.html", data)
	if err != nil {
		log.Error("mail.SendActiveMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{user.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send email verify mail", user.Id)

	// async send mail
	SendAsync(msg)
}
