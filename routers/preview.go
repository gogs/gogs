// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import "github.com/gogits/gogs/modules/middleware"

func Preview(ctx *middleware.Context) {
	content := ctx.Query("content")
	// todo : gfm render content
	// content = Markdown(content)
	ctx.Render.JSON(200, map[string]interface{}{
		"ok":      true,
		"content": "preview : " + content,
	})
}
