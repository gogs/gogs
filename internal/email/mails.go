package email

import (
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/unknwon/i18n"

	"gogs.io/gogs/internal/conf"
)

// definition is an email that a template under templates/mail renders with
// data of type T. Admins can override the templates in the custom directory,
// so the fields of the data types are kept for the templates they wrote.
type definition[T any] struct {
	name    string
	subject func(lang string, data T) string
	// sample returns made-up data to preview the email with.
	sample func(lang string) T
}

func (d definition[T]) compose(lang string, data T) (*Mail, error) {
	loadLocales()
	return composeMail(lang, d.name, d.subject(lang, data), data)
}

func (d definition[T]) templateName() string {
	return d.name
}

func (d definition[T]) preview(lang string) (*Mail, error) {
	return d.compose(lang, d.sample(lang))
}

type previewer interface {
	templateName() string
	preview(lang string) (*Mail, error)
}

func translatedSubject[T any](key string) func(lang string, data T) string {
	return func(lang string, _ T) string {
		return i18n.Tr(lang, key)
	}
}

func dataSubject[T interface{ subject() string }](_ string, data T) string {
	return data.subject()
}

type userCodeData struct {
	Username          string
	ActiveCodeLives   int
	ResetPwdCodeLives int
	Code              string
}

func newUserCodeData(u User, code string) userCodeData {
	return userCodeData{
		Username:          u.DisplayName(),
		ActiveCodeLives:   conf.Auth.ActivateCodeLives / 60,
		ResetPwdCodeLives: conf.Auth.ResetPasswordCodeLives / 60,
		Code:              code,
	}
}

type activateEmailData struct {
	Username        string
	ActiveCodeLives int
	Code            string
	Email           string
}

type registerNotifyData struct {
	Username string
}

type collaboratorData struct {
	Subject  string
	RepoName string
	Link     string
}

func newCollaboratorData(lang, doerName, repoName, link string) collaboratorData {
	loadLocales()
	return collaboratorData{
		Subject:  i18n.Tr(lang, "mail.collaborator", doerName, repoName),
		RepoName: repoName,
		Link:     link,
	}
}

func (d collaboratorData) subject() string { return d.Subject }

type issueData struct {
	Subject string
	// Body is HTML rendered from the Markdown of the issue or comment.
	Body string
	Link string
	Doer User
}

func (d issueData) subject() string { return d.Subject }

// sampleUser is the made-up person in previews.
type sampleUser struct{}

func (sampleUser) ID() int64                                    { return 1 }
func (sampleUser) DisplayName() string                          { return "alice" }
func (sampleUser) Email() string                                { return "alice@example.com" }
func (sampleUser) GenerateEmailActivateCode(string) string      { return sampleCode }
func (sampleUser) GenerateEmailResetPasswordCode(string) string { return sampleCode }

const sampleCode = "202611121200001440c0ffee0c0ffee0c0ffee0c0ffee0c0ffee0c0ffee616c696365"

func sampleIssue(_ string) issueData {
	return issueData{
		Subject: "[alice/hello-world] Pushing an empty commit fails (#7)",
		Body:    "<p>Thanks for the report! I can reproduce it on <code>main</code>, and <a href=\"#\">the fix</a> is on its way.</p>",
		Link:    conf.Server.ExternalURL + "alice/hello-world/issues/7",
		Doer:    sampleUser{},
	}
}

var (
	activateAccountMail = definition[userCodeData]{
		name:    "auth/activate",
		subject: translatedSubject[userCodeData]("mail.activate_account"),
		sample:  func(string) userCodeData { return newUserCodeData(sampleUser{}, sampleCode) },
	}
	resetPasswordMail = definition[userCodeData]{
		name:    "auth/reset_passwd",
		subject: translatedSubject[userCodeData]("mail.reset_password"),
		sample:  func(string) userCodeData { return newUserCodeData(sampleUser{}, sampleCode) },
	}
	activateEmailMail = definition[activateEmailData]{
		name:    "auth/activate_email",
		subject: translatedSubject[activateEmailData]("mail.activate_email"),
		sample: func(string) activateEmailData {
			return activateEmailData{
				Username:        "alice",
				ActiveCodeLives: conf.Auth.ActivateCodeLives / 60,
				Code:            sampleCode,
				Email:           "alice@work.example.com",
			}
		},
	}
	registerNotifyMail = definition[registerNotifyData]{
		name:    "auth/register_notify",
		subject: translatedSubject[registerNotifyData]("mail.register_notify"),
		sample:  func(string) registerNotifyData { return registerNotifyData{Username: "alice"} },
	}
	collaboratorMail = definition[collaboratorData]{
		name:    "notify/collaborator",
		subject: dataSubject[collaboratorData],
		sample: func(lang string) collaboratorData {
			return newCollaboratorData(lang, "bob", "bob/hello-world", conf.Server.ExternalURL+"bob/hello-world")
		},
	}
	issueCommentMail = definition[issueData]{
		name:    "issue/comment",
		subject: dataSubject[issueData],
		sample:  sampleIssue,
	}
	issueMentionMail = definition[issueData]{
		name:    "issue/mention",
		subject: dataSubject[issueData],
		sample:  sampleIssue,
	}
	testMail = definition[struct{}]{
		name: "admin/test_mail",
		subject: func(lang string, _ struct{}) string {
			return i18n.Tr(lang, "mail.test_mail", conf.App.BrandName)
		},
		sample: func(string) struct{} { return struct{}{} },
	}
)

// previews lists every email the server sends, in the order the admin panel
// shows them.
var previews = []previewer{
	activateAccountMail,
	activateEmailMail,
	resetPasswordMail,
	registerNotifyMail,
	collaboratorMail,
	issueCommentMail,
	issueMentionMail,
	AlertNewSignIn,
	AlertPasswordChanged,
	AlertEmailChanged,
	AlertTwoFactorDisabled,
	AlertSSHKeyAdded,
	AlertAccessTokenAdded,
	AlertPasskeyAdded,
	testMail,
}

// Names returns the names of all emails, e.g., "auth/activate".
func Names() []string {
	names := make([]string, len(previews))
	for i, p := range previews {
		names[i] = p.templateName()
	}
	return names
}

// ErrNotFound is returned by Preview for a name that is not an email.
var ErrNotFound = errors.New("email not found")

// Preview composes the named email in the language with made-up data, with
// the templates overridden in the custom directory.
func Preview(name, lang string) (*Mail, error) {
	for _, p := range previews {
		if p.templateName() != name {
			continue
		}
		m, err := p.preview(lang)
		if err != nil || m.Logo == nil {
			return m, err
		}
		// A page cannot show cid: images, so the logo goes into the HTML.
		dataURI := "data:" + http.DetectContentType(m.Logo) + ";base64," + base64.StdEncoding.EncodeToString(m.Logo)
		m.HTML = strings.ReplaceAll(m.HTML, "cid:"+mailLogoName, dataURI)
		return m, nil
	}
	return nil, ErrNotFound
}

// Activity is where a change to an account came from.
type Activity struct {
	UserAgent string
	IP        string
	Time      time.Time
}

type securityAlertData struct {
	Username string
	// Name names what changed, e.g., the name of the new SSH key or the new
	// email address.
	Name string
	// Time is when it changed, e.g., "2026-11-12 15:04 UTC".
	Time string
	// Browser names the browser and the system it changed from, e.g., "Firefox
	// on macOS".
	Browser string
	IP      string
}

func newSecurityAlertData(lang string, u User, a Activity, name string) securityAlertData {
	loadLocales()
	return securityAlertData{
		Username: u.DisplayName(),
		Name:     name,
		Time:     a.Time.UTC().Format("2006-01-02 15:04 MST"),
		Browser:  describeUserAgent(lang, a.UserAgent),
		IP:       a.IP,
	}
}

type namedPattern struct {
	re   *regexp.Regexp
	name string
}

// Order matters: Edge and Opera also claim to be Chrome, and Chrome claims to
// be Safari.
var (
	userAgentBrowsers = []namedPattern{
		{regexp.MustCompile(`Edg(?:e|A|iOS)?/`), "Edge"},
		{regexp.MustCompile(`OPR/|Opera`), "Opera"},
		{regexp.MustCompile(`Firefox/|FxiOS/`), "Firefox"},
		{regexp.MustCompile(`Chrome/|CriOS/`), "Chrome"},
		{regexp.MustCompile(`Safari/`), "Safari"},
	}
	userAgentSystems = []namedPattern{
		{regexp.MustCompile(`iPhone|iPad|iPod`), "iOS"},
		{regexp.MustCompile(`Android`), "Android"},
		{regexp.MustCompile(`CrOS`), "ChromeOS"},
		{regexp.MustCompile(`Mac OS X|Macintosh`), "macOS"},
		{regexp.MustCompile(`Windows`), "Windows"},
		{regexp.MustCompile(`Linux`), "Linux"},
	}
)

func findPattern(patterns []namedPattern, s string) string {
	for _, p := range patterns {
		if p.re.MatchString(s) {
			return p.name
		}
	}
	return ""
}

// describeUserAgent names the browser and the system of a user agent string,
// like describeUserAgent in web/src/lib/user-agent.ts. It never returns the
// user agent itself, which whoever made the change chose.
func describeUserAgent(lang, ua string) string {
	browser, system := findPattern(userAgentBrowsers, ua), findPattern(userAgentSystems, ua)
	switch {
	case browser != "" && system != "":
		return i18n.Tr(lang, "mail.security.browser_on", browser, system)
	case browser != "":
		return browser
	case system != "":
		return system
	}
	return i18n.Tr(lang, "mail.security.unknown_browser")
}

// SecurityAlert is an email that tells a user about a change to the security
// of their account.
type SecurityAlert struct {
	definition[securityAlertData]
}

func securityAlert(name, sampleName string) SecurityAlert {
	return SecurityAlert{definition[securityAlertData]{
		name:    "security/" + name,
		subject: translatedSubject[securityAlertData]("mail.security." + name),
		sample: func(lang string) securityAlertData {
			activity := Activity{
				UserAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 15.7; rv:145.0) Gecko/20100101 Firefox/145.0",
				IP:        "203.0.113.42",
				Time:      time.Date(2026, 11, 12, 15, 4, 0, 0, time.UTC),
			}
			return newSecurityAlertData(lang, sampleUser{}, activity, sampleName)
		},
	}}
}

var (
	// AlertNewSignIn is sent when an account is signed in to from a browser it
	// was not signed in to before.
	AlertNewSignIn         = securityAlert("new_sign_in", "")
	AlertPasswordChanged   = securityAlert("password_changed", "")
	AlertEmailChanged      = securityAlert("email_changed", "alice@new.example.com")
	AlertTwoFactorDisabled = securityAlert("two_factor_disabled", "")
	AlertSSHKeyAdded       = securityAlert("ssh_key_added", "Work laptop")
	AlertAccessTokenAdded  = securityAlert("access_token_added", "CI")
	AlertPasskeyAdded      = securityAlert("passkey_added", "iPhone")
)

// SendSecurityAlertMail tells the user about a change to the security of their
// account. The name names what changed, e.g., the name of the new SSH key or
// the new email address.
func SendSecurityAlertMail(l Locale, u User, to string, alert SecurityAlert, a Activity, name string) error {
	data := newSecurityAlertData(l.Language(), u, a, name)
	return sendUserMail(l.Language(), u, to, alert.definition, data, "security alert "+alert.name)
}
