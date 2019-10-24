// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"path"

	"github.com/unknwon/com"
	log "gopkg.in/clog.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
)

const (
	TEAMS             = "org/team/teams"
	TEAM_NEW          = "org/team/new"
	TEAM_MEMBERS      = "org/team/members"
	TEAM_REPOSITORIES = "org/team/repositories"
)

func Teams(c *context.Context) {
	org := c.Org.Organization
	c.Data["Title"] = org.FullName
	c.Data["PageIsOrgTeams"] = true

	for _, t := range org.Teams {
		if err := t.GetMembers(); err != nil {
			c.Handle(500, "GetMembers", err)
			return
		}
	}
	c.Data["Teams"] = org.Teams

	c.HTML(200, TEAMS)
}

func TeamsAction(c *context.Context) {
	uid := com.StrTo(c.Query("uid")).MustInt64()
	if uid == 0 {
		c.Redirect(c.Org.OrgLink + "/teams")
		return
	}

	page := c.Query("page")
	var err error
	switch c.Params(":action") {
	case "join":
		if !c.Org.IsOwner {
			c.Error(404)
			return
		}
		err = c.Org.Team.AddMember(c.User.ID)
	case "leave":
		err = c.Org.Team.RemoveMember(c.User.ID)
	case "remove":
		if !c.Org.IsOwner {
			c.Error(404)
			return
		}
		err = c.Org.Team.RemoveMember(uid)
		page = "team"
	case "add":
		if !c.Org.IsOwner {
			c.Error(404)
			return
		}
		uname := c.Query("uname")
		var u *db.User
		u, err = db.GetUserByName(uname)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.Flash.Error(c.Tr("form.user_not_exist"))
				c.Redirect(c.Org.OrgLink + "/teams/" + c.Org.Team.LowerName)
			} else {
				c.Handle(500, " GetUserByName", err)
			}
			return
		}

		err = c.Org.Team.AddMember(u.ID)
		page = "team"
	}

	if err != nil {
		if db.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
		} else {
			log.Error(3, "Action(%s): %v", c.Params(":action"), err)
			c.JSON(200, map[string]interface{}{
				"ok":  false,
				"err": err.Error(),
			})
			return
		}
	}

	switch page {
	case "team":
		c.Redirect(c.Org.OrgLink + "/teams/" + c.Org.Team.LowerName)
	default:
		c.Redirect(c.Org.OrgLink + "/teams")
	}
}

func TeamsRepoAction(c *context.Context) {
	if !c.Org.IsOwner {
		c.Error(404)
		return
	}

	var err error
	switch c.Params(":action") {
	case "add":
		repoName := path.Base(c.Query("repo_name"))
		var repo *db.Repository
		repo, err = db.GetRepositoryByName(c.Org.Organization.ID, repoName)
		if err != nil {
			if errors.IsRepoNotExist(err) {
				c.Flash.Error(c.Tr("org.teams.add_nonexistent_repo"))
				c.Redirect(c.Org.OrgLink + "/teams/" + c.Org.Team.LowerName + "/repositories")
				return
			}
			c.Handle(500, "GetRepositoryByName", err)
			return
		}
		err = c.Org.Team.AddRepository(repo)
	case "remove":
		err = c.Org.Team.RemoveRepository(com.StrTo(c.Query("repoid")).MustInt64())
	}

	if err != nil {
		log.Error(3, "Action(%s): '%s' %v", c.Params(":action"), c.Org.Team.Name, err)
		c.Handle(500, "TeamsRepoAction", err)
		return
	}
	c.Redirect(c.Org.OrgLink + "/teams/" + c.Org.Team.LowerName + "/repositories")
}

func NewTeam(c *context.Context) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["PageIsOrgTeamsNew"] = true
	c.Data["Team"] = &db.Team{}
	c.HTML(200, TEAM_NEW)
}

func NewTeamPost(c *context.Context, f form.CreateTeam) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["PageIsOrgTeamsNew"] = true

	t := &db.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        f.TeamName,
		Description: f.Description,
		Authorize:   db.ParseAccessMode(f.Permission),
	}
	c.Data["Team"] = t

	if c.HasError() {
		c.HTML(200, TEAM_NEW)
		return
	}

	if err := db.NewTeam(t); err != nil {
		c.Data["Err_TeamName"] = true
		switch {
		case db.IsErrTeamAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.team_name_been_taken"), TEAM_NEW, &f)
		case db.IsErrNameReserved(err):
			c.RenderWithErr(c.Tr("org.form.team_name_reserved", err.(db.ErrNameReserved).Name), TEAM_NEW, &f)
		default:
			c.Handle(500, "NewTeam", err)
		}
		return
	}
	log.Trace("Team created: %s/%s", c.Org.Organization.Name, t.Name)
	c.Redirect(c.Org.OrgLink + "/teams/" + t.LowerName)
}

func TeamMembers(c *context.Context) {
	c.Data["Title"] = c.Org.Team.Name
	c.Data["PageIsOrgTeams"] = true
	if err := c.Org.Team.GetMembers(); err != nil {
		c.Handle(500, "GetMembers", err)
		return
	}
	c.HTML(200, TEAM_MEMBERS)
}

func TeamRepositories(c *context.Context) {
	c.Data["Title"] = c.Org.Team.Name
	c.Data["PageIsOrgTeams"] = true
	if err := c.Org.Team.GetRepositories(); err != nil {
		c.Handle(500, "GetRepositories", err)
		return
	}
	c.HTML(200, TEAM_REPOSITORIES)
}

func EditTeam(c *context.Context) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["team_name"] = c.Org.Team.Name
	c.Data["desc"] = c.Org.Team.Description
	c.HTML(200, TEAM_NEW)
}

func EditTeamPost(c *context.Context, f form.CreateTeam) {
	t := c.Org.Team
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["Team"] = t

	if c.HasError() {
		c.HTML(200, TEAM_NEW)
		return
	}

	isAuthChanged := false
	if !t.IsOwnerTeam() {
		// Validate permission level.
		var auth db.AccessMode
		switch f.Permission {
		case "read":
			auth = db.ACCESS_MODE_READ
		case "write":
			auth = db.ACCESS_MODE_WRITE
		case "admin":
			auth = db.ACCESS_MODE_ADMIN
		default:
			c.Error(401)
			return
		}

		t.Name = f.TeamName
		if t.Authorize != auth {
			isAuthChanged = true
			t.Authorize = auth
		}
	}
	t.Description = f.Description
	if err := db.UpdateTeam(t, isAuthChanged); err != nil {
		c.Data["Err_TeamName"] = true
		switch {
		case db.IsErrTeamAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.team_name_been_taken"), TEAM_NEW, &f)
		default:
			c.Handle(500, "UpdateTeam", err)
		}
		return
	}
	c.Redirect(c.Org.OrgLink + "/teams/" + t.LowerName)
}

func DeleteTeam(c *context.Context) {
	if err := db.DeleteTeam(c.Org.Team); err != nil {
		c.Flash.Error("DeleteTeam: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("org.teams.delete_team_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Org.OrgLink + "/teams",
	})
}
