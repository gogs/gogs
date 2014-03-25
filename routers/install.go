// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package routers

import "github.com/gogits/gogs/modules/middleware"

func Install(ctx *middleware.Context){
	ctx.Data["PageIsInstall"] = true
	ctx.Data["Title"] = "Install"
	ctx.HTML(200,"install")
}
