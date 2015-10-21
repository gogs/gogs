// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	ORGS base.TplName = "admin/org/list"
)

func Organizations(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.organizations")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminOrganizations"] = true

	total := models.CountOrganizations()
	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}
	ctx.Data["Page"] = paginater.New(int(total), setting.AdminOrgPagingNum, page, 5)
 
    orgs, err := models.Organizations(page, setting.AdminOrgPagingNum)
	
	if err != nil {
		ctx.Handle(500, "Organizations", err)
		return
	}
	
 	ctx.Data["Orgs"] = orgs
	ctx.Data["Total"] = total

	ctx.HTML(200, ORGS)
}
