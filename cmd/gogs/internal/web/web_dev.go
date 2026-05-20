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

	"gogs.io/gogs/internal/context"
)

func mountWebRoutes(f *flamego.Flame) error {
	viteURL, err := url.Parse("http://localhost:5173")
	if err != nil {
		return errors.Wrap(err, "parse Vite URL")
	}
	proxy := httputil.NewSingleHostReverseProxy(viteURL)
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
		body := renderIndex(raw, wc)
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		resp.Header.Set("Content-Length", strconv.Itoa(len(body)))
		if wc.Status != 0 {
			resp.StatusCode = wc.Status
			resp.Status = http.StatusText(wc.Status)
		}
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
