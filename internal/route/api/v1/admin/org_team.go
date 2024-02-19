// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/convert"
	"gogs.io/gogs/internal/route/api/v1/user"
)

func CreateTeam(c *context.APIContext, form api.CreateTeamOption) {
	team := &database.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        form.Name,
		Description: form.Description,
		Authorize:   database.ParseAccessMode(form.Permission),
	}
	if err := database.NewTeam(team); err != nil {
		if database.IsErrTeamAlreadyExist(err) {
			c.ErrorStatus(http.StatusUnprocessableEntity, err)
		} else {
			c.Error(err, "new team")
		}
		return
	}

	c.JSON(http.StatusCreated, convert.ToTeam(team))
}

func AddTeamMember(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.AddMember(u.ID); err != nil {
		c.Error(err, "add member")
		return
	}

	c.NoContent()
}

func RemoveTeamMember(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}

	if err := c.Org.Team.RemoveMember(u.ID); err != nil {
		c.Error(err, "remove member")
		return
	}

	c.NoContent()
}

func ListTeamMembers(c *context.APIContext) {
	team := c.Org.Team
	if err := team.GetMembers(); err != nil {
		c.Error(err, "get team members")
		return
	}

	apiMembers := make([]*api.User, len(team.Members))
	for i := range team.Members {
		apiMembers[i] = team.Members[i].APIFormat()
	}
	c.JSONSuccess(apiMembers)
}
