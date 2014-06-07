package org

import (
	"github.com/go-martini/martini"
	"github.com/gogits/gogs/modules/middleware"
)

func Organization(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization Name" + params["org"]
	ctx.HTML(200, "org/org")
}
