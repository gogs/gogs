// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"encoding/hex"
	"errors"
	"fmt"
	"path"

	"github.com/Unknwon/com"
	"github.com/Unknwon/macaron"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const (
	AUTH_ACTIVE           base.TplName = "mail/auth/active"
	AUTH_ACTIVATE_EMAIL   base.TplName = "mail/auth/activate_email"
	AUTH_REGISTER_SUCCESS base.TplName = "mail/auth/register_success"
	AUTH_RESET_PASSWORD   base.TplName = "mail/auth/reset_passwd"

	NOTIFY_COLLABORATOR base.TplName = "mail/notify/collaborator"
	NOTIFY_MENTION      base.TplName = "mail/notify/mention"
)

// Create New mail message use MailFrom and MailUser
func NewMailMessageFrom(To []string, from, subject, body string) Message {
	return NewHtmlMessage(To, from, subject, body)
}

// Create New mail message use MailFrom and MailUser
func NewMailMessage(To []string, subject, body string) Message {
	return NewMailMessageFrom(To, setting.MailService.From, subject, body)
}

func GetMailTmplData(u *models.User) map[interface{}]interface{} {
	data := make(map[interface{}]interface{}, 10)
	data["AppName"] = setting.AppName
	data["AppVer"] = setting.AppVer
	data["AppUrl"] = setting.AppUrl
	data["ActiveCodeLives"] = setting.Service.ActiveCodeLives / 60
	data["ResetPwdCodeLives"] = setting.Service.ResetPwdCodeLives / 60
	if u != nil {
		data["User"] = u
	}
	return data
}

// create a time limit code for user active
func CreateUserActiveCode(u *models.User, startInf interface{}) string {
	minutes := setting.Service.ActiveCodeLives
	data := com.ToStr(u.Id) + u.Email + u.LowerName + u.Passwd + u.Rands
	code := base.CreateTimeLimitCode(data, minutes, startInf)

	// add tail hex username
	code += hex.EncodeToString([]byte(u.LowerName))
	return code
}

// create a time limit code for user active
func CreateUserEmailActivateCode(u *models.User, e *models.EmailAddress, startInf interface{}) string {
	minutes := setting.Service.ActiveCodeLives
	data := com.ToStr(u.Id) + e.Email + u.LowerName + u.Passwd + u.Rands
	code := base.CreateTimeLimitCode(data, minutes, startInf)

	// add tail hex username
	code += hex.EncodeToString([]byte(u.LowerName))
	return code
}

// Send user register mail with active code
func SendRegisterMail(r macaron.Render, u *models.User) {
	code := CreateUserActiveCode(u, nil)
	subject := "Register success, Welcome"

	data := GetMailTmplData(u)
	data["Code"] = code
	body, err := r.HTMLString(string(AUTH_REGISTER_SUCCESS), data)
	if err != nil {
		log.Error(4, "mail.SendRegisterMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send register mail", u.Id)

	SendAsync(&msg)
}

// Send email verify active email.
func SendActiveMail(r macaron.Render, u *models.User) {
	code := CreateUserActiveCode(u, nil)

	subject := "Verify your e-mail address"

	data := GetMailTmplData(u)
	data["Code"] = code
	body, err := r.HTMLString(string(AUTH_ACTIVE), data)
	if err != nil {
		log.Error(4, "mail.SendActiveMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send active mail", u.Id)

	SendAsync(&msg)
}

// Send email to verify secondary email.
func SendActivateEmail(r macaron.Render, user *models.User, email *models.EmailAddress) {
	code := CreateUserEmailActivateCode(user, email, nil)

	subject := "Verify your e-mail address"

	data := GetMailTmplData(user)
	data["Code"] = code
	data["Email"] = email.Email
	body, err := r.HTMLString(string(AUTH_ACTIVATE_EMAIL), data)
	if err != nil {
		log.Error(4, "mail.SendActiveMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{email.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send activate email to %s", user.Id, email.Email)

	SendAsync(&msg)
}

// Send reset password email.
func SendResetPasswdMail(r macaron.Render, u *models.User) {
	code := CreateUserActiveCode(u, nil)

	subject := "Reset your password"

	data := GetMailTmplData(u)
	data["Code"] = code
	body, err := r.HTMLString(string(AUTH_RESET_PASSWORD), data)
	if err != nil {
		log.Error(4, "mail.SendResetPasswdMail(fail to render): %v", err)
		return
	}

	msg := NewMailMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send reset password email", u.Id)

	SendAsync(&msg)
}

// SendIssueNotifyMail sends mail notification of all watchers of repository.
func SendIssueNotifyMail(u, owner *models.User, repo *models.Repository, issue *models.Issue) ([]string, error) {
	ws, err := models.GetWatchers(repo.ID)
	if err != nil {
		return nil, errors.New("mail.NotifyWatchers(GetWatchers): " + err.Error())
	}

	tos := make([]string, 0, len(ws))
	for i := range ws {
		uid := ws[i].UserID
		if u.Id == uid {
			continue
		}
		u, err := models.GetUserByID(uid)
		if err != nil {
			return nil, errors.New("mail.NotifyWatchers(GetUserById): " + err.Error())
		}
		tos = append(tos, u.Email)
	}

	if len(tos) == 0 {
		return tos, nil
	}

	subject := fmt.Sprintf("[%s] %s (#%d)", repo.Name, issue.Name, issue.Index)
	content := fmt.Sprintf("%s<br>-<br> <a href=\"%s%s/%s/issues/%d\">View it on Gogs</a>.",
		base.RenderSpecialLink([]byte(issue.Content), owner.Name+"/"+repo.Name),
		setting.AppUrl, owner.Name, repo.Name, issue.Index)
	msg := NewMailMessageFrom(tos, u.Email, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, send issue notify emails", subject)
	SendAsync(&msg)
	return tos, nil
}

// SendIssueMentionMail sends mail notification for who are mentioned in issue.
func SendIssueMentionMail(r macaron.Render, u, owner *models.User,
	repo *models.Repository, issue *models.Issue, tos []string) error {

	if len(tos) == 0 {
		return nil
	}

	subject := fmt.Sprintf("[%s] %s (#%d)", repo.Name, issue.Name, issue.Index)

	data := GetMailTmplData(nil)
	data["IssueLink"] = fmt.Sprintf("%s/%s/issues/%d", owner.Name, repo.Name, issue.Index)
	data["Subject"] = subject

	body, err := r.HTMLString(string(NOTIFY_MENTION), data)
	if err != nil {
		return fmt.Errorf("mail.SendIssueMentionMail(fail to render): %v", err)
	}

	msg := NewMailMessageFrom(tos, u.Email, subject, body)
	msg.Info = fmt.Sprintf("Subject: %s, send issue mention emails", subject)
	SendAsync(&msg)
	return nil
}

// SendCollaboratorMail sends mail notification to new collaborator.
func SendCollaboratorMail(r macaron.Render, u, owner *models.User,
	repo *models.Repository) error {

	subject := fmt.Sprintf("%s added you to %s", owner.Name, repo.Name)

	data := GetMailTmplData(nil)
	data["RepoLink"] = path.Join(owner.Name, repo.Name)
	data["Subject"] = subject

	body, err := r.HTMLString(string(NOTIFY_COLLABORATOR), data)
	if err != nil {
		return fmt.Errorf("mail.SendCollaboratorMail(fail to render): %v", err)
	}

	msg := NewMailMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send register mail", u.Id)

	SendAsync(&msg)
	return nil
}
