package org

import (
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
)

const templOrgMemberInvite = "org/member/invite"

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

	c.Success(templOrgMemberInvite)
}
