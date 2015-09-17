// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"errors"
	"fmt"
	"path"

	"github.com/Unknwon/macaron"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

const (
	AUTH_ACTIVATE         base.TplName = "mail/auth/activate"
	AUTH_ACTIVATE_EMAIL   base.TplName = "mail/auth/activate_email"
	AUTH_RESET_PASSWORD   base.TplName = "mail/auth/reset_passwd"
	AUTH_REGISTER_SUCCESS base.TplName = "mail/auth/register_success"

	NOTIFY_COLLABORATOR base.TplName = "mail/notify/collaborator"
	NOTIFY_MENTION      base.TplName = "mail/notify/mention"
)

func ComposeTplData(u *models.User) map[interface{}]interface{} {
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

func SendActivateAccountMail(c *macaron.Context, u *models.User) {
	data := ComposeTplData(u)
	data["Code"] = u.GenerateActivateCode()
	body, err := c.HTMLString(string(AUTH_ACTIVATE), data)
	if err != nil {
		log.Error(4, "HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email}, c.Tr("mail.activate_account"), body)
	msg.Info = fmt.Sprintf("UID: %d, activate account", u.Id)

	SendAsync(msg)
}

// SendActivateAccountMail sends confirmation e-mail.
func SendActivateEmailMail(c *macaron.Context, u *models.User, email *models.EmailAddress) {
	data := ComposeTplData(u)
	data["Code"] = u.GenerateEmailActivateCode(email.Email)
	data["Email"] = email.Email
	body, err := c.HTMLString(string(AUTH_ACTIVATE_EMAIL), data)
	if err != nil {
		log.Error(4, "HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{email.Email}, c.Tr("mail.activate_email"), body)
	msg.Info = fmt.Sprintf("UID: %d, activate email", u.Id)

	SendAsync(msg)
}

// SendResetPasswordMail sends reset password e-mail.
func SendResetPasswordMail(c *macaron.Context, u *models.User) {
	data := ComposeTplData(u)
	data["Code"] = u.GenerateActivateCode()
	body, err := c.HTMLString(string(AUTH_RESET_PASSWORD), data)
	if err != nil {
		log.Error(4, "HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email}, c.Tr("mail.reset_password"), body)
	msg.Info = fmt.Sprintf("UID: %d, reset password", u.Id)

	SendAsync(msg)
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
	msg := NewMessage(tos, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, send issue notify emails", subject)
	SendAsync(msg)
	return tos, nil
}

// SendIssueMentionMail sends mail notification for who are mentioned in issue.
func SendIssueMentionMail(r macaron.Render, u, owner *models.User,
	repo *models.Repository, issue *models.Issue, tos []string) error {

	if len(tos) == 0 {
		return nil
	}

	subject := fmt.Sprintf("[%s] %s (#%d)", repo.Name, issue.Name, issue.Index)

	data := ComposeTplData(nil)
	data["IssueLink"] = fmt.Sprintf("%s/%s/issues/%d", owner.Name, repo.Name, issue.Index)
	data["Subject"] = subject

	body, err := r.HTMLString(string(NOTIFY_MENTION), data)
	if err != nil {
		return fmt.Errorf("mail.SendIssueMentionMail(fail to render): %v", err)
	}

	msg := NewMessage(tos, subject, body)
	msg.Info = fmt.Sprintf("Subject: %s, send issue mention emails", subject)
	SendAsync(msg)
	return nil
}

// SendCollaboratorMail sends mail notification to new collaborator.
func SendCollaboratorMail(r macaron.Render, u, owner *models.User,
	repo *models.Repository) error {

	subject := fmt.Sprintf("%s added you to %s", owner.Name, repo.Name)

	data := ComposeTplData(nil)
	data["RepoLink"] = path.Join(owner.Name, repo.Name)
	data["Subject"] = subject

	body, err := r.HTMLString(string(NOTIFY_COLLABORATOR), data)
	if err != nil {
		return fmt.Errorf("mail.SendCollaboratorMail(fail to render): %v", err)
	}

	msg := NewMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, send register mail", u.Id)

	SendAsync(msg)
	return nil
}
