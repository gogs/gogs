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
