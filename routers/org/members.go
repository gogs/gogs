// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	MEMBERS base.TplName = "org/members"
	INVITE  base.TplName = "org/invite"
)

func Members(ctx *middleware.Context) {
	org := ctx.Org.Organization
	ctx.Data["Title"] = org.Name
	ctx.Data["PageIsOrgMembers"] = true

	if err := org.GetMembers(); err != nil {
		ctx.Handle(500, "GetMembers", err)
		return
	}
	ctx.Data["Members"] = org.Members

	ctx.HTML(200, MEMBERS)
}

func MembersAction(ctx *middleware.Context) {
	uid := com.StrTo(ctx.Query("uid")).MustInt64()
	if uid == 0 {
		ctx.Redirect(ctx.Org.OrgLink + "/members")
		return
	}

	org := ctx.Org.Organization
	var err error
	switch ctx.Params(":action") {
	case "private":
		if ctx.User.Id != uid && !ctx.Org.IsOwner {
			ctx.Error(404)
			return
		}
		err = models.ChangeOrgUserStatus(org.Id, uid, false)
	case "public":
		if ctx.User.Id != uid {
			ctx.Error(404)
			return
		}
		err = models.ChangeOrgUserStatus(org.Id, uid, true)
	case "remove":
		if !ctx.Org.IsOwner {
			ctx.Error(404)
			return
		}
		err = org.RemoveMember(uid)
	}

	if err != nil {
		log.Error(4, "Action(%s): %v", ctx.Params(":action"), err)
		ctx.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}
	ctx.Redirect(ctx.Org.OrgLink + "/members")
}

func Invitation(ctx *middleware.Context) {
	org := ctx.Org.Organization
	ctx.Data["Title"] = org.Name
	ctx.Data["PageIsOrgMembers"] = true

	if ctx.Req.Method == "POST" {
		uname := ctx.Query("uname")
		u, err := models.GetUserByName(uname)
		if err != nil {
			if err == models.ErrUserNotExist {
				ctx.Flash.Error(ctx.Tr("form.user_not_exist"))
				ctx.Redirect(ctx.Org.OrgLink + "/invitations/new")
			} else {
				ctx.Handle(500, " GetUserByName", err)
			}
			return
		}

		if err = org.AddMember(u.Id); err != nil {
			ctx.Handle(500, " AddMember", err)
			return
		}

		log.Trace("New member added(%s): %s", org.Name, u.Name)
		ctx.Redirect(ctx.Org.OrgLink + "/members")
		return
	}

	ctx.HTML(200, INVITE)
}
