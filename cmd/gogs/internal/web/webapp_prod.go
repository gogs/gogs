//go:build prod

package web

import (
	"io/fs"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/public"
)

func mountWebAppRoutes(f *flamego.Flame) error {
	webFS, err := fs.Sub(public.WebAssets, "dist")
	if err != nil {
		return errors.Wrap(err, "load embedded web assets")
	}
	// Index is set to a sentinel that does not exist in the FS so flamego.Static
	// never serves the raw index.html for "/" requests. The catch-all below
	// always renders the shell through renderIndex instead, ensuring template
	// substitutions are applied.
	f.Use(flamego.Static(flamego.StaticOptions{
		FileSystem: http.FS(webFS),
		Index:      "__disabled__",
	}))

	index, err := public.WebAssets.ReadFile("dist/index.html")
	if err != nil {
		return errors.Wrap(err, `read "dist/index.html"`)
	}

	f.Get("/{**}", func(w http.ResponseWriter, r *http.Request) {
		wc := context.WebContextFrom(r)
		body, err := renderIndex(index, wc)
		if err != nil {
			log.Error("Failed to render index: %v", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		// The body is rewritten per request (lang injection, future
		// runtime config), so caching it would serve stale content to
		// any user whose request resolves to a different locale. Use
		// no-store rather than no-cache so the browser cannot keep a
		// copy at all, not even for revalidation. Static assets keep
		// their normal caching via flamego.Static.
		w.Header().Set("Cache-Control", "no-store")
		status := wc.StatusCode
		if status <= 0 {
			status = http.StatusOK
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	})
	return nil
}
