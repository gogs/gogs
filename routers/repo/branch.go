// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
	"net/url"
	"strings"
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

	if ctx.HasError() || !ctx.Repo.IsWriter() || branchName == oldBranchName {
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

	// Was successful, so now need to call models.CommitRepoAction() with the new commitID for webhooks and watchers
	if branch, err := ctx.Repo.Repository.GetBranch(branchName); err != nil {
		log.Error(4, "repo.Repository.GetBranch(%s): %v", branchName, err)
	} else if commit, err := branch.GetCommit(); err != nil {
		log.Error(4, "branch.GetCommit(): %v", err)
	} else {
		pc := &models.PushCommits{
			Len: 1,
			Commits: []*models.PushCommit{&models.PushCommit{
				commit.ID.String(),
				commit.Message(),
				commit.Author.Email,
				commit.Author.Name,
			}},
		}
		oldCommitID := "0000000000000000000000000000000000000000" // New Branch so we use all 0s
		newCommitID := commit.ID.String()
		if err := models.CommitRepoAction(ctx.User.ID, ctx.Repo.Owner.ID, ctx.User.LowerName, ctx.Repo.Owner.Email,
			ctx.Repo.Repository.ID, ctx.Repo.Owner.LowerName, ctx.Repo.Repository.Name, "refs/heads/"+branchName, pc,
			oldCommitID, newCommitID); err != nil {
			log.Error(4, "models.CommitRepoAction(branch = %s): %v", branchName, err)
		}
		models.HookQueue.Add(ctx.Repo.Repository.ID)
	}

	ctx.Redirect(EscapeUrl(ctx.Repo.RepoLink + "/src/" + branchName))
}
