package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flamego/flamego"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/context"
)

func TestRenderIndex_injection(t *testing.T) {
	customDir := t.TempDir()
	injectDir := filepath.Join(customDir, "templates", "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(injectDir, "head.tmpl"), []byte(`<meta name="head-marker">`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(injectDir, "footer.tmpl"), []byte(`<script>footerMarker()</script>`), 0600))

	inject, err := readInjectContent(customDir)
	require.NoError(t, err)

	shell := `<html><head>{{.WebContext}}</head><body><div id="root"></div></body></html>`
	got, err := renderIndex([]byte(shell), context.WebContext{Lang: "en-US"}, inject)
	require.NoError(t, err)

	out := string(got)
	assert.Contains(t, out, `<meta name="head-marker">`)
	assert.Contains(t, out, `<script>footerMarker()</script></body>`)
	// The head injection lands inside <head>, before </head>.
	assert.Less(t, strings.Index(out, `<meta name="head-marker">`), strings.Index(out, `</head>`))
}

func TestRenderIndex_entrypointPaths(t *testing.T) {
	shell := `<head>{{.WebContext}}<script type="module" src="./assets/index.js"></script>` +
		`<link rel="stylesheet" href="./assets/index.css"></head>`
	tests := []struct {
		name   string
		subURL string
		want   string
	}{
		{
			name: "root",
			want: `<script type="module" src="/assets/index.js"></script><link rel="stylesheet" href="/assets/index.css">`,
		},
		{
			name:   "subpath",
			subURL: "/gogs",
			want:   `<script type="module" src="/gogs/assets/index.js"></script><link rel="stylesheet" href="/gogs/assets/index.css">`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := renderIndex([]byte(shell), context.WebContext{Lang: "en-US", SubURL: test.subURL}, injectContent{})
			require.NoError(t, err)
			assert.Contains(t, string(got), test.want)
		})
	}
}

func TestWebLocales(t *testing.T) {
	localeDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(localeDir, "locale_en-US.ini"), []byte("custom_only = Custom\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(localeDir, "locale_de-DE.ini"), []byte("[repo]\nfork = Gabel\n"), 0600))
	var err error
	webLocales, err = loadWebLocales(localeDir, []string{"en-US", "de-DE"})
	require.NoError(t, err)

	f := flamego.New()
	mountWebLocaleRoutes(f)
	get := func(t *testing.T, lang string) map[string]string {
		t.Helper()
		resp := httptest.NewRecorder()
		f.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, webLocaleURL(lang), nil))
		require.Equal(t, http.StatusOK, resp.Code)
		assert.Contains(t, resp.Header().Get("Cache-Control"), "immutable")
		var messages map[string]string
		require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &messages))
		return messages
	}

	de := get(t, "de-DE")
	assert.Equal(t, "Erkunden", de["explore"], "the language's own file")
	assert.Equal(t, "Gabel", de["repo.fork"], "the custom override of the language")
	assert.Equal(t, "Custom", de["custom_only"], "keys missing from the language fall back to en-US")
	assert.Equal(t, "Explore", get(t, "en-US")["explore"])
	assert.NotEqual(t, webLocaleURL("en-US"), webLocaleURL("de-DE"))
	assert.Equal(t, webLocaleURL("en-US"), webLocaleURL("xx-XX"), "unknown languages use en-US")

	out, err := renderIndex([]byte(`<head>{{.WebContext}}</head>`), context.WebContext{Lang: "de-DE", SubURL: "/gogs"}, injectContent{})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"localeURL":"/gogs`+webLocaleURL("de-DE")+`"`)
	assert.Contains(t, string(out), `<link rel="preload" as="fetch" crossorigin href="/gogs`+webLocaleURL("de-DE")+`">`)
}

func TestRenderIndex_serverError(t *testing.T) {
	shell := `<html><head>{{.WebContext}}</head><body><div id="root"></div></body></html>`
	got, err := renderIndex([]byte(shell), context.WebContext{
		Lang:  "en-US",
		Page:  context.WebPageServerError,
		Error: `open </script><script>alert(1)</script>: no such file`,
	}, injectContent{})
	require.NoError(t, err)

	out := string(got)
	assert.Contains(t, out, `"page":"serverError"`)
	// The reason is arbitrary text, so it must stay inside the context script.
	assert.Equal(t, 1, strings.Count(out, "</script>"))
	assert.Contains(t, out, `open \u003c/script\u003e\u003cscript\u003ealert(1)\u003c/script\u003e: no such file`)
}
