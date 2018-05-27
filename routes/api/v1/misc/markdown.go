// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package misc

import (
	api "github.com/gogs/go-gogs-client"

	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/markup"
)

// https://github.com/gogs/go-gogs-client/wiki/Miscellaneous#render-an-arbitrary-markdown-document
func Markdown(c *context.APIContext, form api.MarkdownOption) {
	if c.HasApiError() {
		c.Error(422, "", c.GetErrMsg())
		return
	}

	if len(form.Text) == 0 {
		c.Write([]byte(""))
		return
	}

	switch form.Mode {
	case "gfm":
		c.Write(markup.Markdown([]byte(form.Text), form.Context, nil))
	default:
		c.Write(markup.RawMarkdown([]byte(form.Text), ""))
	}
}

// https://github.com/gogs/go-gogs-client/wiki/Miscellaneous#render-a-markdown-document-in-raw-mode
func MarkdownRaw(c *context.APIContext) {
	body, err := c.Req.Body().Bytes()
	if err != nil {
		c.Error(422, "", err)
		return
	}
	c.Write(markup.RawMarkdown(body, ""))
}
