package email

import (
	"fmt"
	"html/template"
	"net/mail"
	"path/filepath"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/templates"
)

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
	tplRender     *macaron.TplRender
	tplRenderOnce sync.Once
)

// render renders a mail template with given data.
func render(tpl string, data map[string]any) (string, error) {
	tplRenderOnce.Do(func() {
		customDir := filepath.Join(conf.CustomDir(), "templates")
		opt := &macaron.RenderOptions{
			Directory:         filepath.Join(conf.WorkDir(), "templates", "mail"),
			AppendDirectories: []string{filepath.Join(customDir, "mail")},
			Extensions:        []string{".tmpl", ".html"},
			Funcs: []template.FuncMap{map[string]any{
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
			}},
		}
		if !conf.Server.LoadAssetsFromDisk {
			opt.TemplateFileSystem = templates.NewTemplateFileSystem("mail", customDir)
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
	msg, err := newMessage([]string{email}, "Gogs Test Email", "Hello 👋, greeting from Gogs!")
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	return sendMessage(msg)
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

func SendUserMail(_ *macaron.Context, u User, tpl, code, subject, info string) error {
	data := map[string]any{
		"Username":          u.DisplayName(),
		"ActiveCodeLives":   conf.Auth.ActivateCodeLives / 60,
		"ResetPwdCodeLives": conf.Auth.ResetPasswordCodeLives / 60,
		"Code":              code,
	}
	body, err := render(tpl, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage([]string{u.Email()}, subject, body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, %s", u.ID(), info)

	send(msg)
	return nil
}

func SendActivateAccountMail(c *macaron.Context, u User) error {
	return SendUserMail(c, u, tmplAuthActivate, u.GenerateEmailActivateCode(u.Email()), c.Tr("mail.activate_account"), "activate account")
}

func SendResetPasswordMail(c *macaron.Context, u User) error {
	return SendUserMail(c, u, tmplAuthResetPassword, u.GenerateEmailActivateCode(u.Email()), c.Tr("mail.reset_password"), "reset password")
}

func SendActivateEmailMail(c *macaron.Context, u User, email string) error {
	data := map[string]any{
		"Username":        u.DisplayName(),
		"ActiveCodeLives": conf.Auth.ActivateCodeLives / 60,
		"Code":            u.GenerateEmailActivateCode(email),
		"Email":           email,
	}
	body, err := render(tmplAuthActivateEmail, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage([]string{email}, c.Tr("mail.activate_email"), body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, activate email", u.ID())

	send(msg)
	return nil
}

func SendRegisterNotifyMail(c *macaron.Context, u User) error {
	data := map[string]any{
		"Username": u.DisplayName(),
	}
	body, err := render(tmplAuthRegisterNotify, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage([]string{u.Email()}, c.Tr("mail.register_notify"), body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, registration notify", u.ID())

	send(msg)
	return nil
}

func SendCollaboratorMail(u, doer User, repo Repository) error {
	subject := fmt.Sprintf("%s added you to %s", doer.DisplayName(), repo.FullName())

	data := map[string]any{
		"Subject":  subject,
		"RepoName": repo.FullName(),
		"Link":     repo.HTMLURL(),
	}
	body, err := render(tmplNotifyCollaborator, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage([]string{u.Email()}, subject, body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, add collaborator", u.ID())

	send(msg)
	return nil
}

func composeTplData(subject, body, link string) map[string]any {
	data := make(map[string]any, 10)
	data["Subject"] = subject
	data["Body"] = body
	data["Link"] = link
	return data
}

func composeIssueMessage(issue Issue, repo Repository, doer User, tplName string, tos []string, info string) (*message, error) {
	subject := issue.MailSubject()
	body := string(markup.Markdown([]byte(issue.Content()), repo.HTMLURL(), repo.ComposeMetas()))
	data := composeTplData(subject, body, issue.HTMLURL())
	data["Doer"] = doer
	content, err := render(tplName, data)
	if err != nil {
		return nil, errors.Wrapf(err, "render %q", tplName)
	}
	from := (&mail.Address{Name: doer.DisplayName(), Address: conf.Email.FromEmail}).String()
	msg, err := newMessageFrom(tos, from, subject, content)
	if err != nil {
		return nil, errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("Subject: %s, %s", subject, info)
	return msg, nil
}

// SendIssueCommentMail composes and sends issue comment emails to target receivers.
func SendIssueCommentMail(issue Issue, repo Repository, doer User, tos []string) error {
	if len(tos) == 0 {
		return nil
	}

	msg, err := composeIssueMessage(issue, repo, doer, tmplIssueComment, tos, "issue comment")
	if err != nil {
		return errors.Wrap(err, "compose issue message")
	}
	send(msg)
	return nil
}

// SendIssueMentionMail composes and sends issue mention emails to target receivers.
func SendIssueMentionMail(issue Issue, repo Repository, doer User, tos []string) error {
	if len(tos) == 0 {
		return nil
	}
	msg, err := composeIssueMessage(issue, repo, doer, tmplIssueMention, tos, "issue mention")
	if err != nil {
		return errors.Wrap(err, "compose issue message")
	}
	send(msg)
	return nil
}
