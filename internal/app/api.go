package app

import (
	"net/http"

	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/macaron.v1"

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

func SanitizeIpynb() macaron.Handler {
	p := ipynbSanitizer()

	return func(c *macaron.Context) {
		html, err := c.Req.Body().String()
		if err != nil {
			c.Error(http.StatusInternalServerError, "read body")
			return
		}

		c.PlainText(http.StatusOK, []byte(p.Sanitize(html)))
	}
}
