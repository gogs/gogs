// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/log"
)

func DeleteFilePost(ctx *context.Context, form auth.DeleteRepoFileForm) {
	branchName := ctx.Repo.BranchName
	treeName := ctx.Repo.TreeName

	if ctx.HasError() {
		ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName)
		return
	}

	if err := ctx.Repo.Repository.DeleteRepoFile(ctx.User, branchName, treeName, form.CommitSummary); err != nil {
		ctx.Handle(500, "DeleteRepoFile", err)
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
		oldCommitID := ctx.Repo.CommitID
		newCommitID := commit.ID.String()
		if err := models.CommitRepoAction(ctx.User.ID, ctx.Repo.Owner.ID, ctx.User.LowerName, ctx.Repo.Owner.Email,
			ctx.Repo.Repository.ID, ctx.Repo.Owner.LowerName, ctx.Repo.Repository.Name, "refs/heads/"+branchName, pc,
			oldCommitID, newCommitID); err != nil {
			log.Error(4, "models.CommitRepoAction(branch = %s): %v", branchName, err)
		}
		models.HookQueue.Add(ctx.Repo.Repository.ID)
	}

	ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName)
}
