// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	NOTICES base.TplName = "admin/notice"
)

func Notices(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.notices")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminNotices"] = true

	total := models.CountNotices()
	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}
	ctx.Data["Page"] = paginater.New(int(total), setting.AdminNoticePagingNum, page, 5)

	notices, err := models.Notices(page, setting.AdminNoticePagingNum)
	if err != nil {
		ctx.Handle(500, "Notices", err)
		return
	}
	ctx.Data["Notices"] = notices

	ctx.Data["Total"] = total
	ctx.HTML(200, NOTICES)
}

func DeleteNotice(ctx *middleware.Context) {
	id := ctx.ParamsInt64(":id")
	if err := models.DeleteNotice(id); err != nil {
		ctx.Handle(500, "DeleteNotice", err)
		return
	}
	log.Trace("System notice deleted by admin(%s): %d", ctx.User.Name, id)
	ctx.Flash.Success(ctx.Tr("admin.notices.delete_success"))
	ctx.Redirect("/admin/notices")
}
