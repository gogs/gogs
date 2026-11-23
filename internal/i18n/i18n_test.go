package i18n

import (
	"net/http/httptest"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocalizer(t *testing.T) {
	h, err := Localizer(Options{
		Files: map[string][]byte{
			"locale_en-US.ini": []byte("[repo]\nfork = Fork\n"),
			"locale_en-GB.ini": []byte("[repo]\nfork = Fork (GB)\n"),
			"locale_de-DE.ini": []byte("[repo]\nfork = Gabel\n"),
		},
		Langs:       []string{"en-US", "en-GB", "de-DE"},
		Names:       []string{"English", "English (United Kingdom)", "Deutsch"},
		DefaultLang: "en-US",
	})
	require.NoError(t, err)

	tests := []struct {
		name           string
		acceptLanguage string
		wantLang       string
		wantFork       string
	}{
		{name: "exact", acceptLanguage: "de-DE", wantLang: "de-DE", wantFork: "Gabel"},
		{name: "base language", acceptLanguage: "de", wantLang: "de-DE", wantFork: "Gabel"},
		{name: "other region", acceptLanguage: "en-AU", wantLang: "en-GB", wantFork: "Fork (GB)"},
		{name: "unknown language", acceptLanguage: "xx", wantLang: "en-US", wantFork: "Fork"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := flamego.New()
			f.Use(h)
			f.Get("/", func(l Locale) string { return l.Language() + " " + l.Tr("repo::fork") })

			resp := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Accept-Language", test.acceptLanguage)
			f.ServeHTTP(resp, req)

			assert.Equal(t, test.wantLang+" "+test.wantFork, resp.Body.String())
			assert.Contains(t, resp.Header().Get("Set-Cookie"), "lang="+test.wantLang+";")
		})
	}

	t.Run("no languages", func(t *testing.T) {
		_, err := Localizer(Options{
			Files:       map[string][]byte{"locale_en-US.ini": []byte("[repo]\nfork = Fork\n")},
			DefaultLang: "en-US",
		})
		require.EqualError(t, err, "no languages in [i18n] LANGS")
	})

	t.Run("plurals", func(t *testing.T) {
		h, err := Localizer(Options{
			Files: map[string][]byte{
				"locale_en-US.ini": []byte("[repo]\nfiles = {count} ${file, count}\n[plurals]\nfile.one = file\nfile.other = files\n"),
				"locale_ja-JP.ini": []byte("[repo]\nfork = フォーク\n"),
			},
			Langs:       []string{"en-US", "ja-JP"},
			Names:       []string{"English", "日本語"},
			DefaultLang: "en-US",
		})
		require.NoError(t, err)

		f := flamego.New()
		f.Use(h)
		f.Get("/", func(l Locale) string { return l.Tr("repo::files", "count", 1) + ", " + l.Tr("repo::files", "count", 2) })
		for _, lang := range []string{"en-US", "ja-JP"} {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("Accept-Language", lang)
			f.ServeHTTP(resp, req)
			assert.Equal(t, "1 file, 2 files", resp.Body.String(), "messages falling back to en-US use its plural rules in %s", lang)
		}
	})

	t.Run("default language not listed", func(t *testing.T) {
		h, err := Localizer(Options{
			Files: map[string][]byte{
				"locale_en-US.ini": []byte("[repo]\nfork = Fork\nstar = Star\n"),
				"locale_de-DE.ini": []byte("[repo]\nfork = Gabel\n"),
			},
			Langs:       []string{"de-DE"},
			Names:       []string{"Deutsch"},
			DefaultLang: "en-US",
		})
		require.NoError(t, err)

		f := flamego.New()
		f.Use(h)
		f.Get("/", func(l Locale) string { return l.Language() + " " + l.Tr("repo::fork") + " " + l.Tr("repo::star") })
		resp := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", "en-US")
		f.ServeHTTP(resp, req)
		assert.Equal(t, "de-DE Gabel Star", resp.Body.String())
	})
}
