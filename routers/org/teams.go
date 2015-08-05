// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"path"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	TEAMS             base.TplName = "org/team/teams"
	TEAM_NEW          base.TplName = "org/team/new"
	TEAM_MEMBERS      base.TplName = "org/team/members"
	TEAM_REPOSITORIES base.TplName = "org/team/repositories"
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
	uid := com.StrTo(ctx.Query("uid")).MustInt64()
	if uid == 0 {
		ctx.Redirect(ctx.Org.OrgLink + "/teams")
		return
	}

	page := ctx.Query("page")
	var err error
	switch ctx.Params(":action") {
	case "join":
		if !ctx.Org.IsOwner {
			ctx.Error(404)
			return
		}
		err = ctx.Org.Team.AddMember(ctx.User.Id)
	case "leave":
		err = ctx.Org.Team.RemoveMember(ctx.User.Id)
	case "remove":
		if !ctx.Org.IsOwner {
			ctx.Error(404)
			return
		}
		err = ctx.Org.Team.RemoveMember(uid)
		page = "team"
	case "add":
		if !ctx.Org.IsOwner {
			ctx.Error(404)
			return
		}
		uname := ctx.Query("uname")
		var u *models.User
		u, err = models.GetUserByName(uname)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.Flash.Error(ctx.Tr("form.user_not_exist"))
				ctx.Redirect(ctx.Org.OrgLink + "/teams/" + ctx.Org.Team.LowerName)
			} else {
				ctx.Handle(500, " GetUserByName", err)
			}
			return
		}

		err = ctx.Org.Team.AddMember(u.Id)
		page = "team"
	}

	if err != nil {
		if models.IsErrLastOrgOwner(err) {
			ctx.Flash.Error(ctx.Tr("form.last_org_owner"))
		} else {
			log.Error(3, "Action(%s): %v", ctx.Params(":action"), err)
			ctx.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
			return
		}
	}

	switch page {
	case "team":
		ctx.Redirect(ctx.Org.OrgLink + "/teams/" + ctx.Org.Team.LowerName)
	default:
		ctx.Redirect(ctx.Org.OrgLink + "/teams")
	}
}

func TeamsRepoAction(ctx *middleware.Context) {
	if !ctx.Org.IsOwner {
		ctx.Error(404)
		return
	}

	var err error
	switch ctx.Params(":action") {
	case "add":
		repoName := path.Base(ctx.Query("repo-name"))
		var repo *models.Repository
		repo, err = models.GetRepositoryByName(ctx.Org.Organization.Id, repoName)
		if err != nil {
			if models.IsErrRepoNotExist(err) {
				ctx.Flash.Error(ctx.Tr("org.teams.add_nonexistent_repo"))
				ctx.Redirect(ctx.Org.OrgLink + "/teams/" + ctx.Org.Team.LowerName + "/repositories")
				return
			}
			ctx.Handle(500, "GetRepositoryByName", err)
			return
		}
		err = ctx.Org.Team.AddRepository(repo)
	case "remove":
		err = ctx.Org.Team.RemoveRepository(com.StrTo(ctx.Query("repoid")).MustInt64())
	}

	if err != nil {
		log.Error(3, "Action(%s): '%s' %v", ctx.Params(":action"), ctx.Org.Team.Name, err)
		ctx.Handle(500, "TeamsRepoAction", err)
		return
	}
	ctx.Redirect(ctx.Org.OrgLink + "/teams/" + ctx.Org.Team.LowerName + "/repositories")
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
	var auth models.AccessMode
	switch form.Permission {
	case "read":
		auth = models.ACCESS_MODE_READ
	case "write":
		auth = models.ACCESS_MODE_WRITE
	case "admin":
		auth = models.ACCESS_MODE_ADMIN
	default:
		ctx.Error(401)
		return
	}

	org := ctx.Org.Organization

	t := &models.Team{
		OrgID:       org.Id,
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

func TeamMembers(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Org.Team.Name
	ctx.Data["PageIsOrgTeams"] = true
	if err := ctx.Org.Team.GetMembers(); err != nil {
		ctx.Handle(500, "GetMembers", err)
		return
	}
	ctx.HTML(200, TEAM_MEMBERS)
}

func TeamRepositories(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Org.Team.Name
	ctx.Data["PageIsOrgTeams"] = true
	if err := ctx.Org.Team.GetRepositories(); err != nil {
		ctx.Handle(500, "GetRepositories", err)
		return
	}
	ctx.HTML(200, TEAM_REPOSITORIES)
}

func EditTeam(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Org.Organization.FullName
	ctx.Data["PageIsOrgTeams"] = true
	ctx.Data["team_name"] = ctx.Org.Team.Name
	ctx.Data["desc"] = ctx.Org.Team.Description
	ctx.HTML(200, TEAM_NEW)
}

func EditTeamPost(ctx *middleware.Context, form auth.CreateTeamForm) {
	t := ctx.Org.Team
	ctx.Data["Title"] = ctx.Org.Organization.FullName
	ctx.Data["PageIsOrgTeams"] = true
	ctx.Data["team_name"] = t.Name
	ctx.Data["desc"] = t.Description

	if ctx.HasError() {
		ctx.HTML(200, TEAM_NEW)
		return
	}

	isAuthChanged := false
	if !t.IsOwnerTeam() {
		// Validate permission level.
		var auth models.AccessMode
		switch form.Permission {
		case "read":
			auth = models.ACCESS_MODE_READ
		case "write":
			auth = models.ACCESS_MODE_WRITE
		case "admin":
			auth = models.ACCESS_MODE_ADMIN
		default:
			ctx.Error(401)
			return
		}

		t.Name = form.TeamName
		if t.Authorize != auth {
			isAuthChanged = true
			t.Authorize = auth
		}
	}
	t.Description = form.Description
	if err := models.UpdateTeam(t, isAuthChanged); err != nil {
		if err == models.ErrTeamNameIllegal {
			ctx.Data["Err_TeamName"] = true
			ctx.RenderWithErr(ctx.Tr("form.illegal_team_name"), TEAM_NEW, &form)
		} else {
			ctx.Handle(500, "UpdateTeam", err)
		}
		return
	}
	ctx.Redirect(ctx.Org.OrgLink + "/teams/" + t.LowerName)
}

func DeleteTeam(ctx *middleware.Context) {
	if err := models.DeleteTeam(ctx.Org.Team); err != nil {
		ctx.Handle(500, "DeleteTeam", err)
		return
	}
	ctx.Flash.Success(ctx.Tr("org.teams.delete_team_success"))
	ctx.Redirect(ctx.Org.OrgLink + "/teams")
}
