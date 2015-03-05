// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

const (
	PULLS    base.TplName = "repo/pulls"
	NEW_PULL base.TplName = "repo/pull_new"
)

func Pulls(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarPulls"] = true
	ctx.HTML(200, PULLS)
}

func NewPullRequest(ctx *middleware.Context) {
	repo := ctx.Repo.Repository
	if !repo.IsFork {
		ctx.Redirect(ctx.Repo.RepoLink)
		return
	}
	ctx.Data["RequestFrom"] = repo.Owner.Name + "/" + repo.Name

	if err := ctx.Repo.Repository.GetForkRepo(); err != nil {
		ctx.Handle(500, "GetForkRepo", err)
		return
	}

	forkRepo := ctx.Repo.Repository.ForkRepo
	if err := forkRepo.GetOwner(); err != nil {
		ctx.Handle(500, "GetOwner", err)
		return
	}
	ctx.Data["RequestTo"] = forkRepo.Owner.Name + "/" + forkRepo.Name
	ctx.HTML(200, NEW_PULL)
}
