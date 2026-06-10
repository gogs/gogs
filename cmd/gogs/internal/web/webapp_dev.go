//go:build !prod

package web

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
)

func mountWebAppRoutes(f *flamego.Flame) error {
	viteURL, err := url.Parse("http://localhost:5173")
	if err != nil {
		return errors.Wrap(err, "parse Vite URL")
	}
	proxy := newViteReverseProxy(viteURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html") {
			return nil
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			return errors.Wrap(err, "read Vite response body")
		}
		_ = resp.Body.Close()
		wc := context.WebContextFrom(resp.Request)
		body, err := renderIndex(raw, wc)
		if err != nil {
			log.Error("Failed to render index: %v", err)
			body = []byte("Internal Server Error\n")
			resp.StatusCode = http.StatusInternalServerError
			resp.Status = http.StatusText(http.StatusInternalServerError)
			resp.Header.Set("Content-Type", "text/plain; charset=utf-8")
		} else if wc.StatusCode > 0 {
			resp.StatusCode = wc.StatusCode
			resp.Status = http.StatusText(wc.StatusCode)
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		// The upstream validators describe the unmodified body. Drop them
		// so the browser does not satisfy a conditional request from a
		// cached copy that has a stale injected lang attribute.
		resp.Header.Del("ETag")
		resp.Header.Del("Last-Modified")
		return nil
	}
	f.Any("/{**}", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})
	return nil
}

func newViteReverseProxy(viteURL *url.URL) *httputil.ReverseProxy {
	proxy := httputil.NewSingleHostReverseProxy(viteURL)
	director := proxy.Director
	proxy.Director = func(req *http.Request) {
		director(req)
		req.Host = viteURL.Host
	}
	return proxy
}
