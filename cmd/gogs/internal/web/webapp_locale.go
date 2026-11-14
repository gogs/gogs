package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"unknwon.dev/ini/v2"

	embedconf "gogs.io/gogs/conf"
)

// webLocale is the translations of one language for the web app.
type webLocale struct {
	// url is where the web app loads the translations from. It contains a hash of
	// them, so they are cached until they change.
	url  string
	body []byte
}

// webLocales maps each language to its translations for the web app. It is
// built once by loadWebLocales.
var webLocales map[string]webLocale

// loadWebLocales flattens the locale files of every language into JSON objects
// keyed by "section.key", or the bare key for those above any section. Like the
// server-rendered pages, each language reads en-US first, then its own file,
// then its override under the custom directory, so keys missing from a language
// fall back to en-US. customDir is the directory of the overrides.
func loadWebLocales(customDir string, langs []string) (map[string]webLocale, error) {
	readSources := func(lang string) ([]any, error) {
		name := "locale_" + lang + ".ini"
		data, err := embedconf.Files.ReadFile("locale/" + name)
		if err != nil {
			return nil, errors.Wrapf(err, "read %q", name)
		}
		sources := []any{data}
		if custom := filepath.Join(customDir, name); isFile(custom) {
			sources = append(sources, custom)
		}
		return sources, nil
	}

	english, err := readSources("en-US")
	if err != nil {
		return nil, err
	}
	locales := make(map[string]webLocale, len(langs))
	for _, lang := range langs {
		sources := english
		if lang != "en-US" {
			own, err := readSources(lang)
			if err != nil {
				return nil, err
			}
			sources = append(append([]any{}, english...), own...)
		}
		f, err := ini.LoadSources(sources, ini.IgnoreInlineComment(true), ini.UnescapeValueCommentSymbols(true))
		if err != nil {
			return nil, errors.Wrapf(err, "parse locale %q", lang)
		}

		messages := make(map[string]string)
		for _, section := range f.Sections() {
			prefix := ""
			if section.Name() != ini.DefaultSection {
				prefix = section.Name() + "."
			}
			for _, key := range section.Keys() {
				messages[prefix+key.Name()] = key.Value()
			}
		}
		body, err := json.Marshal(messages)
		if err != nil {
			return nil, errors.Wrapf(err, "encode locale %q", lang)
		}
		sum := sha256.Sum256(body)
		locales[lang] = webLocale{
			url:  "/assets/locale/" + lang + "." + hex.EncodeToString(sum[:6]) + ".json",
			body: body,
		}
	}
	return locales, nil
}

// webLocaleURL returns the path, without the subpath, of the translations for
// lang, falling back to en-US.
func webLocaleURL(lang string) string {
	if l, ok := webLocales[lang]; ok {
		return l.url
	}
	return webLocales["en-US"].url
}

// mountWebLocaleRoutes serves the translations at the URLs webLocaleURL returns.
func mountWebLocaleRoutes(f *flamego.Flame) {
	for _, l := range webLocales {
		f.Get(l.url, func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(l.body)
		})
	}
}

func isFile(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
