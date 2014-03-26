// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"github.com/codegangsta/martini"
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

func Commits(ctx *middleware.Context, params martini.Params) {
	brs, err := models.GetBranches(params["username"], params["reponame"])
	if err != nil {
		ctx.Handle(200, "repo.Commits", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Commits", nil)
		return
	}

	ctx.Data["IsRepoToolbarCommits"] = true
	commits, err := models.GetCommits(params["username"],
		params["reponame"], params["branchname"])
	if err != nil {
		ctx.Handle(404, "repo.Commits", nil)
		return
	}
	ctx.Data["Username"] = params["username"]
	ctx.Data["Reponame"] = params["reponame"]
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["Commits"] = commits
	ctx.HTML(200, "repo/commits")
}

func Diff(ctx *middleware.Context, params martini.Params) {
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

	shortSha := params["commitid"][:7]
	ctx.Data["Title"] = commit.Message() + " · " + shortSha
	ctx.Data["Commit"] = commit
	ctx.Data["ShortSha"] = shortSha
	ctx.Data["Diff"] = diff
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.HTML(200, "repo/diff")
}
