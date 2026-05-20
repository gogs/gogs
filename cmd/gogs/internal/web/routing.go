package web

import (
	"bytes"
	"io"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/public"
)

const (
	viteDevURL = "http://localhost:5173"
	// defaultLangAttr is the literal `lang="..."` value committed in
	// web/index.html. Per-request HTML responses replace it with the user's
	// resolved locale before serving.
	defaultLangAttr = `lang="en"`
)

// newRoutingHandler returns an http.Handler that serves the React SPA built
// from /web. In prod mode embedded assets from public.WebAssets are served
// directly and unknown paths fall back to index.html so client-side routing
// works. In dev mode requests are reverse-proxied to the Vite dev server at
// localhost:5173.
//
// It is mounted as macaron's NotFound handler so legacy routes keep working
// and only unmatched paths reach the SPA.
func newRoutingHandler() (http.Handler, error) {
	f := flamego.New()
	f.Use(flamego.Recovery())

	if conf.IsProdMode() {
		if err := mountEmbedded(f); err != nil {
			return nil, err
		}
	} else {
		if err := mountViteProxy(f); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func mountEmbedded(f *flamego.Flame) error {
	webFS, err := fs.Sub(public.WebAssets, "dist")
	if err != nil {
		return errors.Wrap(err, "load embedded web assets")
	}
	f.Use(flamego.Static(flamego.StaticOptions{FileSystem: http.FS(webFS)}))

	index, err := public.WebAssets.ReadFile("dist/index.html")
	if err != nil {
		return errors.Wrap(err, `read "dist/index.html"`)
	}

	f.Get("/{**}", func(w http.ResponseWriter, r *http.Request) {
		body := injectLang(index, context.LangFromRequest(r))
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(body)
	})
	return nil
}

func mountViteProxy(f *flamego.Flame) error {
	viteURL, err := url.Parse(viteDevURL)
	if err != nil {
		return errors.Wrap(err, "parse vite URL")
	}
	proxy := httputil.NewSingleHostReverseProxy(viteURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			return nil
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "read vite response body")
		}
		_ = resp.Body.Close()
		body := injectLang(raw, context.LangFromRequest(resp.Request))
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		return nil
	}
	f.Any("/{**}", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
	return nil
}

// injectLang replaces the placeholder lang attribute in index.html with the
// resolved locale. Returns the input unchanged when lang is empty.
func injectLang(html []byte, lang string) []byte {
	if lang == "" {
		return html
	}
	return bytes.Replace(html, []byte(defaultLangAttr), []byte(`lang="`+lang+`"`), 1)
}
