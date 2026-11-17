package web

import (
	"io"
	"net/http"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	"github.com/microcosm-cc/bluemonday"
	"unknwon.dev/x/logx"

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

var ipynbPolicy = ipynbSanitizer()

// postSanitizeIpynb sanitizes the HTML of a Jupyter notebook in the request
// body, which the web app renders in the browser before showing it. The HTML
// is sent and returned as is, because the images of a notebook can make it
// large.
func postSanitizeIpynb(c flamego.Context, logger *logx.Logger) {
	w := c.ResponseWriter()
	html, err := io.ReadAll(c.Request().Request.Body)
	if err != nil {
		logger.Error("Failed to read request body", "error", err)
		writeErrorResponse(w, http.StatusInternalServerError, errors.Wrap(err, "read body"))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/plain; charset=UTF-8")
	_, _ = w.Write(ipynbPolicy.SanitizeBytes(html))
}
