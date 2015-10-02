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
	STARS base.TplName = "repo/stars"
)

func Stars(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repos.stars")

	page := ctx.QueryInt("page")
	if page <= 0 {
		page = 1
	}

	ctx.Data["Page"] = paginater.New(ctx.Repo.Repository.NumStars, models.ItemsPerPage, page, 5)

	stars, err := ctx.Repo.Repository.GetStars(ctx.QueryInt("page"))

	if err != nil {
		ctx.Handle(500, "GetStars", err)
		return
	}

	if (ctx.QueryInt("page")-1)*models.ItemsPerPage > ctx.Repo.Repository.NumStars {
		ctx.Handle(404, "ctx.Repo.Repository.NumStars", nil)
		return
	}

	ctx.Data["Stars"] = stars

	ctx.HTML(200, STARS)
}
