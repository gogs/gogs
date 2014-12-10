// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"strings"

	"github.com/gogits/gogs/modules/auth/apiv1"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

// Render an arbitrary Markdown document.
func Markdown(ctx *middleware.Context, form apiv1.MarkdownForm) {
	if ctx.HasApiError() {
		ctx.JSON(422, base.ApiJsonErr{ctx.GetErrMsg(), base.DOC_URL})
		return
	}

	if len(form.Text) == 0 {
		ctx.Write([]byte(""))
		return
	}

	switch form.Mode {
	case "gfm":
		ctx.Write(base.RenderMarkdown([]byte(form.Text),
			setting.AppUrl+strings.TrimPrefix(form.Context, "/")))
	default:
		ctx.Write(base.RenderRawMarkdown([]byte(form.Text), ""))
	}
}

// Render a Markdown document in raw mode.
func MarkdownRaw(ctx *middleware.Context) {
	body, err := ctx.Req.Body().Bytes()
	if err != nil {
		ctx.JSON(422, base.ApiJsonErr{err.Error(), base.DOC_URL})
		return
	}
	ctx.Write(base.RenderRawMarkdown(body, ""))
}
