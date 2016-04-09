// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/gomail.v2"
	"gopkg.in/macaron.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/markdown"
	"github.com/gogits/gogs/modules/setting"
)

const (
	AUTH_ACTIVATE        base.TplName = "mail/auth/activate"
	AUTH_ACTIVATE_EMAIL  base.TplName = "mail/auth/activate_email"
	AUTH_REGISTER_NOTIFY base.TplName = "mail/auth/register_notify"
	AUTH_RESET_PASSWORD  base.TplName = "mail/auth/reset_passwd"

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

func SendUserMail(c *macaron.Context, u *models.User, tpl base.TplName, code, subject, info string) {
	data := ComposeTplData(u)
	data["Code"] = code
	body, err := c.HTMLString(string(tpl), data)
	if err != nil {
		log.Error(4, "HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, %s", u.Id, info)

	SendAsync(msg)
}

func SendActivateAccountMail(c *macaron.Context, u *models.User) {
	SendUserMail(c, u, AUTH_ACTIVATE, u.GenerateActivateCode(), c.Tr("mail.activate_account"), "activate account")
}

// SendResetPasswordMail sends reset password e-mail.
func SendResetPasswordMail(c *macaron.Context, u *models.User) {
	SendUserMail(c, u, AUTH_RESET_PASSWORD, u.GenerateActivateCode(), c.Tr("mail.reset_password"), "reset password")
}

// SendRegisterNotifyMail triggers a notify e-mail by admin created a account.
func SendRegisterNotifyMail(c *macaron.Context, u *models.User) {
	body, err := c.HTMLString(string(AUTH_REGISTER_NOTIFY), ComposeTplData(u))
	if err != nil {
		log.Error(4, "HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email}, c.Tr("mail.register_notify"), body)
	msg.Info = fmt.Sprintf("UID: %d, registration notify", u.Id)

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

// SendIssueNotifyMail sends mail notification of all watchers of repository.
func SendIssueNotifyMail(u, owner *models.User, repo *models.Repository, issue *models.Issue) ([]string, error) {
	ws, err := models.GetWatchers(repo.ID)
	if err != nil {
		return nil, fmt.Errorf("GetWatchers[%d]: %v", repo.ID, err)
	}

	tos := make([]string, 0, len(ws))
	for i := range ws {
		uid := ws[i].UserID
		if u.Id == uid {
			continue
		}
		to, err := models.GetUserByID(uid)
		if err != nil {
			return nil, fmt.Errorf("GetUserByID: %v", err)
		}
		if to.IsOrganization() {
			continue
		}

		tos = append(tos, to.Email)
	}

	if len(tos) == 0 {
		return tos, nil
	}

	subject := fmt.Sprintf("[%s] %s (#%d)", repo.Name, issue.Name, issue.Index)
	content := fmt.Sprintf("%s<br>-<br> <a href=\"%s%s/%s/issues/%d\">View it on Gogs</a>.",
		markdown.RenderSpecialLink([]byte(strings.Replace(issue.Content, "\n", "<br>", -1)), owner.Name+"/"+repo.Name, repo.ComposeMetas()),
		setting.AppUrl, owner.Name, repo.Name, issue.Index)
	msg := NewMessage(tos, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, issue notify", subject)

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
	data["ActUserName"] = u.DisplayName()
	data["Content"] = string(markdown.RenderSpecialLink([]byte(issue.Content), owner.Name+"/"+repo.Name, repo.ComposeMetas()))

	body, err := r.HTMLString(string(NOTIFY_MENTION), data)
	if err != nil {
		return fmt.Errorf("HTMLString: %v", err)
	}

	msg := NewMessage(tos, subject, body)
	msg.Info = fmt.Sprintf("Subject: %s, issue mention", subject)

	SendAsync(msg)
	return nil
}

// SendCollaboratorMail sends mail notification to new collaborator.
func SendCollaboratorMail(r macaron.Render, u, doer *models.User, repo *models.Repository) error {
	subject := fmt.Sprintf("%s added you to %s/%s", doer.Name, repo.Owner.Name, repo.Name)

	data := ComposeTplData(nil)
	data["RepoLink"] = path.Join(repo.Owner.Name, repo.Name)
	data["Subject"] = subject

	body, err := r.HTMLString(string(NOTIFY_COLLABORATOR), data)
	if err != nil {
		return fmt.Errorf("HTMLString: %v", err)
	}

	msg := NewMessage([]string{u.Email}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, add collaborator", u.Id)

	SendAsync(msg)
	return nil
}

func SendTestMail(email string) error {
	return gomail.Send(&Sender{}, NewMessage([]string{email}, "Gogs Test Email!", "Gogs Test Email!").Message)
}
