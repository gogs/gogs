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

func newRoutingHandler() (http.Handler, error) {
	f := flamego.New()
	f.Use(flamego.Recovery())

	if err := mountWebRoutes(f); err != nil {
		return nil, errors.Wrap(err, "mount web routes")
	}
	return f, nil
}

func mountWebRoutes(f *flamego.Flame) error {
	if conf.IsProdMode() {
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
			body := bytes.Replace(index, []byte("{{.Lang}}"), []byte(context.LangFromRequest(r)), 1)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(body)
		})
		return nil
	}

	viteURL, err := url.Parse("http://localhost:5173")
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
		body := bytes.Replace(raw, []byte("{{.Lang}}"), []byte(context.LangFromRequest(resp.Request)), 1)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		// The upstream validators describe the unmodified body. Drop them so
		// the browser does not satisfy a conditional request from a cached
		// copy that has a stale injected lang attribute.
		resp.Header.Del("ETag")
		resp.Header.Del("Last-Modified")
		return nil
	}
	f.Any("/{**}", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
	return nil
}
