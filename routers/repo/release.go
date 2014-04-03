// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func Releases(ctx *middleware.Context) {
	ctx.Data["Title"] = "Releases"
	ctx.Data["IsRepoToolbarReleases"] = true
	tags, err := models.GetTags(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
	if err != nil {
		ctx.Handle(404, "repo.Releases(GetTags)", err)
		return
	}
	ctx.Data["Releases"] = tags
	ctx.HTML(200, "release/list")
}
