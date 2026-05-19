package email

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/mail"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/templates"
)

// Translator is the minimal locale-translation contract used by mail
// composition. It decouples this package from any specific web framework so
// callers can pass either macaron.Context or, post-migration, Flamego's
// i18n.Locale.
type Translator interface {
	Tr(format string, args ...any) string
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
	tplSet     *template.Template
	tplSetOnce sync.Once
	tplSetErr  error
)

func funcMap() template.FuncMap {
	return template.FuncMap{
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
}

// Recognized mail-template file extensions. A template's name is its path
// relative to the "mail" directory, without extension (e.g. "auth/activate").
var mailTemplateExts = []string{".tmpl", ".html"}

// loadMailTemplates parses every mail template under the embedded "mail" tree
// (or "<work>/templates/mail" when LoadAssetsFromDisk is set), then overlays
// files from "<custom>/templates/mail" so an admin can override any builtin.
func loadMailTemplates() (*template.Template, error) {
	root := template.New("").Funcs(funcMap())
	parse := func(name string, data []byte) error {
		_, err := root.New(name).Parse(string(data))
		return errors.Wrapf(err, "parse %q", name)
	}

	if conf.Server.LoadAssetsFromDisk {
		baseRoot := filepath.Join(conf.WorkDir(), "templates", "mail")
		if _, err := os.Stat(baseRoot); err != nil {
			return nil, errors.Wrapf(err, "stat base mail templates %q", baseRoot)
		}
		if err := overlayDiskMailTemplates(baseRoot, parse); err != nil {
			return nil, err
		}
	} else {
		for _, name := range templates.MailFileNames() {
			ext := strings.ToLower(filepath.Ext(name))
			if !slices.Contains(mailTemplateExts, ext) {
				continue
			}
			data, err := templates.ReadMailFile(name)
			if err != nil {
				return nil, errors.Wrapf(err, "read embedded %q", name)
			}
			if err := parse(strings.TrimSuffix(filepath.ToSlash(name), ext), data); err != nil {
				return nil, err
			}
		}
	}
	if err := overlayDiskMailTemplates(filepath.Join(conf.CustomDir(), "templates", "mail"), parse); err != nil {
		return nil, err
	}
	return root, nil
}

// overlayDiskMailTemplates walks root and parses every recognized template
// file via parse. A missing root is not an error: custom overrides are optional.
func overlayDiskMailTemplates(root string, parse func(name string, data []byte) error) error {
	return filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(p))
		if !slices.Contains(mailTemplateExts, ext) {
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return errors.Wrapf(err, "read %q", p)
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		return parse(strings.TrimSuffix(filepath.ToSlash(rel), ext), data)
	})
}

func render(tpl string, data map[string]any) (string, error) {
	set, err := mailTemplateSet()
	if err != nil {
		return "", errors.Wrap(err, "load mail templates")
	}
	t := set.Lookup(tpl)
	if t == nil {
		return "", errors.Newf("mail template %q not found", tpl)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", errors.Wrapf(err, "execute %q", tpl)
	}
	return buf.String(), nil
}

// mailTemplateSet returns the parsed template set. When assets are loaded from
// disk, templates are reloaded on every call so admin edits under
// <work>/templates/mail or <custom>/templates/mail take effect without a
// restart — matching the hot-reload behavior of the previous macaron renderer
// for non-production environments. When assets are embedded, the set is loaded
// once and cached for the process lifetime.
func mailTemplateSet() (*template.Template, error) {
	if conf.Server.LoadAssetsFromDisk {
		return loadMailTemplates()
	}
	tplSetOnce.Do(func() {
		tplSet, tplSetErr = loadMailTemplates()
	})
	return tplSet, tplSetErr
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

func SendUserMail(_ Translator, u User, tpl, code, subject, info string) error {
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

func SendActivateAccountMail(t Translator, u User) error {
	return SendUserMail(t, u, tmplAuthActivate, u.GenerateEmailActivateCode(u.Email()), t.Tr("mail.activate_account"), "activate account")
}

func SendResetPasswordMail(t Translator, u User) error {
	return SendUserMail(t, u, tmplAuthResetPassword, u.GenerateEmailActivateCode(u.Email()), t.Tr("mail.reset_password"), "reset password")
}

func SendActivateEmailMail(t Translator, u User, email string) error {
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

	msg, err := newMessage([]string{email}, t.Tr("mail.activate_email"), body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, activate email", u.ID())

	send(msg)
	return nil
}

func SendRegisterNotifyMail(t Translator, u User) error {
	data := map[string]any{
		"Username": u.DisplayName(),
	}
	body, err := render(tmplAuthRegisterNotify, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage([]string{u.Email()}, t.Tr("mail.register_notify"), body)
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
