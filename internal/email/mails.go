package email

import (
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
		if p.templateName() == name {
			return p.preview(lang)
		}
	}
	return nil, ErrNotFound
}
