// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/routes/api/v1/convert"
	"github.com/gogits/gogs/routes/api/v1/user"
)

func CreateTeam(c *context.APIContext, form api.CreateTeamOption) {
	team := &models.Team{
		OrgID:       c.Org.Organization.ID,
		Name:        form.Name,
		Description: form.Description,
		Authorize:   models.ParseAccessMode(form.Permission),
	}
	if err := models.NewTeam(team); err != nil {
		if models.IsErrTeamAlreadyExist(err) {
			c.Error(422, "", err)
		} else {
			c.Error(500, "NewTeam", err)
		}
		return
	}

	c.JSON(201, convert.ToTeam(team))
}

func AddTeamMember(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}
	if err := c.Org.Team.AddMember(u.ID); err != nil {
		c.Error(500, "AddMember", err)
		return
	}

	c.Status(204)
}

func RemoveTeamMember(c *context.APIContext) {
	u := user.GetUserByParams(c)
	if c.Written() {
		return
	}

	if err := c.Org.Team.RemoveMember(u.ID); err != nil {
		c.Error(500, "RemoveMember", err)
		return
	}

	c.Status(204)
}
