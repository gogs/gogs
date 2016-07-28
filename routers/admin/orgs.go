// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/routers"
)

const (
	ORGS base.TplName = "admin/org/list"
)

func Organizations(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("admin.organizations")
	ctx.Data["PageIsAdmin"] = true
	ctx.Data["PageIsAdminOrganizations"] = true

	routers.RenderUserSearch(ctx, &routers.UserSearchOptions{
		Type:     models.USER_TYPE_ORGANIZATION,
		Counter:  models.CountOrganizations,
		Ranger:   models.Organizations,
		PageSize: setting.UI.Admin.OrgPagingNum,
		OrderBy:  "id ASC",
		TplName:  ORGS,
	})
}
