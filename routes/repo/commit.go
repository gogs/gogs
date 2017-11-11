// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"container/list"
	"path"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/tool"
)

const (
	COMMITS = "repo/commits"
	DIFF    = "repo/diff/page"
)

func RefCommits(c *context.Context) {
	c.Data["PageIsViewFiles"] = true
	switch {
	case len(c.Repo.TreePath) == 0:
		Commits(c)
	case c.Repo.TreePath == "search":
		SearchCommits(c)
	default:
		FileHistory(c)
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

func renderCommits(c *context.Context, filename string) {
	c.Data["Title"] = c.Tr("repo.commits.commit_history") + " · " + c.Repo.Repository.FullName()
	c.Data["PageIsCommits"] = true

	page := c.QueryInt("page")
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("pageSize")
	if pageSize < 1 {
		pageSize = git.DefaultCommitsPageSize
	}

	// Both 'git log branchName' and 'git log commitID' work.
	var err error
	var commits *list.List
	if len(filename) == 0 {
		commits, err = c.Repo.Commit.CommitsByRangeSize(page, pageSize)
	} else {
		commits, err = c.Repo.GitRepo.CommitsByFileAndRangeSize(c.Repo.BranchName, filename, page, pageSize)
	}
	if err != nil {
		c.Handle(500, "CommitsByRangeSize/CommitsByFileAndRangeSize", err)
		return
	}
	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)
	c.Data["Commits"] = commits

	if page > 1 {
		c.Data["HasPrevious"] = true
		c.Data["PreviousPage"] = page - 1
	}
	if commits.Len() == pageSize {
		c.Data["HasNext"] = true
		c.Data["NextPage"] = page + 1
	}
	c.Data["PageSize"] = pageSize

	c.Data["Username"] = c.Repo.Owner.Name
	c.Data["Reponame"] = c.Repo.Repository.Name
	c.HTML(200, COMMITS)
}

func Commits(c *context.Context) {
	renderCommits(c, "")
}

func SearchCommits(c *context.Context) {
	c.Data["PageIsCommits"] = true

	keyword := c.Query("q")
	if len(keyword) == 0 {
		c.Redirect(c.Repo.RepoLink + "/commits/" + c.Repo.BranchName)
		return
	}

	commits, err := c.Repo.Commit.SearchCommits(keyword)
	if err != nil {
		c.Handle(500, "SearchCommits", err)
		return
	}
	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	commits = models.ValidateCommitsWithEmails(commits)
	c.Data["Commits"] = commits

	c.Data["Keyword"] = keyword
	c.Data["Username"] = c.Repo.Owner.Name
	c.Data["Reponame"] = c.Repo.Repository.Name
	c.Data["Branch"] = c.Repo.BranchName
	c.HTML(200, COMMITS)
}

func FileHistory(c *context.Context) {
	renderCommits(c, c.Repo.TreePath)
}

func Diff(c *context.Context) {
	c.Data["PageIsDiff"] = true
	c.Data["RequireHighlightJS"] = true

	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	commitID := c.Params(":sha")

	commit, err := c.Repo.GitRepo.GetCommit(commitID)
	if err != nil {
		if git.IsErrNotExist(err) {
			c.Handle(404, "Repo.GitRepo.GetCommit", err)
		} else {
			c.Handle(500, "Repo.GitRepo.GetCommit", err)
		}
		return
	}

	diff, err := models.GetDiffCommit(models.RepoPath(userName, repoName),
		commitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		c.NotFoundOrServerError("GetDiffCommit", git.IsErrNotExist, err)
		return
	}

	parents := make([]string, commit.ParentCount())
	for i := 0; i < commit.ParentCount(); i++ {
		sha, err := commit.ParentID(i)
		parents[i] = sha.String()
		if err != nil {
			c.Handle(404, "repo.Diff", err)
			return
		}
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.Data["CommitID"] = commitID
	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["Title"] = commit.Summary() + " · " + tool.ShortSHA1(commitID)
	c.Data["Commit"] = commit
	c.Data["Author"] = models.ValidateCommitWithEmail(commit)
	c.Data["Diff"] = diff
	c.Data["Parents"] = parents
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "src", commitID)
	if commit.ParentCount() > 0 {
		c.Data["BeforeSourcePath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "src", parents[0])
	}
	c.Data["RawPath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "raw", commitID)
	c.HTML(200, DIFF)
}

func RawDiff(c *context.Context) {
	if err := git.GetRawDiff(
		models.RepoPath(c.Repo.Owner.Name, c.Repo.Repository.Name),
		c.Params(":sha"),
		git.RawDiffType(c.Params(":ext")),
		c.Resp,
	); err != nil {
		c.NotFoundOrServerError("GetRawDiff", git.IsErrNotExist, err)
		return
	}
}

func CompareDiff(c *context.Context) {
	c.Data["IsDiffCompare"] = true
	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	beforeCommitID := c.Params(":before")
	afterCommitID := c.Params(":after")

	commit, err := c.Repo.GitRepo.GetCommit(afterCommitID)
	if err != nil {
		c.Handle(404, "GetCommit", err)
		return
	}

	diff, err := models.GetDiffRange(models.RepoPath(userName, repoName), beforeCommitID,
		afterCommitID, setting.Git.MaxGitDiffLines,
		setting.Git.MaxGitDiffLineCharacters, setting.Git.MaxGitDiffFiles)
	if err != nil {
		c.Handle(404, "GetDiffRange", err)
		return
	}

	commits, err := commit.CommitsBeforeUntil(beforeCommitID)
	if err != nil {
		c.Handle(500, "CommitsBeforeUntil", err)
		return
	}
	commits = models.ValidateCommitsWithEmails(commits)

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["CommitRepoLink"] = c.Repo.RepoLink
	c.Data["Commits"] = commits
	c.Data["CommitsCount"] = commits.Len()
	c.Data["BeforeCommitID"] = beforeCommitID
	c.Data["AfterCommitID"] = afterCommitID
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["Title"] = "Comparing " + tool.ShortSHA1(beforeCommitID) + "..." + tool.ShortSHA1(afterCommitID) + " · " + userName + "/" + repoName
	c.Data["Commit"] = commit
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "src", afterCommitID)
	c.Data["BeforeSourcePath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "src", beforeCommitID)
	c.Data["RawPath"] = setting.AppSubURL + "/" + path.Join(userName, repoName, "raw", afterCommitID)
	c.HTML(200, DIFF)
}
