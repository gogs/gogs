// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package app

import (
	"net/http"

	"github.com/microcosm-cc/bluemonday"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
)

func SanitizeIpynb() macaron.Handler {
	p := bluemonday.UGCPolicy()
	p.AllowAttrs("class", "data-prompt-number").OnElements("div")
	p.AllowURLSchemes("data")

	return func(c *context.Context) {
		html, err := c.Req.Body().String()
		if err != nil {
			c.Error(err, "read body")
			return
		}

		c.PlainText(http.StatusOK, p.Sanitize(html))
	}
}
