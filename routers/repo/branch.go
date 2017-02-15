// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
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

func DeleteBranchPost(ctx *context.Context) {
	branchName := ctx.Params("*")
	commitID := ctx.Query("commit")

	defer func() {
		redirectTo := ctx.Query("redirect_to")
		if len(redirectTo) == 0 {
			redirectTo = ctx.Repo.RepoLink
		}
		ctx.Redirect(redirectTo)
	}()

	if !ctx.Repo.GitRepo.IsBranchExist(branchName) {
		return
	}
	if len(commitID) > 0 {
		branchCommitID, err := ctx.Repo.GitRepo.GetBranchCommitID(branchName)
		if err != nil {
			log.Error(2, "GetBranchCommitID: %v", err)
			return
		}

		if branchCommitID != commitID {
			ctx.Flash.Error(ctx.Tr("repo.pulls.delete_branch_has_new_commits"))
			return
		}
	}

	if err := ctx.Repo.GitRepo.DeleteBranch(branchName, git.DeleteBranchOptions{
		Force: true,
	}); err != nil {
		log.Error(2, "DeleteBranch '%s': %v", branchName, err)
		return
	}
}
