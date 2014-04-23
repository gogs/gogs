// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/go-martini/martini"

	"github.com/gogits/gogs/modules/middleware"
)

func Branches(ctx *middleware.Context, params martini.Params) {
	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(404, "repo.Branches", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Branches", nil)
		return
	}

	ctx.Data["Branches"] = brs
	ctx.Data["IsRepoToolbarBranches"] = true

	ctx.HTML(200, "repo/branches")
}
