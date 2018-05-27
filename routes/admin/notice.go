// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"
	log "gopkg.in/clog.v1"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/setting"
)

const (
	NOTICES = "admin/notice"
)

func Notices(c *context.Context) {
	c.Data["Title"] = c.Tr("admin.notices")
	c.Data["PageIsAdmin"] = true
	c.Data["PageIsAdminNotices"] = true

	total := models.CountNotices()
	page := c.QueryInt("page")
	if page <= 1 {
		page = 1
	}
	c.Data["Page"] = paginater.New(int(total), setting.UI.Admin.NoticePagingNum, page, 5)

	notices, err := models.Notices(page, setting.UI.Admin.NoticePagingNum)
	if err != nil {
		c.Handle(500, "Notices", err)
		return
	}
	c.Data["Notices"] = notices

	c.Data["Total"] = total
	c.HTML(200, NOTICES)
}

func DeleteNotices(c *context.Context) {
	strs := c.QueryStrings("ids[]")
	ids := make([]int64, 0, len(strs))
	for i := range strs {
		id := com.StrTo(strs[i]).MustInt64()
		if id > 0 {
			ids = append(ids, id)
		}
	}

	if err := models.DeleteNoticesByIDs(ids); err != nil {
		c.Flash.Error("DeleteNoticesByIDs: " + err.Error())
		c.Status(500)
	} else {
		c.Flash.Success(c.Tr("admin.notices.delete_success"))
		c.Status(200)
	}
}

func EmptyNotices(c *context.Context) {
	if err := models.DeleteNotices(0, 0); err != nil {
		c.Handle(500, "DeleteNotices", err)
		return
	}

	log.Trace("System notices deleted by admin (%s): [start: %d]", c.User.Name, 0)
	c.Flash.Success(c.Tr("admin.notices.delete_success"))
	c.Redirect(setting.AppSubURL + "/admin/notices")
}
