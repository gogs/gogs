// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	TEAMS    base.TplName = "org/team/teams"
	TEAM_NEW base.TplName = "org/team/new"
)

func Teams(ctx *middleware.Context) {
	org := ctx.Org.Organization
	ctx.Data["Title"] = org.FullName
	ctx.Data["PageIsOrgTeams"] = true

	if err := org.GetTeams(); err != nil {
		ctx.Handle(500, "GetTeams", err)
		return
	}
	for _, t := range org.Teams {
		if err := t.GetMembers(); err != nil {
			ctx.Handle(500, "GetMembers", err)
			return
		}
	}
	ctx.Data["Teams"] = org.Teams

	ctx.HTML(200, TEAMS)
}

func TeamsAction(ctx *middleware.Context) {
	var err error
	switch ctx.Params(":action") {
	case "join":
		err = ctx.Org.Team.AddMember(ctx.User.Id)
	case "leave":
		err = ctx.Org.Team.RemoveMember(ctx.User.Id)
	}

	if err != nil {
		log.Error(4, "Action(%s): %v", ctx.Params(":action"), err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}
	ctx.Redirect(ctx.Org.OrgLink + "/teams")
}

func NewTeam(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Org.Organization.FullName
	ctx.Data["PageIsOrgTeams"] = true
	ctx.Data["PageIsOrgTeamsNew"] = true
	ctx.Data["Team"] = &models.Team{}
	ctx.HTML(200, TEAM_NEW)
}

func NewTeamPost(ctx *middleware.Context, form auth.CreateTeamForm) {
	ctx.Data["Title"] = ctx.Org.Organization.FullName
	ctx.Data["PageIsOrgTeams"] = true
	ctx.Data["PageIsOrgTeamsNew"] = true
	ctx.Data["Team"] = &models.Team{}

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

	org := ctx.Org.Organization

	t := &models.Team{
		OrgId:       org.Id,
		Name:        form.TeamName,
		Description: form.Description,
		Authorize:   auth,
	}
	if err := models.NewTeam(t); err != nil {
		switch err {
		case models.ErrTeamNameIllegal:
			ctx.Data["Err_TeamName"] = true
			ctx.RenderWithErr(ctx.Tr("form.illegal_team_name"), TEAM_NEW, &form)
		case models.ErrTeamAlreadyExist:
			ctx.Data["Err_TeamName"] = true
			ctx.RenderWithErr(ctx.Tr("form.team_name_been_taken"), TEAM_NEW, &form)
		default:
			ctx.Handle(500, "NewTeam", err)
		}
		return
	}
	log.Trace("Team created: %s/%s", org.Name, t.Name)
	ctx.Redirect(ctx.Org.OrgLink + "/teams/" + t.LowerName)
}

func EditTeam(ctx *middleware.Context) {
	ctx.Data["Title"] = "Organization " + ctx.Params(":org") + " Edit Team"
	ctx.HTML(200, "org/edit_team")
}

func SingleTeam(ctx *middleware.Context) {
	ctx.Data["Title"] = "single-team" + ctx.Params(":org")
	ctx.HTML(200, "org/team")
}
