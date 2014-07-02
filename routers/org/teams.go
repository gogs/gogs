// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	TEAMS    base.TplName = "org/teams"
	TEAM_NEW base.TplName = "org/team_new"
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
	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.NewTeam(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.NewTeam(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	// Check ownership of organization.
	if !org.IsOrgOwner(ctx.User.Id) {
		ctx.Error(403)
		return
	}

	ctx.HTML(200, TEAM_NEW)
}

func NewTeamPost(ctx *middleware.Context, params martini.Params, form auth.CreateTeamForm) {
	org, err := models.GetUserByName(params["org"])
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Handle(404, "org.NewTeamPost(GetUserByName)", err)
		} else {
			ctx.Handle(500, "org.NewTeamPost(GetUserByName)", err)
		}
		return
	}
	ctx.Data["Org"] = org

	// Check ownership of organization.
	if !org.IsOrgOwner(ctx.User.Id) {
		ctx.Error(403)
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, TEAM_NEW)
		return
	}

	// Validate permission level.
	var auth models.AuthorizeType
	switch form.Permission {
	case "read":
		auth = models.ORG_READABLE
	case "write":
		auth = models.ORG_WRITABLE
	case "admin":
		auth = models.ORG_ADMIN
	default:
		ctx.Error(401)
		return
	}

	t := &models.Team{
		OrgId:       org.Id,
		Name:        form.TeamName,
		Description: form.Description,
		Authorize:   auth,
	}
	if err = models.NewTeam(t); err != nil {
		if err == models.ErrTeamAlreadyExist {
			ctx.Data["Err_TeamName"] = true
			ctx.RenderWithErr("Team name has already been used", TEAM_NEW, &form)
		} else {
			ctx.Handle(500, "org.NewTeamPost(NewTeam)", err)
		}
		return
	}
	log.Trace("%s Team created: %s/%s", ctx.Req.RequestURI, org.Name, t.Name)
	ctx.Redirect("/org/" + org.LowerName + "/teams/" + t.LowerName)
}

func EditTeam(ctx *middleware.Context, params martini.Params) {
	ctx.Data["Title"] = "Organization " + params["org"] + " Edit Team"
	ctx.HTML(200, "org/edit_team")
}
