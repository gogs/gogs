// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"container/list"
	"fmt"
	"path"

	"github.com/codegangsta/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func Commits(ctx *middleware.Context, params martini.Params) {
	userName := params["username"]
	repoName := params["reponame"]
	branchName := params["branchname"]

	brs, err := models.GetBranches(userName, repoName)
	if err != nil {
		ctx.Handle(200, "repo.Commits", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Commits", nil)
		return
	}

	var commits *list.List
	if models.IsBranchExist(userName, repoName, branchName) {
		commits, err = models.GetCommitsByBranch(userName, repoName, branchName)
	} else {
		commits, err = models.GetCommitsByCommitId(userName, repoName, branchName)
	}

	if err != nil {
		ctx.Handle(404, "repo.Commits", nil)
		return
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["Commits"] = commits
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.HTML(200, "repo/commits")
}

func Diff(ctx *middleware.Context, params martini.Params) {
	fmt.Println(params["branchname"])
	commit, err := models.GetCommit(params["username"], params["reponame"], params["branchname"], params["commitid"])
	if err != nil {
		ctx.Handle(404, "repo.Diff", err)
		return
	}

	diff, err := models.GetDiff(models.RepoPath(params["username"], params["reponame"]), params["commitid"])
	if err != nil {
		ctx.Handle(404, "repo.Diff", err)
		return
	}

	shortSha := params["commitid"][:10]
	ctx.Data["Title"] = commit.Message() + " · " + shortSha
	ctx.Data["Commit"] = commit
	ctx.Data["ShortSha"] = shortSha
	ctx.Data["Diff"] = diff
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.Data["SourcePath"] = "/" + path.Join(params["username"], params["reponame"], "src", params["commitid"])
	ctx.HTML(200, "repo/diff")
}
