package org

import (
	"net/http"

	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
)

const tmplOrgTeamNew = "org/team/new"

func NewTeam(c *context.Context) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["PageIsOrgTeamsNew"] = true
	c.Data["Team"] = &database.Team{}
	c.Success(tmplOrgTeamNew)
}

func NewTeamPost(c *context.Context, f form.CreateTeam) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["PageIsOrgTeamsNew"] = true

	t := &database.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        f.TeamName,
		Description: f.Description,
		Authorize:   database.ParseAccessMode(f.Permission),
	}
	c.Data["Team"] = t

	if c.HasError() {
		c.HTML(http.StatusBadRequest, tmplOrgTeamNew)
		return
	}

	if err := database.NewTeam(t); err != nil {
		c.Data["Err_TeamName"] = true
		switch {
		case database.IsErrTeamAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.team_name_been_taken"), http.StatusUnprocessableEntity, tmplOrgTeamNew, &f)
		case database.IsErrNameNotAllowed(err):
			c.RenderWithErr(c.Tr("org.form.team_name_not_allowed", err.(database.ErrNameNotAllowed).Value()), http.StatusBadRequest, tmplOrgTeamNew, &f)
		default:
			c.Error(err, "new team")
		}
		return
	}
	log.Trace("Team created: %s/%s", c.Org.Organization.Name, t.Name)
	c.Redirect(c.Org.OrgLink + "/teams/" + t.LowerName)
}

func EditTeam(c *context.Context) {
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["team_name"] = c.Org.Team.Name
	c.Data["desc"] = c.Org.Team.Description
	c.Success(tmplOrgTeamNew)
}

func EditTeamPost(c *context.Context, f form.CreateTeam) {
	t := c.Org.Team
	c.Data["Title"] = c.Org.Organization.FullName
	c.Data["PageIsOrgTeams"] = true
	c.Data["Team"] = t

	if c.HasError() {
		c.HTML(http.StatusBadRequest, tmplOrgTeamNew)
		return
	}

	isAuthChanged := false
	if !t.IsOwnerTeam() {
		// Validate permission level.
		var auth database.AccessMode
		switch f.Permission {
		case "read":
			auth = database.AccessModeRead
		case "write":
			auth = database.AccessModeWrite
		case "admin":
			auth = database.AccessModeAdmin
		default:
			c.Status(http.StatusUnauthorized)
			return
		}

		t.Name = f.TeamName
		if t.Authorize != auth {
			isAuthChanged = true
			t.Authorize = auth
		}
	}
	t.Description = f.Description
	if err := database.UpdateTeam(t, isAuthChanged); err != nil {
		c.Data["Err_TeamName"] = true
		switch {
		case database.IsErrTeamAlreadyExist(err):
			c.RenderWithErr(c.Tr("form.team_name_been_taken"), http.StatusUnprocessableEntity, tmplOrgTeamNew, &f)
		default:
			c.Error(err, "update team")
		}
		return
	}
	c.Redirect(c.Org.OrgLink + "/teams/" + t.LowerName)
}

func DeleteTeam(c *context.Context) {
	if err := database.DeleteTeam(c.Org.Team); err != nil {
		c.Flash.Error("DeleteTeam: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("org.teams.delete_team_success"))
	}

	c.JSONSuccess(map[string]any{
		"redirect": c.Org.OrgLink + "/teams",
	})
}
