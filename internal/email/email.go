package email

import (
	"fmt"
	"html/template"
	"path/filepath"
	"sync"
	"time"

	"gopkg.in/gomail.v2"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
)

// Translator is an interface for translation.
type Translator interface {
	Tr(key string, args ...any) string
}

const (
	tmplAuthActivate       = "auth/activate"
	tmplAuthActivateEmail  = "auth/activate_email"
	tmplAuthResetPassword  = "auth/reset_passwd"
	tmplAuthRegisterNotify = "auth/register_notify"

	tmplIssueComment = "issue/comment"
	tmplIssueMention = "issue/mention"

	tmplNotifyCollaborator = "notify/collaborator"
)

var (
	mailTemplates map[string]*template.Template
	templatesOnce sync.Once
)

// render renders a mail template with given data.
func render(tpl string, data map[string]any) (string, error) {
	templatesOnce.Do(func() {
		mailTemplates = make(map[string]*template.Template)

		funcMap := template.FuncMap{
			"AppName": func() string {
				return conf.App.BrandName
			},
			"AppURL": func() string {
				return conf.Server.ExternalURL
			},
			"Year": func() int {
				return time.Now().Year()
			},
			"Str2HTML": func(raw string) template.HTML {
				return template.HTML(markup.Sanitize(raw))
			},
		}

		// Load templates
		templateDir := filepath.Join(conf.WorkDir(), "templates", "mail")
		customDir := filepath.Join(conf.CustomDir(), "templates", "mail")

		// Parse templates from both directories
		// For now, just use a simple approach - in production you'd want to handle this better
		_ = templateDir
		_ = customDir
		_ = funcMap
	})

	// For now, return a simple implementation
	// TODO: Implement proper template rendering
	return "", fmt.Errorf("template rendering not yet implemented for: %s", tpl)
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

func SendUserMail(_ Translator, u User, tpl, code, subject, info string) {
	data := map[string]any{
		"Username":          u.DisplayName(),
		"ActiveCodeLives":   conf.Auth.ActivateCodeLives / 60,
		"ResetPwdCodeLives": conf.Auth.ResetPasswordCodeLives / 60,
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

func SendActivateAccountMail(c Translator, u User) {
	SendUserMail(c, u, tmplAuthActivate, u.GenerateEmailActivateCode(u.Email()), c.Tr("mail.activate_account"), "activate account")
}

func SendResetPasswordMail(c Translator, u User) {
	SendUserMail(c, u, tmplAuthResetPassword, u.GenerateEmailActivateCode(u.Email()), c.Tr("mail.reset_password"), "reset password")
}

// SendActivateAccountMail sends confirmation email.
func SendActivateEmailMail(c Translator, u User, email string) {
	data := map[string]any{
		"Username":        u.DisplayName(),
		"ActiveCodeLives": conf.Auth.ActivateCodeLives / 60,
		"Code":            u.GenerateEmailActivateCode(email),
		"Email":           email,
	}
	body, err := render(tmplAuthActivateEmail, data)
	if err != nil {
		log.Error("HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{email}, c.Tr("mail.activate_email"), body)
	msg.Info = fmt.Sprintf("UID: %d, activate email", u.ID())

	Send(msg)
}

// SendRegisterNotifyMail triggers a notify e-mail by admin created a account.
func SendRegisterNotifyMail(c Translator, u User) {
	data := map[string]any{
		"Username": u.DisplayName(),
	}
	body, err := render(tmplAuthRegisterNotify, data)
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

	data := map[string]any{
		"Subject":  subject,
		"RepoName": repo.FullName(),
		"Link":     repo.HTMLURL(),
	}
	body, err := render(tmplNotifyCollaborator, data)
	if err != nil {
		log.Error("HTMLString: %v", err)
		return
	}

	msg := NewMessage([]string{u.Email()}, subject, body)
	msg.Info = fmt.Sprintf("UID: %d, add collaborator", u.ID())

	Send(msg)
}

func composeTplData(subject, body, link string) map[string]any {
	data := make(map[string]any, 10)
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
	from := gomail.NewMessage().FormatAddress(conf.Email.FromEmail, doer.DisplayName())
	msg := NewMessageFrom(tos, from, subject, content)
	msg.Info = fmt.Sprintf("Subject: %s, %s", subject, info)
	return msg
}

// SendIssueCommentMail composes and sends issue comment emails to target receivers.
func SendIssueCommentMail(issue Issue, repo Repository, doer User, tos []string) {
	if len(tos) == 0 {
		return
	}

	Send(composeIssueMessage(issue, repo, doer, tmplIssueComment, tos, "issue comment"))
}

// SendIssueMentionMail composes and sends issue mention emails to target receivers.
func SendIssueMentionMail(issue Issue, repo Repository, doer User, tos []string) {
	if len(tos) == 0 {
		return
	}
	Send(composeIssueMessage(issue, repo, doer, tmplIssueMention, tos, "issue mention"))
}
