// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/gogits/gogs/modules/middleware"
)

func Dashboard(ctx *middleware.Context) {
	ctx.Data["Title"] = "Admin Dashboard"
	ctx.HTML(200, "admin/dashboard")
}

func Users(ctx *middleware.Context) {
	ctx.Data["Title"] = "User Management"
	ctx.HTML(200, "admin/users")
}

func Repositories(ctx *middleware.Context) {
	ctx.Data["Title"] = "Repository Management"
	ctx.HTML(200, "admin/repos")
}
