// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/log"
	"strings"
	"net/url"
)

const (
	BRANCH base.TplName = "repo/branch"
)

func Branches(ctx *context.Context) {
	ctx.Data["Title"] = "Branches"
	ctx.Data["IsRepoToolbarBranches"] = true

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "repo.Branches(GetBranches)", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Branches(GetBranches)", nil)
		return
	}

	ctx.Data["Branches"] = brs
	ctx.HTML(200, BRANCH)
}

func NewBranchPost(ctx *context.Context, form auth.NewBranchForm) {
	oldBranchName := form.OldBranchName
	branchName := form.BranchName

	if ctx.HasError() || ! ctx.Repo.IsWriter() || branchName == oldBranchName {
		ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/src/" + oldBranchName))
		return
	}

	branchName = url.QueryEscape(strings.Replace(strings.Trim(branchName, " "), " ", "-", -1))

	if _, err := ctx.Repo.Repository.GetBranch(branchName); err == nil {
		ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/src/" + branchName))
		return
	}

	if err := ctx.Repo.Repository.CreateNewBranch(ctx.User, oldBranchName, branchName); err != nil {
		ctx.Handle(404, "repo.Branches(CreateNewBranch)", err)
		log.Error(4, "%s: %v", "EditFile", err)
		return
	}

	ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/src/" + branchName))
}
