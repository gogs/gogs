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
	"golang.org/x/text/language"
	"unknwon.dev/i18n"

	"gogs.io/gogs/internal/osx"
)

// Locale is the language of a request.
type Locale struct {
	// Lang is the language tag, e.g., "en-US".
	Lang string

	// messages and fallback are nil when no locale files are loaded, e.g., in
	// tests, then Tr returns the key itself.
	messages *i18n.Locale
	fallback *i18n.Locale
}

// Language returns the language tag, e.g., "en-US".
func (l Locale) Language() string {
	return l.Lang
}

// Tr returns the message of the key, with each "{name}" placeholder replaced by
// the value that follows the name in args, e.g., l.Tr("repo::editor.add", "path",
// "README.md"). A message missing from the language is taken from the default
// language.
func (l Locale) Tr(key string, args ...any) string {
	if l.messages == nil {
		return key
	}
	return l.messages.TranslateNamedWithFallback(l.fallback, key, args...)
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
	// DefaultLang is the language of messages missing from other languages. It
	// need not be one of Langs.
	DefaultLang string
}

// Localizer loads the locale files and returns the middleware that maps the
// Locale of each request. The language comes from the "lang" query parameter,
// then the "lang" cookie, then the Accept-Language header. A language not read
// from the cookie is remembered in it. When the language is chosen by the query
// parameter, the viewer is redirected to the same path without the query.
func Localizer(opts Options) (flamego.Handler, error) {
	if len(opts.Langs) == 0 {
		return nil, errors.New("no languages")
	}
	if len(opts.Langs) != len(opts.Names) {
		return nil, errors.New("number of languages and names do not match")
	}
	store := i18n.NewStore()
	load := func(lang, desc string) (*i18n.Locale, error) {
		name := fmt.Sprintf("locale_%s.ini", lang)
		data, ok := opts.Files[name]
		if !ok {
			return nil, errors.Newf("locale file %q does not exist", name)
		}
		var custom []any
		if customPath := filepath.Join(opts.CustomDirectory, name); osx.IsFile(customPath) {
			custom = append(custom, customPath)
		}
		l, err := store.AddLocale(lang, desc, data, custom...)
		if err != nil {
			return nil, errors.Wrapf(err, "load locale file %q", name)
		}
		return l, nil
	}

	locales := make(map[string]*i18n.Locale, len(opts.Langs))
	tags := make([]language.Tag, len(opts.Langs))
	for i, lang := range opts.Langs {
		tags[i] = language.Raw.Make(lang)
		l, err := load(lang, opts.Names[i])
		if err != nil {
			return nil, err
		}
		locales[lang] = l
	}
	// The default language fills messages missing from the others, even when
	// admins leave it out of the languages to choose from.
	fallback, ok := locales[opts.DefaultLang]
	if !ok {
		var err error
		fallback, err = load(opts.DefaultLang, opts.DefaultLang)
		if err != nil {
			return nil, errors.Wrap(err, "load default language")
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

		messages, ok := locales[lang]
		if !ok {
			tags, _, _ := language.ParseAcceptLanguage(r.Header.Get("Accept-Language"))
			// The index, unlike the matched tag, names one of the languages, e.g.,
			// "en-GB" rather than "en-GB-u-rg-auzzzz" for "en-AU".
			_, i, _ := matcher.Match(tags...)
			lang = opts.Langs[i]
			messages = locales[lang]
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

		c.Map(Locale{Lang: lang, messages: messages, fallback: fallback})

		if fromQuery {
			c.Redirect(subURL+path.Clean(r.URL.EscapedPath()), http.StatusFound)
		}
	}, nil
}
