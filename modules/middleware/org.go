// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"github.com/Unknwon/macaron"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

func OrgAssignment(redirect bool, args ...bool) macaron.Handler {
	return func(ctx *Context) {
		var (
			requireMember    bool
			requireOwner     bool
			requireAdminTeam bool
		)
		if len(args) >= 1 {
			requireMember = args[0]
		}
		if len(args) >= 2 {
			requireOwner = args[1]
		}
		if len(args) >= 3 {
			requireAdminTeam = args[2]
		}

		orgName := ctx.Params(":org")

		var err error
		ctx.Org.Organization, err = models.GetUserByName(orgName)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.Handle(404, "GetUserByName", err)
			} else if redirect {
				log.Error(4, "GetUserByName", err)
				ctx.Redirect(setting.AppSubUrl + "/")
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

		if ctx.IsSigned {
			ctx.Org.IsOwner = org.IsOwnedBy(ctx.User.Id)
			if ctx.Org.IsOwner {
				ctx.Org.IsMember = true
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

		ctx.Org.OrgLink = setting.AppSubUrl + "/org/" + org.Name
		ctx.Data["OrgLink"] = ctx.Org.OrgLink

		// Team.
		teamName := ctx.Params(":team")
		if len(teamName) > 0 {
			ctx.Org.Team, err = org.GetTeam(teamName)
			if err != nil {
				if err == models.ErrTeamNotExist {
					ctx.Handle(404, "GetTeam", err)
				} else if redirect {
					log.Error(4, "GetTeam", err)
					ctx.Redirect(setting.AppSubUrl + "/")
				} else {
					ctx.Handle(500, "GetTeam", err)
				}
				return
			}
			ctx.Data["Team"] = ctx.Org.Team
			ctx.Org.IsAdminTeam = ctx.Org.Team.IsOwnerTeam() || ctx.Org.Team.Authorize >= models.ACCESS_MODE_ADMIN
		}
		ctx.Data["IsAdminTeam"] = ctx.Org.IsAdminTeam
		if requireAdminTeam && !ctx.Org.IsAdminTeam {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}
	}
}
