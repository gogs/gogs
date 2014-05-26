// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"io/ioutil"
	"strings"

	"github.com/gogits/gogs/modules/auth/apiv1"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const DOC_URL = "http://gogs.io/docs"

// Render an arbitrary Markdown document.
func Markdown(ctx *middleware.Context, form apiv1.MarkdownForm) {
	if ctx.HasApiError() {
		ctx.JSON(422, base.ApiJsonErr{ctx.GetErrMsg(), DOC_URL})
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
	body, err := ioutil.ReadAll(ctx.Req.Body)
	if err != nil {
		ctx.JSON(422, base.ApiJsonErr{err.Error(), DOC_URL})
		return
	}
	ctx.Write(base.RenderRawMarkdown(body, ""))
}
