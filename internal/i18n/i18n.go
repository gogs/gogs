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
// the value that follows the name in args, e.g., l.Tr("repo.editor.add", "path",
// "README.md"). A message missing from the language is taken from the default
// language.
func (l Locale) Tr(key string, args ...any) string {
	if l.messages == nil {
		return key
	}
	// Keys are written as "section.key", which the library looks up as
	// "section::key". Keys above any section have no dot.
	if i := strings.IndexByte(key, '.'); i > 0 {
		key = key[:i] + "::" + key[i+1:]
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
	// DefaultLang is the language of messages missing from other languages.
	DefaultLang string
}

// Localizer loads the locale files and returns the middleware that maps the
// Locale of each request. The language comes from the "lang" query parameter,
// then the "lang" cookie, then the Accept-Language header. A language not read
// from the cookie is remembered in it. When the language is chosen by the query
// parameter, the viewer is redirected to the same path without the query.
func Localizer(opts Options) (flamego.Handler, error) {
	if len(opts.Langs) != len(opts.Names) {
		return nil, errors.New("number of languages and names do not match")
	}
	store := i18n.NewStore()
	locales := make(map[string]*i18n.Locale, len(opts.Langs))
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
		// The library picks plural rules by the tag, and does not know some
		// region subtags, e.g., "SP" of "sr-SP", so those use the base language.
		storeLang := lang
		if _, err := language.Parse(lang); err != nil {
			storeLang, _, _ = strings.Cut(lang, "-")
		}
		l, err := store.AddLocale(storeLang, opts.Names[i], data, custom...)
		if err != nil {
			return nil, errors.Wrapf(err, "load locale file %q", name)
		}
		locales[lang] = l
	}
	fallback, ok := locales[opts.DefaultLang]
	if !ok {
		return nil, errors.Newf("default language %q is not one of the languages", opts.DefaultLang)
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
			tag, _, _ := matcher.Match(tags...)
			lang = tag.String()
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
