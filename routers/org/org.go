package org

import (
	"github.com/go-martini/martini"
	"github.com/gogits/gogs/modules/middleware"
)

func Organization(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization "+params["org"]
	ctx.HTML(200, "org/org")
}

func Members(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization "+params["org"]+" Members"
	ctx.HTML(200, "org/members")
}

func Teams(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization "+params["org"]+" Teams"
	ctx.HTML(200, "org/teams")
}

func New(ctx *middleware.Context) {
	ctx.Data["Title"] = "Create an Organization"
	ctx.HTML(200, "org/new")
}

func Dashboard(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Dashboard"
	ctx.HTML(200, "org/dashboard")
}

func Setting(ctx *middleware.Context, param martini.Params) {
	ctx.Data["Title"] = "Setting"
	ctx.HTML(200, "org/setting")
}
