// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"strings"

	"gopkg.in/macaron.v1"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/setting"
)

func HandleOrgAssignment(ctx *Context, args ...bool) {
	var (
		requireMember     bool
		requireOwner      bool
		requireTeamMember bool
		requireAdminTeam  bool
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
		requireAdminTeam = args[3]
	}

	orgName := ctx.Params(":org")

	var err error
	ctx.Org.Organization, err = models.GetUserByName(orgName)
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByName", err)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return
	}
	org := ctx.Org.Organization
	ctx.Data["Org"] = org

	// Force redirection when username is actually a user.
	if !org.IsOrganization() {
		ctx.Redirect("/" + org.Name)
		return
	}

	// Admin has super access.
	if ctx.IsSigned && ctx.User.IsAdmin {
		ctx.Org.IsOwner = true
		ctx.Org.IsMember = true
		ctx.Org.IsTeamMember = true
		ctx.Org.IsAdminTeam = true
	} else if ctx.IsSigned {
		ctx.Org.IsOwner = org.IsOwnedBy(ctx.User.Id)
		if ctx.Org.IsOwner {
			ctx.Org.IsMember = true
			ctx.Org.IsTeamMember = true
			ctx.Org.IsAdminTeam = true
		} else {
			if org.IsOrgMember(ctx.User.Id) {
				ctx.Org.IsMember = true
			}
		}
	} else {
		// Fake data.
		ctx.Data["SignedUser"] = &models.User{}
	}
	if (requireMember && !ctx.Org.IsMember) ||
		(requireOwner && !ctx.Org.IsOwner) {
		ctx.Handle(404, "OrgAssignment", err)
		return
	}
	ctx.Data["IsOrganizationOwner"] = ctx.Org.IsOwner
	ctx.Data["IsOrganizationMember"] = ctx.Org.IsMember

	ctx.Org.OrgLink = setting.AppSubUrl + "/org/" + org.Name
	ctx.Data["OrgLink"] = ctx.Org.OrgLink

	// Team.
	if ctx.Org.IsMember {
		if err := org.GetUserTeams(ctx.User.Id); err != nil {
			ctx.Handle(500, "GetUserTeams", err)
			return
		}
	}

	teamName := ctx.Params(":team")
	if len(teamName) > 0 {
		teamExists := false
		for _, team := range org.Teams {
			if strings.ToLower(team.Name) == strings.ToLower(teamName) {
				teamExists = true
				ctx.Org.Team = team
				ctx.Org.IsTeamMember = true
				ctx.Data["Team"] = ctx.Org.Team
				break
			}
		}

		if !teamExists {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}

		ctx.Data["IsTeamMember"] = ctx.Org.IsTeamMember
		if requireTeamMember && !ctx.Org.IsTeamMember {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}

		ctx.Org.IsAdminTeam = ctx.Org.Team.IsOwnerTeam() || ctx.Org.Team.Authorize >= models.ACCESS_MODE_ADMIN
		ctx.Data["IsAdminTeam"] = ctx.Org.IsAdminTeam
		if requireAdminTeam && !ctx.Org.IsAdminTeam {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}
	}

}

func OrgAssignment(args ...bool) macaron.Handler {
	return func(ctx *Context) {
		HandleOrgAssignment(ctx, args...)
	}
}
