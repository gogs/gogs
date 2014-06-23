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

const (
	COMMITS base.TplName = "repo/commits"
	DIFF    base.TplName = "repo/diff"
)

func Commits(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarCommits"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "repo.Commits(GetBranches)", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.Commits(GetBranches)", nil)
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

	// Both `git log branchName` and `git log commitId` work.
	ctx.Data["Commits"], err = ctx.Repo.Commit.CommitsByRange(page)
	if err != nil {
		ctx.Handle(500, "repo.Commits(CommitsByRange)", err)
		return
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commitsCount
	ctx.Data["LastPageNum"] = lastPage
	ctx.Data["NextPageNum"] = nextPage
	ctx.HTML(200, COMMITS)
}

func SearchCommits(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsSearchPage"] = true
	ctx.Data["IsRepoToolbarCommits"] = true

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
	ctx.HTML(200, COMMITS)
}

func Diff(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarCommits"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	commitId := ctx.Repo.CommitId

	commit := ctx.Repo.Commit

	diff, err := models.GetDiff(models.RepoPath(userName, repoName), commitId)
	if err != nil {
		ctx.Handle(404, "repo.Diff(GetDiff)", err)
		return
	}

	isImageFile := func(name string) bool {
		blob, err := ctx.Repo.Commit.GetBlobByPath(name)
		if err != nil {
			return false
		}

		dataRc, err := blob.Data()
		if err != nil {
			return false
		}
		buf := make([]byte, 1024)
		n, _ := dataRc.Read(buf)
		if n > 0 {
			buf = buf[:n]
		}
		dataRc.Close()
		_, isImage := base.IsImageFile(buf)
		return isImage
	}

	parents := make([]string, commit.ParentCount())
	for i := 0; i < commit.ParentCount(); i++ {
		sha, err := commit.ParentId(i)
		parents[i] = sha.String()
		if err != nil {
			ctx.Handle(404, "repo.Diff", err)
			return
		}
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["IsImageFile"] = isImageFile
	ctx.Data["Title"] = commit.Summary() + " · " + base.ShortSha(commitId)
	ctx.Data["Commit"] = commit
	ctx.Data["Diff"] = diff
	ctx.Data["Parents"] = parents
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["SourcePath"] = "/" + path.Join(userName, repoName, "src", commitId)
	ctx.Data["RawPath"] = "/" + path.Join(userName, repoName, "raw", commitId)
	ctx.HTML(200, DIFF)
}

func FileHistory(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarCommits"] = true

	fileName := params["_1"]
	if len(fileName) == 0 {
		Commits(ctx, params)
		return
	}

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	branchName := params["branchname"]

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "repo.FileHistory", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "repo.FileHistory", nil)
		return
	}

	commitsCount, err := ctx.Repo.GitRepo.FileCommitsCount(branchName, fileName)
	if err != nil {
		ctx.Handle(500, "repo.FileHistory(GetCommitsCount)", err)
		return
	} else if commitsCount == 0 {
		ctx.Handle(404, "repo.FileHistory", nil)
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

	ctx.Data["Commits"], err = ctx.Repo.GitRepo.CommitsByFileAndRange(
		branchName, fileName, page)
	if err != nil {
		ctx.Handle(500, "repo.FileHistory(CommitsByRange)", err)
		return
	}

	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["FileName"] = fileName
	ctx.Data["CommitCount"] = commitsCount
	ctx.Data["LastPageNum"] = lastPage
	ctx.Data["NextPageNum"] = nextPage
	ctx.HTML(200, COMMITS)
}
