// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	NOTICES base.TplName = "admin/notice"
)

func Notices(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.notices")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminNotices"] = true

	pageNum := 50
	p := pagination(ctx, models.CountNotices(), pageNum)

	notices, err := models.GetNotices(pageNum, (p-1)*pageNum)
	if err != nil {
		ctx.Handle(500, "GetNotices", err)
		return
	}
	ctx.Data["Notices"] = notices
	ctx.HTML(200, NOTICES)
}

func DeleteNotice(ctx *middleware.Context) {
	id := com.StrTo(ctx.Params(":id")).MustInt64()
	if err := models.DeleteNotice(id); err != nil {
		ctx.Handle(500, "DeleteNotice", err)
		return
	}
	log.Trace("System notice deleted by admin(%s): %d", ctx.User.Name, id)
	ctx.Flash.Success(ctx.Tr("admin.notices.delete_success"))
	ctx.Redirect("/admin/notices")
}
