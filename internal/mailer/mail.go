// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package mailer

import (
	"fmt"
	"html/template"
	"path"
	"sync"
	"time"

	"gopkg.in/gomail.v2"
	"gopkg.in/macaron.v1"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/assets/templates"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/setting"
)

const (
	MAIL_AUTH_ACTIVATE        = "auth/activate"
	MAIL_AUTH_ACTIVATE_EMAIL  = "auth/activate_email"
	MAIL_AUTH_RESET_PASSWORD  = "auth/reset_passwd"
	MAIL_AUTH_REGISTER_NOTIFY = "auth/register_notify"

	MAIL_ISSUE_COMMENT = "issue/comment"
	MAIL_ISSUE_MENTION = "issue/mention"

	MAIL_NOTIFY_COLLABORATOR = "notify/collaborator"
)

var (
	tplRender     *macaron.TplRender
	tplRenderOnce sync.Once
)

// render renders a mail template with given data.
func render(tpl string, data map[string]interface{}) (string, error) {
	tplRenderOnce.Do(func() {
		opt := &macaron.RenderOptions{
			Directory:         path.Join(setting.StaticRootPath, "templates/mail"),
			AppendDirectories: []string{path.Join(setting.CustomPath, "templates/mail")},
			Extensions:        []string{".tmpl", ".html"},
			Funcs: []template.FuncMap{map[string]interface{}{
				"AppName": func() string {
					return setting.AppName
				},
				"AppURL": func() string {
					return setting.AppURL
				},
				"Year": func() int {
					return time.Now().Year()
				},
				"Str2HTML": func(raw string) template.HTML {
					return template.HTML(markup.Sanitize(raw))
				},
			}},
		}
		if !setting.LoadAssetsFromDisk {
			opt.TemplateFileSystem = templates.NewTemplateFileSystem("mail", opt.AppendDirectories[0])
		}

		ts := macaron.NewTemplateSet()
		ts.Set(macaron.DEFAULT_TPL_SET_NAME, opt)
		tplRender = &macaron.TplRender{
			TemplateSet: ts,
			Opt:         opt,
		}
	})

	return tplRender.HTMLString(tpl, data)
}

func SendTestMail(email string) error {
	return gomail.Send(&Sender{}, NewMessage([]string{email}, "Gogs Test Email", "Hello 👋, greeting from Gogs!").Message)
}

/*
	Setup interfaces of used methods in mail to avoid cycle import.
*/

type User interface {
	ID() int64
	DisplayName() string
	Email() string
	GenerateActivateCode() string
	GenerateEmailActivateCode(string) string
}

type Repository interface {
	FullName() string
	HTMLURL() string
	ComposeMetas() map[string]string
}

type Issue interface {
	MailSubject() string
	Content() string
	HTMLURL() string
}

func SendUserMail(c *macaron.Context, u User, tpl, code, subject, info string) {
	data := map[string]interface{}{
		"Username":          u.DisplayName(),
		"ActiveCodeLives":   setting.Service.ActiveCodeLives / 60,
		"ResetPwdCodeLives": setting.Service.ResetPwdCodeLives / 60,
		"Code":              code,
	}
	body, err := render(tpl, data)
	if err != nil {
		log.Error("render: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email()}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, %s", u.ID(), info)

	Send(msg)
}

func SendActivateAccountMail(c *macaron.Context, u User) {
	SendUserMail(c, u, MAIL_AUTH_ACTIVATE, u.GenerateActivateCode(), c.Tr("mail.activate_account"), "activate account")
}

func SendResetPasswordMail(c *macaron.Context, u User) {
	SendUserMail(c, u, MAIL_AUTH_RESET_PASSWORD, u.GenerateActivateCode(), c.Tr("mail.reset_password"), "reset password")
}

// SendActivateAccountMail sends confirmation email.
func SendActivateEmailMail(c *macaron.Context, u User, email string) {
	data := map[string]interface{}{
		"Username":        u.DisplayName(),
		"ActiveCodeLives": setting.Service.ActiveCodeLives / 60,
		"Code":            u.GenerateEmailActivateCode(email),
		"Email":           email,
	}
	body, err := render(MAIL_AUTH_ACTIVATE_EMAIL, data)
	if err != nil {
		log.Error("HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{email}, c.Tr("mail.activate_email"), body)
	msg.Info = fmt.Sprintf("UID: %d, activate email", u.ID())

	Send(msg)
}

// SendRegisterNotifyMail triggers a notify e-mail by admin created a account.
func SendRegisterNotifyMail(c *macaron.Context, u User) {
	data := map[string]interface{}{
		"Username": u.DisplayName(),
	}
	body, err := render(MAIL_AUTH_REGISTER_NOTIFY, data)
	if err != nil {
		log.Error("HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email()}, c.Tr("mail.register_notify"), body)
	msg.Info = fmt.Sprintf("UID: %d, registration notify", u.ID())

	Send(msg)
}

// SendCollaboratorMail sends mail notification to new collaborator.
func SendCollaboratorMail(u, doer User, repo Repository) {
	subject := fmt.Sprintf("%s added you to %s", doer.DisplayName(), repo.FullName())

	data := map[string]interface{}{
		"Subject":  subject,
		"RepoName": repo.FullName(),
		"Link":     repo.HTMLURL(),
	}
	body, err := render(MAIL_NOTIFY_COLLABORATOR, data)
	if err != nil {
		log.Error("HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email()}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, add collaborator", u.ID())

	Send(msg)
}

func composeTplData(subject, body, link string) map[string]interface{} {
	data := make(map[string]interface{}, 10)
	data["Subject"] = subject
	data["Body"] = body
	data["Link"] = link
	return data
}

func composeIssueMessage(issue Issue, repo Repository, doer User, tplName string, tos []string, info string) *Message {
	subject := issue.MailSubject()
	body := string(markup.Markdown([]byte(issue.Content()), repo.HTMLURL(), repo.ComposeMetas()))
	data := composeTplData(subject, body, issue.HTMLURL())
	data["Doer"] = doer
	content, err := render(tplName, data)
	if err != nil {
		log.Error("HTMLString (%s): %v", tplName, err)
	}
	from := gomail.NewMessage().FormatAddress(setting.MailService.FromEmail, doer.DisplayName())
	msg := NewMessageFrom(tos, from, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, %s", subject, info)
	return msg
}

// SendIssueCommentMail composes and sends issue comment emails to target receivers.
func SendIssueCommentMail(issue Issue, repo Repository, doer User, tos []string) {
	if len(tos) == 0 {
		return
	}

	Send(composeIssueMessage(issue, repo, doer, MAIL_ISSUE_COMMENT, tos, "issue comment"))
}

// SendIssueMentionMail composes and sends issue mention emails to target receivers.
func SendIssueMentionMail(issue Issue, repo Repository, doer User, tos []string) {
	if len(tos) == 0 {
		return
	}
	Send(composeIssueMessage(issue, repo, doer, MAIL_ISSUE_MENTION, tos, "issue mention"))
}
