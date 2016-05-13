// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/auth"
)

func DeleteFilePost(ctx *context.Context, form auth.DeleteRepoFileForm) {
	branchName := ctx.Repo.BranchName
	treeName := ctx.Repo.TreeName

	if ctx.HasError() || ! ctx.Repo.IsWriter() {
		ctx.Redirect(ctx.Repo.RepoLink + "/src/" + branchName + "/" + treeName)
		return
	}

	if err := ctx.Repo.Repository.DeleteRepoFile(ctx.User, branchName, treeName, form.CommitSummary); err != nil {
		ctx.Handle(500, "DeleteRepoFile", err)
		return
	}

	ctx.Redirect(ctx.Repo.RepoLink+"/src/"+branchName)
}
