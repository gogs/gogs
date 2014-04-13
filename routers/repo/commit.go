// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"path"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

func Commits(ctx *middleware.Context, params martini.Params) {
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "repo.Commits", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Commits", nil)
		return
	}

	commitsCount, err := ctx.Repo.Commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "repo.Commits(GetCommitsCount)", err)
		return
	}

	// Calculate and validate page number.
	page, _ := base.StrTo(ctx.Query("p")).Int()
	if page < 1 {
		page = 1
	}
	lastPage := page - 1
	if lastPage < 0 {
		lastPage = 0
	}
	nextPage := page + 1
	if nextPage*50 > commitsCount {
		nextPage = 0
	}

	//both `git log branchName` and `git log  commitId` work
	commits, err := ctx.Repo.Commit.CommitsByRange(page)
	if err != nil {
		ctx.Handle(500, "repo.Commits(get commits)", err)
		return
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commitsCount
	ctx.Data["Commits"] = commits
	ctx.Data["LastPageNum"] = lastPage
	ctx.Data["NextPageNum"] = nextPage
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.HTML(200, "repo/commits")
}

func Diff(ctx *middleware.Context, params martini.Params) {
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	commitId := ctx.Repo.CommitId

	commit := ctx.Repo.Commit

	diff, err := models.GetDiff(models.RepoPath(userName, repoName), commitId)
	if err != nil {
		ctx.Handle(404, "repo.Diff", err)
		return
	}

	isImageFile := func(name string) bool {
		blob, err := ctx.Repo.Commit.GetBlobByPath(name)
		if err != nil {
			return false
		}

		data, err := blob.Data()
		if err != nil {
			return false
		}
		_, isImage := base.IsImageFile(data)
		return isImage
	}

	ctx.Data["IsImageFile"] = isImageFile
	ctx.Data["Title"] = commit.Message() + " · " + base.ShortSha(commitId)
	ctx.Data["Commit"] = commit
	ctx.Data["Diff"] = diff
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.Data["SourcePath"] = "/" + path.Join(userName, repoName, "src", commitId)
	ctx.Data["RawPath"] = "/" + path.Join(userName, repoName, "raw", commitId)
	ctx.HTML(200, "repo/diff")
}

func SearchCommits(ctx *middleware.Context, params martini.Params) {
	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		ctx.Redirect(ctx.Repo.RepoLink + "/commits/" + ctx.Repo.BranchName)
		return
	}

	userName := params["username"]
	repoName := params["reponame"]

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "repo.SearchCommits(GetBranches)", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.SearchCommits(GetBranches)", nil)
		return
	}

	commits, err := ctx.Repo.Commit.SearchCommits(keyword)
	if err != nil {
		ctx.Handle(500, "repo.SearchCommits(SearchCommits)", err)
		return
	}

	ctx.Data["Keyword"] = keyword
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["Commits"] = commits
	ctx.Data["IsSearchPage"] = true
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.HTML(200, "repo/commits")
}
