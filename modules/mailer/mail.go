// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

// Create New mail message use MailFrom and MailUser
func NewMailMessageFrom(To []string, from, subject, body string) Message {
	msg := NewHtmlMessage(To, from, subject, body)
	msg.User = base.MailService.User
	return msg
}

// Create New mail message use MailFrom and MailUser
func NewMailMessage(To []string, subject, body string) Message {
	return NewMailMessageFrom(To, base.MailService.User, subject, body)
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
	minutes := base.Service.ActiveCodeLives
	data := base.ToStr(user.Id) + user.Email + user.LowerName + user.Passwd + user.Rands
	code := base.CreateTimeLimitCode(data, minutes, startInf)

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

	SendAsync(&msg)
}

// Send email verify active email.
func SendActiveMail(r *middleware.Render, user *models.User) {
	code := CreateUserActiveCode(user, nil)

	subject := "Verify your e-mail address"

	data := GetMailTmplData(user)
	data["Code"] = code
	body, err := r.HTMLString("mail/auth/active_email", data)
	if err != nil {
		log.Error("mail.SendActiveMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{user.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send active mail", user.Id)

	SendAsync(&msg)
}

// Send reset password email.
func SendResetPasswdMail(r *middleware.Render, user *models.User) {
	code := CreateUserActiveCode(user, nil)

	subject := "Reset your password"

	data := GetMailTmplData(user)
	data["Code"] = code
	body, err := r.HTMLString("mail/auth/reset_passwd", data)
	if err != nil {
		log.Error("mail.SendResetPasswdMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{user.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send reset password email", user.Id)

	SendAsync(&msg)
}

// SendIssueNotifyMail sends mail notification of all watchers of repository.
func SendIssueNotifyMail(user, owner *models.User, repo *models.Repository, issue *models.Issue) ([]string, error) {
	watches, err := models.GetWatches(repo.Id)
	if err != nil {
		return nil, errors.New("mail.NotifyWatchers(get watches): " + err.Error())
	}

	tos := make([]string, 0, len(watches))
	for i := range watches {
		uid := watches[i].UserId
		if user.Id == uid {
			continue
		}
		u, err := models.GetUserById(uid)
		if err != nil {
			return nil, errors.New("mail.NotifyWatchers(get user): " + err.Error())
		}
		tos = append(tos, u.Email)
	}

	if len(tos) == 0 {
		return tos, nil
	}

	subject := fmt.Sprintf("[%s] %s", repo.Name, issue.Name)
	content := fmt.Sprintf("%s<br>-<br> <a href=\"%s%s/%s/issues/%d\">View it on Gogs</a>.",
		base.RenderSpecialLink([]byte(issue.Content), owner.Name+"/"+repo.Name),
		base.AppUrl, owner.Name, repo.Name, issue.Index)
	msg := NewMailMessageFrom(tos, user.Name, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, send issue notify emails", subject)
	SendAsync(&msg)
	return tos, nil
}

// SendIssueMentionMail sends mail notification for who are mentioned in issue.
func SendIssueMentionMail(user, owner *models.User, repo *models.Repository, issue *models.Issue, tos []string) error {
	if len(tos) == 0 {
		return nil
	}

	issueLink := fmt.Sprintf("%s%s/%s/issues/%d", base.AppUrl, owner.Name, repo.Name, issue.Index)
	body := fmt.Sprintf(`%s mentioned you.`, user.Name)
	subject := fmt.Sprintf("[%s] %s", repo.Name, issue.Name)
	content := fmt.Sprintf("%s<br>-<br> <a href=\"%s\">View it on Gogs</a>.", body, issueLink)
	msg := NewMailMessageFrom(tos, user.Name, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, send issue mention emails", subject)
	SendAsync(&msg)
	return nil
}
