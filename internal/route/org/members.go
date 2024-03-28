// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

const (
	MEMBERS       = "org/member/members"
	MEMBER_INVITE = "org/member/invite"
)

func Members(c *context.Context) {
	org := c.Org.Organization
	c.Data["Title"] = org.FullName
	c.Data["PageIsOrgMembers"] = true

	if err := org.GetMembers(0); err != nil {
		c.Error(err, "get members")
		return
	}
	c.Data["Members"] = org.Members

	c.Success(MEMBERS)
}

func MembersAction(c *context.Context) {
	uid := com.StrTo(c.Query("uid")).MustInt64()
	if uid == 0 {
		c.Redirect(c.Org.OrgLink + "/members")
		return
	}

	org := c.Org.Organization
	var err error
	switch c.Params(":action") {
	case "private":
		if c.User.ID != uid && !c.Org.IsOwner {
			c.NotFound()
			return
		}
		err = database.ChangeOrgUserStatus(org.ID, uid, false)
	case "public":
		if c.User.ID != uid && !c.Org.IsOwner {
			c.NotFound()
			return
		}
		err = database.ChangeOrgUserStatus(org.ID, uid, true)
	case "remove":
		if !c.Org.IsOwner {
			c.NotFound()
			return
		}
		err = org.RemoveMember(uid)
		if database.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
			c.Redirect(c.Org.OrgLink + "/members")
			return
		}
	case "leave":
		err = org.RemoveMember(c.User.ID)
		if database.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
			c.Redirect(c.Org.OrgLink + "/members")
			return
		}
	}

	if err != nil {
		log.Error("Action(%s): %v", c.Params(":action"), err)
		c.JSONSuccess(map[string]any{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}

	if c.Params(":action") != "leave" {
		c.Redirect(c.Org.OrgLink + "/members")
	} else {
		c.Redirect(conf.Server.Subpath + "/")
	}
}

func Invitation(c *context.Context) {
	org := c.Org.Organization
	c.Data["Title"] = org.FullName
	c.Data["PageIsOrgMembers"] = true

	if c.Req.Method == "POST" {
		uname := c.Query("uname")
		u, err := database.Handle.Users().GetByUsername(c.Req.Context(), uname)
		if err != nil {
			if database.IsErrUserNotExist(err) {
				c.Flash.Error(c.Tr("form.user_not_exist"))
				c.Redirect(c.Org.OrgLink + "/invitations/new")
			} else {
				c.Error(err, "get user by name")
			}
			return
		}

		if err = org.AddMember(u.ID); err != nil {
			c.Error(err, "add member")
			return
		}

		log.Trace("New member added(%s): %s", org.Name, u.Name)
		c.Redirect(c.Org.OrgLink + "/members")
		return
	}

	c.Success(MEMBER_INVITE)
}
