// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Issues(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarIssues"] = true

	milestoneId, _ := base.StrTo(params["milestone"]).Int()
	page, _ := base.StrTo(params["page"]).Int()

	var err error
	ctx.Data["Issues"], err = models.GetIssues(0, ctx.Repo.Repository.Id, 0,
		int64(milestoneId), page, params["state"] == "closed", false, params["labels"], params["sortType"])
	if err != nil {
		ctx.Handle(200, "issue.Issues: %v", err)
		return
	}

	ctx.HTML(200, "repo/issues")
}
