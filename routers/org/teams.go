package org

import (
	"github.com/go-martini/martini"
	"github.com/gogits/gogs/modules/middleware"
)

func Teams(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization "+params["org"]+" Teams"
	ctx.HTML(200, "org/teams")
}

func NewTeam(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization "+params["org"]+" New Team"
	ctx.HTML(200, "org/new_team")
}
