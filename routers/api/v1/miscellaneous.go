// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Markdown(ctx *middleware.Context) {
	content := ctx.Query("content")
	ctx.Render.JSON(200, map[string]interface{}{
		"ok":      true,
		"content": string(base.RenderMarkdown([]byte(content), ctx.Query("repoLink"))),
	})
}
