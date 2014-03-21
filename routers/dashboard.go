// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import (
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/routers/user"
)

func Home(ctx *middleware.Context) {
	if ctx.IsSigned {
		user.Dashboard(ctx)
		return
	}
	ctx.Data["PageIsHome"] = true
	ctx.HTML(200, "home")
}

func Help(ctx *middleware.Context) {
	ctx.Data["PageIsHelp"] = true
	ctx.HTML(200, "help")
}
