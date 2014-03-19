// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package auth

import (
	"encoding/hex"
	"fmt"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/mailer"
)

// create a time limit code for user active
func CreateUserActiveCode(user *models.User, startInf interface{}) string {
	hours := base.Service.ActiveCodeLives / 60
	data := fmt.Sprintf("%d", user.Id) + user.Email + user.LowerName + user.Passwd + user.Rands
	code := base.CreateTimeLimitCode(data, hours, startInf)

	// add tail hex username
	code += hex.EncodeToString([]byte(user.LowerName))
	return code
}

// Send user register mail with active code
func SendRegisterMail(user *models.User) {
	code := CreateUserActiveCode(user, nil)
	subject := "Register success, Welcome"

	data := mailer.GetMailTmplData(user)
	data["Code"] = code
	body := base.RenderTemplate("mail/auth/register_success.html", data)
	_, _, _ = code, subject, body

	// msg := mailer.NewMailMessage([]string{user.Email}, subject, body)
	// msg.Info = fmt.Sprintf("UID: %d, send register mail", user.Id)

	// // async send mail
	// mailer.SendAsync(msg)
}
