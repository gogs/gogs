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
			requireMember bool
			requireOwner  bool
		)
		if len(args) >= 1 {
			requireMember = args[0]
		}
		if len(args) >= 2 {
			requireOwner = args[1]
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
			} else {
				ctx.Org.IsMember = ctx.Org.Organization.IsOrgMember(ctx.User.Id)
			}
		}
		if (requireMember && !ctx.Org.IsMember) || (requireOwner && !ctx.Org.IsOwner) {
			ctx.Handle(404, "OrgAssignment", err)
			return
		}
	}
}
