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
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/templates"
)

// Translator translates the messages of mail to the language of the recipient.
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

// Options are the settings that sending mail reads.
type Options struct {
	Email conf.EmailOpts
	// BrandName and ExternalURL are what the templates show as the name and the
	// URL of the application.
	BrandName   string
	ExternalURL string
	// LoadAssetsFromDisk loads the templates from the work directory on every
	// render instead of the embedded ones.
	LoadAssetsFromDisk bool
	// ActivateCodeLives and ResetPasswordCodeLives are the lifetimes of codes in
	// minutes.
	ActivateCodeLives      int
	ResetPasswordCodeLives int
	// Markup renders the content of issues.
	Markup markup.Options
	// HookMode blocks sending until the message is sent, so that the process
	// does not exit before.
	HookMode bool
}

// funcMap returns the template functions that show the brand name and the
// external URL.
func funcMap(brandName, externalURL string) template.FuncMap {
	return template.FuncMap{
		"AppName": func() string {
			return brandName
		},
		"AppURL": func() string {
			return externalURL
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
// relative to the "mail" directory, without extension (e.g., "auth/activate").
var mailTemplateExts = []string{".tmpl", ".html"}

// loadMailTemplates parses every mail template under the embedded "mail" tree
// (or "<work>/templates/mail" when fromDisk is set), then overlays files from
// "<custom>/templates/mail" so an admin can override any builtin. The template
// functions are bound when rendering, see render.
func loadMailTemplates(fromDisk bool) (*template.Template, error) {
	root := template.New("").Funcs(funcMap("", ""))
	parse := func(name string, data []byte) error {
		_, err := root.New(name).Parse(string(data))
		return errors.Wrapf(err, "parse %q", name)
	}

	if fromDisk {
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

func render(opts Options, tpl string, data map[string]any) (string, error) {
	set, err := mailTemplateSet(opts.LoadAssetsFromDisk)
	if err != nil {
		return "", errors.Wrap(err, "load mail templates")
	}
	set, err = set.Clone()
	if err != nil {
		return "", errors.Wrap(err, "clone mail templates")
	}
	t := set.Funcs(funcMap(opts.BrandName, opts.ExternalURL)).Lookup(tpl)
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
// restart. When assets are embedded, the set is loaded once and cached for the
// process lifetime.
func mailTemplateSet(fromDisk bool) (*template.Template, error) {
	if fromDisk {
		return loadMailTemplates(true)
	}
	tplSetOnce.Do(func() {
		tplSet, tplSetErr = loadMailTemplates(false)
	})
	return tplSet, tplSetErr
}

func SendTestMail(logger *logx.Logger, opts conf.EmailOpts, email string) error {
	msg, err := newMessage(logger, opts, []string{email}, "Gogs Test Email", "Hello 👋, greeting from Gogs!")
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	return sendMessage(opts, msg)
}

/*
	Setup interfaces of used methods in mail to avoid cycle import.
*/

type User interface {
	ID() int64
	DisplayName() string
	Email() string
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

func SendUserMail(logger *logx.Logger, opts Options, _ Translator, u User, tpl, code, subject, info string) error {
	data := map[string]any{
		"Username":          u.DisplayName(),
		"ActiveCodeLives":   opts.ActivateCodeLives / 60,
		"ResetPwdCodeLives": opts.ResetPasswordCodeLives / 60,
		"Code":              code,
	}
	body, err := render(opts, tpl, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage(logger, opts.Email, []string{u.Email()}, subject, body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, %s", u.ID(), info)

	send(opts, msg)
	return nil
}

// SendActivateAccountMail sends the code that activates the account of the
// user, see userx.GenerateActivateCode.
func SendActivateAccountMail(logger *logx.Logger, opts Options, t Translator, u User, code string) error {
	return SendUserMail(logger, opts, t, u, tmplAuthActivate, code, t.Tr("mail::activate_account"), "activate account")
}

// SendResetPasswordMail sends the code that resets the password of the user,
// see userx.GenerateResetPasswordCode.
func SendResetPasswordMail(logger *logx.Logger, opts Options, t Translator, u User, code string) error {
	return SendUserMail(logger, opts, t, u, tmplAuthResetPassword, code, t.Tr("mail::reset_password"), "reset password")
}

// SendActivateEmailMail sends the code that confirms the email address of the
// user, see userx.GenerateConfirmEmailCode.
func SendActivateEmailMail(logger *logx.Logger, opts Options, t Translator, u User, email, code string) error {
	data := map[string]any{
		"Username":        u.DisplayName(),
		"ActiveCodeLives": opts.ActivateCodeLives / 60,
		"Code":            code,
		"Email":           email,
	}
	body, err := render(opts, tmplAuthActivateEmail, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage(logger, opts.Email, []string{email}, t.Tr("mail::activate_email"), body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, activate email", u.ID())

	send(opts, msg)
	return nil
}

func SendRegisterNotifyMail(logger *logx.Logger, opts Options, t Translator, u User) error {
	data := map[string]any{
		"Username": u.DisplayName(),
	}
	body, err := render(opts, tmplAuthRegisterNotify, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage(logger, opts.Email, []string{u.Email()}, t.Tr("mail::register_notify"), body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, registration notify", u.ID())

	send(opts, msg)
	return nil
}

func SendCollaboratorMail(logger *logx.Logger, opts Options, u, doer User, repo Repository) error {
	subject := fmt.Sprintf("%s added you to %s", doer.DisplayName(), repo.FullName())

	data := map[string]any{
		"Subject":  subject,
		"RepoName": repo.FullName(),
		"Link":     repo.HTMLURL(),
	}
	body, err := render(opts, tmplNotifyCollaborator, data)
	if err != nil {
		return errors.Wrap(err, "render")
	}

	msg, err := newMessage(logger, opts.Email, []string{u.Email()}, subject, body)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, add collaborator", u.ID())

	send(opts, msg)
	return nil
}

func composeTplData(subject, body, link string) map[string]any {
	data := make(map[string]any, 10)
	data["Subject"] = subject
	data["Body"] = body
	data["Link"] = link
	return data
}

func composeIssueMessage(logger *logx.Logger, opts Options, issue Issue, repo Repository, doer User, tplName string, tos []string, info string) (*message, error) {
	subject := issue.MailSubject()
	body := string(markup.Markdown(opts.Markup, []byte(issue.Content()), repo.HTMLURL(), repo.ComposeMetas()))
	data := composeTplData(subject, body, issue.HTMLURL())
	data["Doer"] = doer
	content, err := render(opts, tplName, data)
	if err != nil {
		return nil, errors.Wrapf(err, "render %q", tplName)
	}
	from := (&mail.Address{Name: doer.DisplayName(), Address: opts.Email.FromEmail}).String()
	msg, err := newMessageFrom(logger, opts.Email, tos, from, subject, content)
	if err != nil {
		return nil, errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("Subject: %s, %s", subject, info)
	return msg, nil
}

// SendIssueCommentMail composes and sends issue comment emails to target receivers.
func SendIssueCommentMail(logger *logx.Logger, opts Options, issue Issue, repo Repository, doer User, tos []string) error {
	if len(tos) == 0 {
		return nil
	}

	msg, err := composeIssueMessage(logger, opts, issue, repo, doer, tmplIssueComment, tos, "issue comment")
	if err != nil {
		return errors.Wrap(err, "compose issue message")
	}
	send(opts, msg)
	return nil
}

// SendIssueMentionMail composes and sends issue mention emails to target receivers.
func SendIssueMentionMail(logger *logx.Logger, opts Options, issue Issue, repo Repository, doer User, tos []string) error {
	if len(tos) == 0 {
		return nil
	}
	msg, err := composeIssueMessage(logger, opts, issue, repo, doer, tmplIssueMention, tos, "issue mention")
	if err != nil {
		return errors.Wrap(err, "compose issue message")
	}
	send(opts, msg)
	return nil
}
