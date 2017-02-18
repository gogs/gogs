// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"container/list"
	"path"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/context"
	"github.com/gogits/gogs/modules/setting"
)

const (
	COMMITS base.TplName = "repo/commits"
	DIFF    base.TplName = "repo/diff/page"
)

func RefCommits(ctx *context.Context) {
	switch {
	case len(ctx.Repo.TreePath) == 0:
		Commits(ctx)
	case ctx.Repo.TreePath == "search":
		SearchCommits(ctx)
	default:
		FileHistory(ctx)
	}
}

func RenderIssueLinks(oldCommits *list.List, repoLink string) *list.List {
	newCommits := list.New()
	for e := oldCommits.Front(); e != nil; e = e.Next() {
		c := e.Value.(*git.Commit)
		newCommits.PushBack(c)
	}
	return newCommits
}

func renderCommits(ctx *context.Context, filename string) {
	ctx.Data["Title"] = ctx.Tr("repo.commits.commit_history") + " · " + ctx.Repo.Repository.FullName()
	ctx.Data["PageIsCommits"] = true

	page := ctx.QueryInt("page")
	if page < 1 {
		page = 1
	}
	pageSize := ctx.QueryInt("pageSize")
	if pageSize < 1 {
		pageSize = git.DEFAULT_COMMITS_PAGE_SIZE
	}

	// Both 'git log branchName' and 'git log commitID' work.
	var err error
	var commits *list.List
	if len(filename) == 0 {
		commits, err = ctx.Repo.Commit.CommitsByRangeSize(page, pageSize)
	} else {
		commits, err = ctx.Repo.GitRepo.CommitsByFileAndRangeSize(ctx.Repo.BranchName, filename, page, pageSize)
	}
	if err != nil {
		ctx.Handle(500, "CommitsByRangeSize/CommitsByFileAndRangeSize", err)
		return
	}
	commits = RenderIssueLinks(commits, ctx.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)
	ctx.Data["Commits"] = commits

	if page > 1 {
		ctx.Data["HasPrevious"] = true
		ctx.Data["PreviousPage"] = page - 1
	}
	if commits.Len() == pageSize {
		ctx.Data["HasNext"] = true
		ctx.Data["NextPage"] = page + 1
	}
	ctx.Data["PageSize"] = pageSize

	ctx.Data["Username"] = ctx.Repo.Owner.Name
	ctx.Data["Reponame"] = ctx.Repo.Repository.Name
	ctx.HTML(200, COMMITS)
}

func Commits(ctx *context.Context) {
	renderCommits(ctx, "")
}

func SearchCommits(ctx *context.Context) {
	ctx.Data["PageIsCommits"] = true

	keyword := ctx.Query("q")
	if len(keyword) == 0 {
		ctx.Redirect(ctx.Repo.RepoLink + "/commits/" + ctx.Repo.BranchName)
		return
	}

	commits, err := ctx.Repo.Commit.SearchCommits(keyword)
	if err != nil {
		ctx.Handle(500, "SearchCommits", err)
		return
	}
	commits = RenderIssueLinks(commits, ctx.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)
	ctx.Data["Commits"] = commits

	ctx.Data["Keyword"] = keyword
	ctx.Data["Username"] = ctx.Repo.Owner.Name
	ctx.Data["Reponame"] = ctx.Repo.Repository.Name
	ctx.Data["Branch"] = ctx.Repo.BranchName
	ctx.HTML(200, COMMITS)
}

func FileHistory(ctx *context.Context) {
	renderCommits(ctx, ctx.Repo.TreePath)
}

func Diff(ctx *context.Context) {
	ctx.Data["PageIsDiff"] = true
	ctx.Data["RequireHighlightJS"] = true

	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	commitID := ctx.Params(":sha")

	commit, err := ctx.Repo.GitRepo.GetCommit(commitID)
	if err != nil {
		if git.IsErrNotExist(err) {
			ctx.Handle(404, "Repo.GitRepo.GetCommit", err)
		} else {
			ctx.Handle(500, "Repo.GitRepo.GetCommit", err)
		}
		return
	}

	diff, err := models.GetDiffCommit(models.RepoPath(userName, repoName),
		commitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		ctx.Handle(404, "GetDiffCommit", err)
		return
	}

	parents := make([]string, commit.ParentCount())
	for i := 0; i < commit.ParentCount(); i++ {
		sha, err := commit.ParentID(i)
		parents[i] = sha.String()
		if err != nil {
			ctx.Handle(404, "repo.Diff", err)
			return
		}
	}

	setEditorconfigIfExists(ctx)
	if ctx.Written() {
		return
	}

	ctx.Data["CommitID"] = commitID
	ctx.Data["IsSplitStyle"] = ctx.Query("style") == "split"
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["IsImageFile"] = commit.IsImageFile
	ctx.Data["Title"] = commit.Summary() + " · " + base.ShortSha(commitID)
	ctx.Data["Commit"] = commit
	ctx.Data["Author"] = models.ValidateCommitWithEmail(commit)
	ctx.Data["Diff"] = diff
	ctx.Data["Parents"] = parents
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["SourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", commitID)
	if commit.ParentCount() > 0 {
		ctx.Data["BeforeSourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", parents[0])
	}
	ctx.Data["RawPath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "raw", commitID)
	ctx.HTML(200, DIFF)
}

func RawDiff(ctx *context.Context) {
	if err := models.GetRawDiff(
		models.RepoPath(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name),
		ctx.Params(":sha"),
		models.RawDiffType(ctx.Params(":ext")),
		ctx.Resp,
	); err != nil {
		ctx.Handle(500, "GetRawDiff", err)
		return
	}
}

func CompareDiff(ctx *context.Context) {
	ctx.Data["IsDiffCompare"] = true
	userName := ctx.Repo.Owner.Name
	repoName := ctx.Repo.Repository.Name
	beforeCommitID := ctx.Params(":before")
	afterCommitID := ctx.Params(":after")

	commit, err := ctx.Repo.GitRepo.GetCommit(afterCommitID)
	if err != nil {
		ctx.Handle(404, "GetCommit", err)
		return
	}

	diff, err := models.GetDiffRange(models.RepoPath(userName, repoName), beforeCommitID,
		afterCommitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		ctx.Handle(404, "GetDiffRange", err)
		return
	}

	commits, err := commit.CommitsBeforeUntil(beforeCommitID)
	if err != nil {
		ctx.Handle(500, "CommitsBeforeUntil", err)
		return
	}
	commits = models.ValidateCommitsWithEmails(commits)

	ctx.Data["IsSplitStyle"] = ctx.Query("style") == "split"
	ctx.Data["CommitRepoLink"] = ctx.Repo.RepoLink
	ctx.Data["Commits"] = commits
	ctx.Data["CommitsCount"] = commits.Len()
	ctx.Data["BeforeCommitID"] = beforeCommitID
	ctx.Data["AfterCommitID"] = afterCommitID
	ctx.Data["Username"] = userName
	ctx.Data["Reponame"] = repoName
	ctx.Data["IsImageFile"] = commit.IsImageFile
	ctx.Data["Title"] = "Comparing " + base.ShortSha(beforeCommitID) + "..." + base.ShortSha(afterCommitID) + " · " + userName + "/" + repoName
	ctx.Data["Commit"] = commit
	ctx.Data["Diff"] = diff
	ctx.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	ctx.Data["SourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", afterCommitID)
	ctx.Data["BeforeSourcePath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "src", beforeCommitID)
	ctx.Data["RawPath"] = setting.AppSubUrl + "/" + path.Join(userName, repoName, "raw", afterCommitID)
	ctx.HTML(200, DIFF)
}
