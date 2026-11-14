package email

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	"io/fs"
	"net/mail"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	texttemplate "text/template"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/inbucket/html2text"
	"github.com/unknwon/i18n"

	embedconf "gogs.io/gogs/conf"
	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/markup"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/templates"
)

// Locale is the language of the person an email is for. It decouples this
// package from any specific web framework, e.g., both macaron.Context and
// Flamego's i18n.Locale satisfy it.
type Locale interface {
	Language() string
}

// defaultLang is the language of emails to several people at once, e.g.,
// issue comments, and of emails with no person behind them.
const defaultLang = "en-US"

var localesOnce sync.Once

// loadLocales loads the locale files for translating emails unless the web
// server already did, e.g., when a Git hook sends issue emails.
func loadLocales() {
	localesOnce.Do(func() {
		if i18n.Count() > 0 {
			return
		}
		langs, names := []string{defaultLang}, []string{"English"}
		if conf.I18n != nil && len(conf.I18n.Langs) > 0 {
			langs, names = conf.I18n.Langs, conf.I18n.Names
		}
		for i, lang := range langs {
			fname := "locale_" + lang + ".ini"
			data, err := embedconf.Files.ReadFile("locale/" + fname)
			if err != nil {
				continue
			}
			var custom []any
			if p := filepath.Join(conf.CustomDir(), "conf", "locale", fname); osx.IsFile(p) {
				custom = append(custom, p)
			}
			_ = i18n.SetMessageWithDesc(lang, names[i], data, custom...)
		}
		i18n.SetDefaultLang(defaultLang)
	})
}

// funcs returns the functions shared by the HTML and plain text templates. Tr
// and Lang stand for the language of each email, see templateSet.render.
func funcs() map[string]any {
	return map[string]any{
		"AppName": func() string {
			return conf.App.BrandName
		},
		"AppURL": func() string {
			return conf.Server.ExternalURL
		},
		"Year": func() int {
			return time.Now().Year()
		},
		"Lang": func() string {
			return defaultLang
		},
		"Tr": func(key string, args ...any) string {
			return i18n.Tr(defaultLang, key, args...)
		},
		// dict passes several values to a shared part of the templates, e.g.,
		// {{template "button" (dict "URL" $link "Label" (Tr "mail.x"))}}.
		"dict": func(pairs ...any) (map[string]any, error) {
			if len(pairs)%2 != 0 {
				return nil, errors.New("dict wants pairs of keys and values")
			}
			m := make(map[string]any, len(pairs)/2)
			for i := 0; i < len(pairs); i += 2 {
				key, ok := pairs[i].(string)
				if !ok {
					return nil, errors.Newf("dict key %v is not a string", pairs[i])
				}
				m[key] = pairs[i+1]
			}
			return m, nil
		},
	}
}

func htmlFuncs() htmltemplate.FuncMap {
	fm := funcs()
	// Markup prints HTML rendered from user content, e.g., an issue comment.
	fm["Markup"] = func(raw string) htmltemplate.HTML {
		return htmltemplate.HTML(markup.Sanitize(raw))
	}
	// Str2HTML is the name of Markup in templates written before it.
	fm["Str2HTML"] = fm["Markup"]
	return fm
}

func textFuncs() texttemplate.FuncMap {
	fm := funcs()
	fm["Markup"] = func(raw string) (string, error) {
		return html2text.FromString(markup.Sanitize(raw))
	}
	return fm
}

// A template's name is its path relative to the "mail" directory, without
// extension (e.g. "auth/activate"). HTML templates end with .tmpl or .html,
// plain text ones with .txt.
var (
	htmlTemplateExts = []string{".tmpl", ".html"}
	textTemplateExt  = ".txt"
)

// mailLogoName is the file name of the logo that the emails show, and its
// content ID: the HTML templates show it with <img src="cid:logo.png">.
const mailLogoName = "logo.png"

// templateSet holds every mail template.
type templateSet struct {
	html *htmltemplate.Template
	text *texttemplate.Template
	logo []byte
	// htmlOnly holds the names of HTML templates overridden in the custom
	// directory without a plain text one. Their plain text is converted from
	// the HTML, so that it does not differ from the HTML.
	htmlOnly map[string]bool
}

// loadMailTemplates parses every mail template under the embedded "mail" tree
// (or "<work>/templates/mail" when LoadAssetsFromDisk is set), then overlays
// files from "<custom>/templates/mail" so an admin can override any builtin.
func loadMailTemplates() (*templateSet, error) {
	set := &templateSet{
		html:     htmltemplate.New("").Funcs(htmlFuncs()),
		text:     texttemplate.New("").Funcs(textFuncs()),
		htmlOnly: make(map[string]bool),
	}
	customText := make(map[string]bool)
	parse := func(custom bool) func(name, ext string, data []byte) error {
		return func(name, ext string, data []byte) error {
			var err error
			if ext == textTemplateExt {
				_, err = set.text.New(name).Parse(string(data))
				if custom {
					customText[name] = true
				}
			} else {
				_, err = set.html.New(name).Parse(string(data))
				if custom {
					set.htmlOnly[name] = true
				}
			}
			return errors.Wrapf(err, "parse %q", name+ext)
		}
	}

	if conf.Server.LoadAssetsFromDisk {
		baseRoot := filepath.Join(conf.WorkDir(), "templates", "mail")
		if _, err := os.Stat(baseRoot); err != nil {
			return nil, errors.Wrapf(err, "stat base mail templates %q", baseRoot)
		}
		if err := overlayDiskMailTemplates(baseRoot, parse(false)); err != nil {
			return nil, err
		}
	} else {
		for _, name := range templates.MailFileNames() {
			ext := mailTemplateExt(name)
			if ext == "" {
				continue
			}
			data, err := templates.ReadMailFile(name)
			if err != nil {
				return nil, errors.Wrapf(err, "read embedded %q", name)
			}
			if err := parse(false)(strings.TrimSuffix(filepath.ToSlash(name), ext), ext, data); err != nil {
				return nil, err
			}
		}
	}
	if err := overlayDiskMailTemplates(filepath.Join(conf.CustomDir(), "templates", "mail"), parse(true)); err != nil {
		return nil, err
	}
	for name := range customText {
		delete(set.htmlOnly, name)
	}

	var err error
	set.logo, err = loadMailLogo()
	if err != nil {
		return nil, errors.Wrap(err, "load logo")
	}
	return set, nil
}

// loadMailLogo returns the logo that the emails show: logo.png in
// "<custom>/templates/mail", else the custom favicon of the site, else the
// builtin logo.
func loadMailLogo() ([]byte, error) {
	for _, p := range []string{
		filepath.Join(conf.CustomDir(), "templates", "mail", mailLogoName),
		filepath.Join(conf.CustomDir(), "public", "img", "favicon.png"),
	} {
		if osx.IsFile(p) {
			data, err := os.ReadFile(p)
			return data, errors.Wrapf(err, "read %q", p)
		}
	}
	if conf.Server.LoadAssetsFromDisk {
		p := filepath.Join(conf.WorkDir(), "templates", "mail", mailLogoName)
		data, err := os.ReadFile(p)
		return data, errors.Wrapf(err, "read %q", p)
	}
	data, err := templates.ReadMailFile(mailLogoName)
	return data, errors.Wrap(err, "read embedded logo")
}

// mailTemplateExt returns the extension of a recognized template file, or an
// empty string.
func mailTemplateExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == textTemplateExt || slices.Contains(htmlTemplateExts, ext) {
		return ext
	}
	return ""
}

// overlayDiskMailTemplates walks root and parses every recognized template
// file via parse. A missing root is not an error: custom overrides are optional.
func overlayDiskMailTemplates(root string, parse func(name, ext string, data []byte) error) error {
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
		ext := mailTemplateExt(p)
		if ext == "" {
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
		return parse(strings.TrimSuffix(filepath.ToSlash(rel), ext), ext, data)
	})
}

var blankLines = regexp.MustCompile(`\n{3,}`)

// render executes the named template in the language, and returns the HTML
// and plain text bodies of the email.
func (s *templateSet) render(lang, name string, data any) (htmlBody, textBody string, err error) {
	langFuncs := map[string]any{
		"Tr": func(key string, args ...any) string {
			return i18n.Tr(lang, key, args...)
		},
		"Lang": func() string {
			return lang
		},
	}

	// Clones keep the parsed set free of any language, so it can be cloned again.
	htmlSet, err := s.html.Clone()
	if err != nil {
		return "", "", errors.Wrap(err, "clone HTML templates")
	}
	t := htmlSet.Funcs(langFuncs).Lookup(name)
	if t == nil {
		return "", "", errors.Newf("mail template %q not found", name)
	}
	var buf bytes.Buffer
	if err = t.Execute(&buf, data); err != nil {
		return "", "", errors.Wrapf(err, "execute %q", name)
	}
	htmlBody = buf.String()

	if s.text.Lookup(name) == nil || s.htmlOnly[name] {
		textBody, err = html2text.FromString(htmlBody)
		return htmlBody, textBody, errors.Wrap(err, "convert HTML to plain text")
	}
	textSet, err := s.text.Clone()
	if err != nil {
		return "", "", errors.Wrap(err, "clone plain text templates")
	}
	buf.Reset()
	if err = textSet.Funcs(langFuncs).Lookup(name).Execute(&buf, data); err != nil {
		return "", "", errors.Wrapf(err, "execute %q", name+textTemplateExt)
	}
	// Elements the template leaves out, e.g., an empty paragraph, leave runs of
	// blank lines behind.
	textBody = blankLines.ReplaceAllString(strings.TrimSpace(buf.String()), "\n\n") + "\n"
	return htmlBody, textBody, nil
}

var (
	tplSet     *templateSet
	tplSetOnce sync.Once
	tplSetErr  error
)

// mailTemplateSet returns the parsed template set. When assets are loaded from
// disk, templates are reloaded on every call so admin edits under
// <work>/templates/mail or <custom>/templates/mail take effect without a
// restart, matching the hot-reload behavior of the previous macaron renderer
// for non-production environments. When assets are embedded, the set is loaded
// once and cached for the process lifetime.
func mailTemplateSet() (*templateSet, error) {
	if conf.Server.LoadAssetsFromDisk {
		return loadMailTemplates()
	}
	tplSetOnce.Do(func() {
		tplSet, tplSetErr = loadMailTemplates()
	})
	return tplSet, tplSetErr
}

// Mail is a composed email.
type Mail struct {
	Subject string
	HTML    string
	Text    string
	// Logo is the image the HTML shows as cid:logo.png, or nil when it shows
	// no logo, e.g., a custom template without one.
	Logo []byte
}

// composeMail renders the named template in the language.
func composeMail(lang, name, subject string, data any) (*Mail, error) {
	set, err := mailTemplateSet()
	if err != nil {
		return nil, errors.Wrap(err, "load mail templates")
	}
	htmlBody, textBody, err := set.render(lang, name, data)
	if err != nil {
		return nil, err
	}
	m := &Mail{Subject: subject, HTML: htmlBody, Text: textBody}
	if strings.Contains(htmlBody, "cid:"+mailLogoName) {
		m.Logo = set.logo
	}
	return m, nil
}

func SendTestMail(l Locale, email string) error {
	m, err := testMail.compose(l.Language(), struct{}{})
	if err != nil {
		return errors.Wrap(err, "compose")
	}
	msg, err := newMessage([]string{email}, m)
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
	GenerateEmailResetPasswordCode(string) string
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

// sendUserMail sends an email to the user.
func sendUserMail[T any](lang string, u User, to string, d definition[T], data T, info string) error {
	m, err := d.compose(lang, data)
	if err != nil {
		return errors.Wrap(err, "compose")
	}
	msg, err := newMessage([]string{to}, m)
	if err != nil {
		return errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("UID: %d, %s", u.ID(), info)

	send(msg)
	return nil
}

func SendActivateAccountMail(l Locale, u User) error {
	return sendUserMail(l.Language(), u, u.Email(), activateAccountMail, newUserCodeData(u, u.GenerateEmailActivateCode(u.Email())), "activate account")
}

func SendResetPasswordMail(l Locale, u User) error {
	return sendUserMail(l.Language(), u, u.Email(), resetPasswordMail, newUserCodeData(u, u.GenerateEmailResetPasswordCode(u.Email())), "reset password")
}

func SendActivateEmailMail(l Locale, u User, email string) error {
	data := activateEmailData{
		Username:        u.DisplayName(),
		ActiveCodeLives: conf.Auth.ActivateCodeLives / 60,
		Code:            u.GenerateEmailActivateCode(email),
		Email:           email,
	}
	return sendUserMail(l.Language(), u, email, activateEmailMail, data, "activate email")
}

func SendRegisterNotifyMail(l Locale, u User) error {
	return sendUserMail(l.Language(), u, u.Email(), registerNotifyMail, registerNotifyData{Username: u.DisplayName()}, "registration notify")
}

func SendCollaboratorMail(u, doer User, repo Repository) error {
	data := newCollaboratorData(defaultLang, doer.DisplayName(), repo.FullName(), repo.HTMLURL())
	return sendUserMail(defaultLang, u, u.Email(), collaboratorMail, data, "add collaborator")
}

func composeIssueMessage(issue Issue, repo Repository, doer User, d definition[issueData], tos []string, info string) (*message, error) {
	data := issueData{
		Subject: issue.MailSubject(),
		Body:    string(markup.Markdown([]byte(issue.Content()), repo.HTMLURL(), repo.ComposeMetas())),
		Link:    issue.HTMLURL(),
		Doer:    doer,
	}
	m, err := d.compose(defaultLang, data)
	if err != nil {
		return nil, errors.Wrapf(err, "compose %q", d.name)
	}
	from := (&mail.Address{Name: doer.DisplayName(), Address: conf.Email.FromEmail}).String()
	msg, err := newMessageFrom(tos, from, m)
	if err != nil {
		return nil, errors.Wrap(err, "new message")
	}
	msg.info = fmt.Sprintf("Subject: %s, %s", m.Subject, info)
	return msg, nil
}

// SendIssueCommentMail composes and sends issue comment emails to target receivers.
func SendIssueCommentMail(issue Issue, repo Repository, doer User, tos []string) error {
	if len(tos) == 0 {
		return nil
	}

	msg, err := composeIssueMessage(issue, repo, doer, issueCommentMail, tos, "issue comment")
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
	msg, err := composeIssueMessage(issue, repo, doer, issueMentionMail, tos, "issue mention")
	if err != nil {
		return errors.Wrap(err, "compose issue message")
	}
	send(msg)
	return nil
}
