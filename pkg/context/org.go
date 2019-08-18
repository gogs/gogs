// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package context

import (
	"strings"

	"gopkg.in/macaron.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/setting"
)

type Organization struct {
	IsOwner      bool
	IsMember     bool
	IsTeamMember bool // Is member of team.
	IsTeamAdmin  bool // In owner team or team that has admin permission level.
	Organization *models.User
	OrgLink      string

	Team *models.Team
}

func HandleOrgAssignment(c *Context, args ...bool) {
	var (
		requireMember     bool
		requireOwner      bool
		requireTeamMember bool
		requireTeamAdmin  bool
	)
	if len(args) >= 1 {
		requireMember = args[0]
	}
	if len(args) >= 2 {
		requireOwner = args[1]
	}
	if len(args) >= 3 {
		requireTeamMember = args[2]
	}
	if len(args) >= 4 {
		requireTeamAdmin = args[3]
	}

	orgName := c.Params(":org")

	var err error
	c.Org.Organization, err = models.GetUserByName(orgName)
	if err != nil {
		c.NotFoundOrServerError("GetUserByName", errors.IsUserNotExist, err)
		return
	}
	org := c.Org.Organization
	c.Data["Org"] = org

	// Force redirection when username is actually a user.
	if !org.IsOrganization() {
		c.Redirect("/" + org.Name)
		return
	}

	// Admin has super access.
	if c.IsLogged && c.User.IsAdmin {
		c.Org.IsOwner = true
		c.Org.IsMember = true
		c.Org.IsTeamMember = true
		c.Org.IsTeamAdmin = true
	} else if c.IsLogged {
		c.Org.IsOwner = org.IsOwnedBy(c.User.ID)
		if c.Org.IsOwner {
			c.Org.IsMember = true
			c.Org.IsTeamMember = true
			c.Org.IsTeamAdmin = true
		} else {
			if org.IsOrgMember(c.User.ID) {
				c.Org.IsMember = true
			}
		}
	} else {
		// Fake data.
		c.Data["SignedUser"] = &models.User{}
	}
	if (requireMember && !c.Org.IsMember) ||
		(requireOwner && !c.Org.IsOwner) {
		c.Handle(404, "OrgAssignment", err)
		return
	}
	c.Data["IsOrganizationOwner"] = c.Org.IsOwner
	c.Data["IsOrganizationMember"] = c.Org.IsMember

	c.Org.OrgLink = setting.AppSubURL + "/org/" + org.Name
	c.Data["OrgLink"] = c.Org.OrgLink

	// Team.
	if c.Org.IsMember {
		if c.Org.IsOwner {
			if err := org.GetTeams(); err != nil {
				c.Handle(500, "GetTeams", err)
				return
			}
		} else {
			org.Teams, err = org.GetUserTeams(c.User.ID)
			if err != nil {
				c.Handle(500, "GetUserTeams", err)
				return
			}
		}
	}

	teamName := c.Params(":team")
	if len(teamName) > 0 {
		teamExists := false
		for _, team := range org.Teams {
			if team.LowerName == strings.ToLower(teamName) {
				teamExists = true
				c.Org.Team = team
				c.Org.IsTeamMember = true
				c.Data["Team"] = c.Org.Team
				break
			}
		}

		if !teamExists {
			c.Handle(404, "OrgAssignment", err)
			return
		}

		c.Data["IsTeamMember"] = c.Org.IsTeamMember
		if requireTeamMember && !c.Org.IsTeamMember {
			c.Handle(404, "OrgAssignment", err)
			return
		}

		c.Org.IsTeamAdmin = c.Org.Team.IsOwnerTeam() || c.Org.Team.Authorize >= models.ACCESS_MODE_ADMIN
		c.Data["IsTeamAdmin"] = c.Org.IsTeamAdmin
		if requireTeamAdmin && !c.Org.IsTeamAdmin {
			c.Handle(404, "OrgAssignment", err)
			return
		}
	}
}

func OrgAssignment(args ...bool) macaron.Handler {
	return func(c *Context) {
		HandleOrgAssignment(c, args...)
	}
}
