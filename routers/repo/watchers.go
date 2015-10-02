// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	WATCHERS base.TplName = "repo/watchers"
)

func Watchers(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repos.watches")

	page := ctx.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	ctx.Data["Page"] = paginater.New(ctx.Repo.Repository.NumWatches, models.ItemsPerPage, page, 5)

	watchers, err := ctx.Repo.Repository.GetWatchers(ctx.QueryInt("page"))

	if err != nil {
		ctx.Handle(500, "GetWatchers", err)
		return
	}

	if (ctx.QueryInt("page")-1)*models.ItemsPerPage > ctx.Repo.Repository.NumWatches {
		ctx.Handle(404, "ctx.Repo.Repository.NumWatches", nil)
		return
	}

	ctx.Data["Watchers"] = watchers

	ctx.HTML(200, WATCHERS)
}
