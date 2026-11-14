// Package i18n chooses the language of each request and translates messages to
// it.
package i18n

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/unknwon/i18n"
	"golang.org/x/text/language"

	"gogs.io/gogs/internal/osx"
)

// Locale is the language of a request.
type Locale struct {
	i18n.Locale
}

// Language returns the language tag, e.g., "en-US".
func (l Locale) Language() string {
	return l.Lang
}

// Options configures the languages Localizer chooses from.
type Options struct {
	// SubURL is the path prefix of the site, e.g., "/gogs".
	SubURL string
	// Files holds the embedded locale files by name, e.g., "locale_en-US.ini".
	Files map[string][]byte
	// CustomDirectory is the directory of locale files that override messages of
	// the embedded ones.
	CustomDirectory string
	// Langs are the language tags, e.g., "en-US".
	Langs []string
	// Names are the display names of Langs, in the same order.
	Names []string
	// DefaultLang is the language of messages missing from other languages.
	DefaultLang string
}

// Localizer loads the locale files and returns the middleware that maps the
// Locale of each request. The language comes from the "lang" query parameter,
// then the "lang" cookie, then the Accept-Language header. A language chosen by
// the query parameter is remembered in the cookie, and the viewer is redirected
// to the same path without the query.
func Localizer(opts Options) (flamego.Handler, error) {
	if len(opts.Langs) != len(opts.Names) {
		return nil, errors.New("number of languages and names do not match")
	}
	i18n.SetDefaultLang(opts.DefaultLang)

	tags := make([]language.Tag, len(opts.Langs))
	for i, lang := range opts.Langs {
		tags[i] = language.Raw.Make(lang)

		name := fmt.Sprintf("locale_%s.ini", lang)
		data, ok := opts.Files[name]
		if !ok {
			return nil, errors.Newf("locale file %q does not exist", name)
		}
		var custom []any
		if customPath := filepath.Join(opts.CustomDirectory, name); osx.IsFile(customPath) {
			custom = append(custom, customPath)
		}
		err := i18n.SetMessageWithDesc(lang, opts.Names[i], data, custom...)
		if err != nil && err != i18n.ErrLangAlreadyExist {
			return nil, errors.Wrapf(err, "load locale file %q", name)
		}
	}
	matcher := language.NewMatcher(tags)
	subURL := strings.TrimSuffix(opts.SubURL, "/")

	return func(c flamego.Context) {
		r := c.Request().Request
		lang := r.URL.Query().Get("lang")
		fromQuery := lang != ""
		fromCookie := false
		if !fromQuery {
			if cookie, err := r.Cookie("lang"); err == nil {
				lang, _ = url.QueryUnescape(cookie.Value)
				fromCookie = true
			}
		}

		if !i18n.IsExist(lang) {
			tags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
			tag, _, _ := matcher.Match(tags...)
			lang = tag.String()
			fromQuery, fromCookie = false, false
		}

		if !fromCookie {
			http.SetCookie(c.ResponseWriter(), &http.Cookie{
				Name:   "lang",
				Value:  url.QueryEscape(lang),
				Path:   "/" + strings.TrimPrefix(subURL, "/"),
				MaxAge: 1<<31 - 1,
			})
		}

		c.Map(Locale{Locale: i18n.Locale{Lang: lang}})

		if fromQuery {
			c.Redirect(subURL+path.Clean(r.URL.EscapedPath()), http.StatusFound)
		}
	}, nil
}
