package app

import (
	"io"
	"net/http"

	"github.com/microcosm-cc/bluemonday"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/markup"
)

func ipynbSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "data-prompt-number").OnElements("div")
	p.AllowAttrs("class").OnElements("img")
	// Only allow data URIs with safe image MIME types to prevent XSS via
	// "data:text/html" payloads.
	p.AllowURLSchemeWithCustomPolicy("data", markup.IsSafeDataURI)
	return p
}

// SanitizeIpynb returns the handler that sanitizes the HTML of a rendered
// Jupyter notebook in the request body.
func SanitizeIpynb() http.HandlerFunc {
	p := ipynbSanitizer()

	return func(w http.ResponseWriter, r *http.Request) {
		html, err := io.ReadAll(r.Body)
		if err != nil {
			log.Error("SanitizeIpynb: read body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("read body"))
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
		_, _ = w.Write(p.SanitizeBytes(html))
	}
}
