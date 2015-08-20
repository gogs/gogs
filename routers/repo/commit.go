// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"container/list"
	"path"

	"github.com/Unknwon/com"
	"github.com/Unknwon/paginater"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	COMMITS base.TplName = "repo/commits"
	DIFF    base.TplName = "repo/diff"
)

func RefCommits(ctx *middleware.Context) {
	switch {
	case len(ctx.Repo.TreeName) == 0:
		Commits(ctx)
	case ctx.Repo.TreeName == "search":
		SearchCommits(ctx)
	default:
		FileHistory(ctx)
	}
}

func RenderIssueLinks(oldCommits *list.List, repoLink string) *list.List {
	newCommits := list.New()
	for e := oldCommits.Front(); e != nil; e = e.Next() {
		c := e.Value.(*git.Commit)
		c.CommitMessage = c.CommitMessage
		newCommits.PushBack(c)
	}
	return newCommits
}

func Commits(ctx *middleware.Context) {
	ctx.Data["PageIsCommits"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "GetBranches", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "GetBranches", nil)
		return
	}

	commitsCount, err := ctx.Repo.Commit.CommitsCount()
	if err != nil {
		ctx.Handle(500, "GetCommitsCount", err)
		return
	}

	page := ctx.QueryInt("page")
	if page <= 1 {
		page = 1
	}
	ctx.Data["Page"] = paginater.New(commitsCount, git.CommitsRangeSize, page, 5)

	// Both `git log branchName` and `git log commitId` work.
	commits, err := ctx.Repo.Commit.CommitsByRange(page)
	if err != nil {
		ctx.Handle(500, "CommitsByRange", err)
		return
	}
	commits = RenderIssueLinks(commits, ctx.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)

	ctx.Data["Commits"] = commits
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commitsCount
	ctx.HTML(200, COMMITS)
}

func SearchCommits(ctx *middleware.Context) {
	ctx.Data["PageIsCommits"] = true

	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		ctx.Redirect(ctx.Repo.RepoLink + "/commits/" + ctx.Repo.BranchName)
		return
	}

	userName := ctx.Params(":username")
	repoName := ctx.Params(":reponame")

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "GetBranches", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "GetBranches", nil)
		return
	}

	commits, err := ctx.Repo.Commit.SearchCommits(keyword)
	if err != nil {
		ctx.Handle(500, "SearchCommits", err)
		return
	}
	commits = RenderIssueLinks(commits, ctx.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)

	ctx.Data["Keyword"] = keyword
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["Commits"] = commits
	ctx.HTML(200, COMMITS)
}

func FileHistory(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarCommits"] = true

	fileName := ctx.Repo.TreeName
	if len(fileName) == 0 {
		Commits(ctx)
		return
	}

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	branchName := ctx.Repo.BranchName

	brs, err := ctx.Repo.GitRepo.GetBranches()
	if err != nil {
		ctx.Handle(500, "GetBranches", err)
		return
	} else if len(brs) == 0 {
		ctx.Handle(404, "GetBranches", nil)
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
	page := com.StrTo(ctx.Query("p")).MustInt()
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

	commits, err := ctx.Repo.GitRepo.CommitsByFileAndRange(
		branchName, fileName, page)
	if err != nil {
		ctx.Handle(500, "repo.FileHistory(CommitsByRange)", err)
		return
	}
	commits = RenderIssueLinks(commits, ctx.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)

	ctx.Data["Commits"] = commits
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["FileName"] = fileName
	ctx.Data["CommitCount"] = commitsCount
	ctx.Data["LastPageNum"] = lastPage
	ctx.Data["NextPageNum"] = nextPage
	ctx.HTML(200, COMMITS)
}

func Diff(ctx *middleware.Context) {
	ctx.Data["PageIsDiff"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	commitId := ctx.Repo.CommitId

	commit := ctx.Repo.Commit
	commit.CommitMessage = commit.CommitMessage
	diff, err := models.GetDiffCommit(models.RepoPath(userName, repoName),
		commitId, setting.Git.MaxGitDiffLines)
	if err != nil {
		ctx.Handle(404, "GetDiffCommit", err)
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
	ctx.Data["Author"] = models.ValidateCommitWithEmail(commit)
	ctx.Data["Diff"] = diff
	ctx.Data["Parents"] = parents
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["SourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", commitId)
	if commit.ParentCount() > 0 {
		ctx.Data["BeforeSourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", parents[0])
	}
	ctx.Data["RawPath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "raw", commitId)
	ctx.HTML(200, DIFF)
}

func CompareDiff(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarCommits"] = true
	ctx.Data["IsDiffCompare"] = true
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	beforeCommitId := ctx.Params(":before")
	afterCommitId := ctx.Params(":after")

	commit, err := ctx.Repo.GitRepo.GetCommit(afterCommitId)
	if err != nil {
		ctx.Handle(404, "GetCommit", err)
		return
	}

	diff, err := models.GetDiffRange(models.RepoPath(userName, repoName), beforeCommitId,
		afterCommitId, setting.Git.MaxGitDiffLines)
	if err != nil {
		ctx.Handle(404, "GetDiffRange", err)
		return
	}

	isImageFile := func(name string) bool {
		blob, err := commit.GetBlobByPath(name)
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
		_, isImage := base.IsImageFile(buf)
		return isImage
	}

	commits, err := commit.CommitsBeforeUntil(beforeCommitId)
	if err != nil {
		ctx.Handle(500, "CommitsBeforeUntil", err)
		return
	}
	commits = models.ValidateCommitsWithEmails(commits)

	ctx.Data["Commits"] = commits
	ctx.Data["CommitCount"] = commits.Len()
	ctx.Data["BeforeCommitId"] = beforeCommitId
	ctx.Data["AfterCommitId"] = afterCommitId
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["IsImageFile"] = isImageFile
	ctx.Data["Title"] = "Comparing " + base.ShortSha(beforeCommitId) + "..." + base.ShortSha(afterCommitId) + " · " + userName + "/" + repoName
	ctx.Data["Commit"] = commit
	ctx.Data["Diff"] = diff
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["SourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", afterCommitId)
	ctx.Data["BeforeSourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", beforeCommitId)
	ctx.Data["RawPath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "raw", afterCommitId)
	ctx.HTML(200, DIFF)
}
