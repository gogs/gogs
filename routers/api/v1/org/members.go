// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/routers/api/v1/convert"
	"github.com/gogits/gogs/routers/api/v1/user"
)

func AddOrgMembership(ctx *context.APIContext, form api.AddOrgMembershipOption) {
	org := user.GetUserByParamsName(ctx, ":orgname")
	member := user.GetUserByParamsName(ctx, ":username")
	if ctx.Written() {
		return
	}

	if !org.IsOwnedBy(ctx.User.Id) {
		ctx.Status(403)
		return
	}

	if err := org.AddMember(member.Id); err != nil {
		ctx.Error(500, "AddMember", err)
		return
	}
	if form.Role == "admin" {
		team, err := org.GetOwnerTeam();
		if err != nil {
			ctx.Error(500, "GetOwnerTeam", err)
			return
		}
		if err := team.AddMember(member.Id); err != nil {
			ctx.Error(500, "AddMember", err)
			return
		}
	}
	ret := map[string]interface{} {
		"organization": convert.ToOrganization(org),
		"user": convert.ToUser(member),
	}
	ctx.JSON(200, ret)
}
