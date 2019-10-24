// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package org

import (
	"github.com/unknwon/com"
	log "gopkg.in/clog.v1"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/setting"
)

const (
	MEMBERS       = "org/member/members"
	MEMBER_INVITE = "org/member/invite"
)

func Members(c *context.Context) {
	org := c.Org.Organization
	c.Data["Title"] = org.FullName
	c.Data["PageIsOrgMembers"] = true

	if err := org.GetMembers(); err != nil {
		c.Handle(500, "GetMembers", err)
		return
	}
	c.Data["Members"] = org.Members

	c.HTML(200, MEMBERS)
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
			c.Error(404)
			return
		}
		err = db.ChangeOrgUserStatus(org.ID, uid, false)
	case "public":
		if c.User.ID != uid && !c.Org.IsOwner {
			c.Error(404)
			return
		}
		err = db.ChangeOrgUserStatus(org.ID, uid, true)
	case "remove":
		if !c.Org.IsOwner {
			c.Error(404)
			return
		}
		err = org.RemoveMember(uid)
		if db.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
			c.Redirect(c.Org.OrgLink + "/members")
			return
		}
	case "leave":
		err = org.RemoveMember(c.User.ID)
		if db.IsErrLastOrgOwner(err) {
			c.Flash.Error(c.Tr("form.last_org_owner"))
			c.Redirect(c.Org.OrgLink + "/members")
			return
		}
	}

	if err != nil {
		log.Error(4, "Action(%s): %v", c.Params(":action"), err)
		c.JSON(200, map[string]interface{}{
			"ok":  false,
			"err": err.Error(),
		})
		return
	}

	if c.Params(":action") != "leave" {
		c.Redirect(c.Org.OrgLink + "/members")
	} else {
		c.Redirect(setting.AppSubURL + "/")
	}
}

func Invitation(c *context.Context) {
	org := c.Org.Organization
	c.Data["Title"] = org.FullName
	c.Data["PageIsOrgMembers"] = true

	if c.Req.Method == "POST" {
		uname := c.Query("uname")
		u, err := db.GetUserByName(uname)
		if err != nil {
			if errors.IsUserNotExist(err) {
				c.Flash.Error(c.Tr("form.user_not_exist"))
				c.Redirect(c.Org.OrgLink + "/invitations/new")
			} else {
				c.Handle(500, " GetUserByName", err)
			}
			return
		}

		if err = org.AddMember(u.ID); err != nil {
			c.Handle(500, " AddMember", err)
			return
		}

		log.Trace("New member added(%s): %s", org.Name, u.Name)
		c.Redirect(c.Org.OrgLink + "/members")
		return
	}

	c.HTML(200, MEMBER_INVITE)
}
