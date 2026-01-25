package app

import (
	"net/http"

	"github.com/flamego/flamego"
	"github.com/microcosm-cc/bluemonday"
)

func ipynbSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "data-prompt-number").OnElements("div")
	p.AllowAttrs("class").OnElements("img")
	p.AllowURLSchemes("data")
	return p
}

func SanitizeIpynb() flamego.Handler {
	p := ipynbSanitizer()

	return func(c flamego.Context) {
		body, err := c.Request().Body().Bytes()
		if err != nil {
			c.ResponseWriter().WriteHeader(http.StatusInternalServerError)
			c.ResponseWriter().Write([]byte("read body"))
			return
		}

		c.ResponseWriter().WriteHeader(http.StatusOK)
		c.ResponseWriter().Write([]byte(p.Sanitize(string(body))))
	}
}
