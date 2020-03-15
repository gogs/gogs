// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"net/http"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/route/api/v1/convert"
	"gogs.io/gogs/internal/route/api/v1/user"
)

func CreateTeam(c *context.APIContext, form api.CreateTeamOption) {
	team := &db.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        form.Name,
		Description: form.Description,
		Authorize:   db.ParseAccessMode(form.Permission),
	}
	if err := db.NewTeam(team); err != nil {
		if db.IsErrTeamAlreadyExist(err) {
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
