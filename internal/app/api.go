// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"

	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/macaron.v1"
)

func ipynbSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "data-prompt-number").OnElements("div")
	p.AllowAttrs("class").OnElements("img")
	p.AllowURLSchemes("data")
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
