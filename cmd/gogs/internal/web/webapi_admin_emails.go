package web

import (
	"net/http"
	"slices"

	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/email"
)

type adminEmailLang struct {
	Lang string `json:"lang"`
	Name string `json:"name"`
}

// adminEmailPreview is an email the server sends, composed with made-up data.
type adminEmailPreview struct {
	Names []string         `json:"names"`
	Langs []adminEmailLang `json:"langs"`
	Name  string           `json:"name"`
	Lang  string           `json:"lang"`
	// Error tells why the email could not be composed, e.g., a broken template
	// in the custom directory. The other fields of the email are empty then.
	Error   string `json:"error,omitempty"`
	Subject string `json:"subject"`
	HTML    string `json:"html"`
	Text    string `json:"text"`
}

// getAdminEmailPreview composes the email named by the "name" query in the
// language of the "language" query, not "lang", which the i18n middleware
// takes to switch the language of the site. It shows the first email and the
// viewer's language by default.
func getAdminEmailPreview(c flamego.Context, l i18n.Locale) (statusCode int, resp any, err error) {
	names := email.Names()
	preview := &adminEmailPreview{
		Names: names,
		Name:  c.QueryTrim("name"),
		Lang:  c.QueryTrim("language"),
	}
	for i, lang := range conf.I18n.Langs {
		preview.Langs = append(preview.Langs, adminEmailLang{Lang: lang, Name: conf.I18n.Names[i]})
	}
	if preview.Name == "" {
		preview.Name = names[0]
	} else if !slices.Contains(names, preview.Name) {
		return http.StatusNotFound, nil, email.ErrNotFound
	}
	if !slices.Contains(conf.I18n.Langs, preview.Lang) {
		preview.Lang = l.Language()
	}

	m, err := email.Preview(preview.Name, preview.Lang)
	if err != nil {
		preview.Error = err.Error()
		return http.StatusOK, preview, nil
	}
	preview.Subject = conf.Email.SubjectPrefix + m.Subject
	preview.HTML = m.HTML
	preview.Text = m.Text
	return http.StatusOK, preview, nil
}
