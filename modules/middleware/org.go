// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package middleware

import (
	"github.com/Unknwon/macaron"

	"github.com/gogits/gogs/models"
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
			if err == models.ErrUserNotExist {
				ctx.Handle(404, "GetUserByName", err)
			} else if redirect {
				ctx.Redirect("/")
			} else {
				ctx.Handle(500, "GetUserByName", err)
			}
			return
		}
		ctx.Data["Org"] = ctx.Org.Organization

		if ctx.IsSigned {
			ctx.Org.IsOwner = ctx.Org.Organization.IsOrgOwner(ctx.User.Id)
			if ctx.Org.IsOwner {
				ctx.Org.IsMember = true
				ctx.Org.IsAdminTeam = true
			} else {
				if ctx.Org.Organization.IsOrgMember(ctx.User.Id) {
					ctx.Org.IsMember = true
					// TODO: ctx.Org.IsAdminTeam
				}
			}
		}
		if (requireMember && !ctx.Org.IsMember) ||
			(requireOwner && !ctx.Org.IsOwner) ||
			(requireAdminTeam && !ctx.Org.IsAdminTeam) {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}
		ctx.Data["IsAdminTeam"] = ctx.Org.IsAdminTeam
		ctx.Data["IsOrganizationOwner"] = ctx.Org.IsOwner

		ctx.Org.OrgLink = "/org/" + ctx.Org.Organization.Name
		ctx.Data["OrgLink"] = ctx.Org.OrgLink
	}
}
