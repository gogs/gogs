// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package misc

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

// https://github.com/gogits/go-gogs-client/wiki/Miscellaneous#render-an-arbitrary-markdown-document
func Markdown(ctx *middleware.Context, form api.MarkdownOption) {
	if ctx.HasApiError() {
		ctx.APIError(422, "", ctx.GetErrMsg())
		return
	}

	if len(form.Text) == 0 {
		ctx.Write([]byte(""))
		return
	}

	switch form.Mode {
	case "gfm":
		ctx.Write(base.RenderMarkdown([]byte(form.Text), form.Context))
	default:
		ctx.Write(base.RenderRawMarkdown([]byte(form.Text), ""))
	}
}

// https://github.com/gogits/go-gogs-client/wiki/Miscellaneous#render-a-markdown-document-in-raw-mode
func MarkdownRaw(ctx *middleware.Context) {
	body, err := ctx.Req.Body().Bytes()
	if err != nil {
		ctx.APIError(422, "", err)
		return
	}
	ctx.Write(base.RenderRawMarkdown(body, ""))
}
