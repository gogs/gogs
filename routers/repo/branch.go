// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/codegangsta/martini"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func Branches(ctx *middleware.Context, params martini.Params) {
	if !ctx.Repo.IsValid {
		return
	}

	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Branches", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Branches", nil)
		return
	}

	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]

	if len(params["branchname"]) == 0 {
		params["branchname"] = "master"
	}
	ctx.Data["Branchname"] = params["branchname"]
	ctx.Data["Branches"] = brs
	ctx.Data["IsRepoToolbarBranches"] = true

	ctx.HTML(200, "repo/branches")
}
