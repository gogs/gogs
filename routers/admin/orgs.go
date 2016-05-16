// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package admin

import (
	"github.com/gigforks/gogs/models"
	"github.com/gigforks/gogs/modules/base"
	"github.com/gigforks/gogs/modules/context"
	"github.com/gigforks/gogs/modules/setting"
	"github.com/gigforks/gogs/routers"
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
		PageSize: setting.AdminOrgPagingNum,
		OrderBy:  "id ASC",
		TplName:  ORGS,
	})
}
