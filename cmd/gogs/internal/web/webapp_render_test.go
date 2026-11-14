package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/unknwon/i18n"
	"unknwon.dev/ini/v2"

	embedconf "gogs.io/gogs/conf"

	"gogs.io/gogs/internal/context"
)

func TestRenderIndex_injection(t *testing.T) {
	customDir := t.TempDir()
	injectDir := filepath.Join(customDir, "templates", "inject")
	require.NoError(t, os.MkdirAll(injectDir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(injectDir, "head.tmpl"), []byte(`<meta name="head-marker">`), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(injectDir, "footer.tmpl"), []byte(`<script>footerMarker()</script>`), 0600))
	t.Setenv("GOGS_CUSTOM", customDir)

	inject, err := readInjectContent()
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

func TestRenderIndex_messages(t *testing.T) {
	en := "explore = Explore\n[repo]\nfork = Fork\n"
	require.NoError(t, i18n.SetMessage("en-US", []byte(en)))
	require.NoError(t, i18n.SetMessage("de-DE", []byte("explore = Erkunden\n")))
	i18n.SetDefaultLang("en-US")

	got := webLocaleMessages("de-DE")
	assert.Equal(t, "Erkunden", got["explore"])
	assert.Equal(t, "Fork", got["repo.fork"], "missing keys fall back to en-US")

	out, err := renderIndex([]byte(`<head>{{.WebContext}}</head>`), context.WebContext{Lang: "de-DE"}, injectContent{})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"explore":"Erkunden"`)
}

// Every key the web app uses must exist in en-US, which other locales fall back
// to.
func TestWebLocaleKeys_inEnglish(t *testing.T) {
	data, err := embedconf.Files.ReadFile("locale/locale_en-US.ini")
	require.NoError(t, err)
	f, err := ini.Load(data)
	require.NoError(t, err)

	for _, key := range webLocaleKeys {
		section, name := ini.DefaultSection, key
		if i := strings.IndexByte(key, '.'); i > 0 {
			section, name = key[:i], key[i+1:]
		}
		assert.True(t, f.Section(section).HasKey(name), key)
	}
}
