// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	TEAMS base.TplName = "org/teams"
)

func Teams(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Teams"

	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.Teams(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.Teams(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	if err = org.GetTeams(); err != nil {
		ctx.Handle(500, "org.Teams(GetTeams)", err)
		return
	}
	for _, t := range org.Teams {
		if err = t.GetMembers(); err != nil {
			ctx.Handle(500, "org.Home(GetMembers)", err)
			return
		}
	}
	ctx.Data["Teams"] = org.Teams

	ctx.HTML(200, TEAMS)
}

func NewTeam(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " New Team"
	ctx.HTML(200, "org/new_team")
}

func EditTeam(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Edit Team"
	ctx.HTML(200, "org/edit_team")
}
