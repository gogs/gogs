// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package misc

import (
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/markup"
)

func Markdown(c *context.APIContext, form api.MarkdownOption) {
	if len(form.Text) == 0 {
		_, _ = c.Write([]byte(""))
		return
	}

	_, _ = c.Write(markup.Markdown([]byte(form.Text), form.Context, nil))
}

func MarkdownRaw(c *context.APIContext) {
	body, err := c.Req.Body().Bytes()
	if err != nil {
		c.Error(err, "read body")
		return
	}
	_, _ = c.Write(markup.SanitizeBytes(markup.RawMarkdown(body, "")))
}
