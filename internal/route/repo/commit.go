// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"path"

	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/gitutil"
	"gogs.io/gogs/internal/tool"
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

// TODO(unknwon)
func RenderIssueLinks(oldCommits []*git.Commit, repoLink string) []*git.Commit {
	return oldCommits
}

func renderCommits(c *context.Context, filename string) {
	c.Data["Title"] = c.Tr("repo.commits.commit_history") + " · " + c.Repo.Repository.FullName()
	c.Data["PageIsCommits"] = true
	c.Data["FileName"] = filename

	page := c.QueryInt("page")
	if page < 1 {
		page = 1
	}
	pageSize := c.QueryInt("pageSize")
	if pageSize < 1 {
		pageSize = conf.UI.User.CommitsPagingNum
	}

	commits, err := c.Repo.Commit.CommitsByPage(page, pageSize, git.CommitsByPageOptions{Path: filename})
	if err != nil {
		c.ServerError("paging commits", err)
		return
	}

	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	c.Data["Commits"] = db.ValidateCommitsWithEmails(commits)

	if page > 1 {
		c.Data["HasPrevious"] = true
		c.Data["PreviousPage"] = page - 1
	}
	if len(commits) == pageSize {
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
		c.ServerError("SearchCommits", err)
		return
	}

	commits = RenderIssueLinks(commits, c.Repo.RepoLink)
	c.Data["Commits"] = db.ValidateCommitsWithEmails(commits)

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
	c.PageIs("Diff")
	c.RequireHighlightJS()

	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	commitID := c.Params(":sha")

	commit, err := c.Repo.GitRepo.CatFileCommit(commitID)
	if err != nil {
		c.NotFoundOrServerError("get commit by ID", gitutil.IsErrRevisionNotExist, err)
		return
	}

	diff, err := gitutil.RepoDiff(c.Repo.GitRepo,
		commitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
	)
	if err != nil {
		c.NotFoundOrServerError("get diff", gitutil.IsErrRevisionNotExist, err)
		return
	}

	parents := make([]string, commit.ParentsCount())
	for i := 0; i < commit.ParentsCount(); i++ {
		sha, err := commit.ParentID(i)
		if err != nil {
			c.NotFound()
			return
		}
		parents[i] = sha.String()
	}

	setEditorconfigIfExists(c)
	if c.Written() {
		return
	}

	c.RawTitle(commit.Summary() + " · " + tool.ShortSHA1(commitID))
	c.Data["CommitID"] = commitID
	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["Commit"] = commit
	c.Data["Author"] = db.ValidateCommitWithEmail(commit)
	c.Data["Diff"] = diff
	c.Data["Parents"] = parents
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", commitID)
	if commit.ParentsCount() > 0 {
		c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", parents[0])
	}
	c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", commitID)
	c.Success(DIFF)
}

func RawDiff(c *context.Context) {
	if err := c.Repo.GitRepo.RawDiff(
		c.Params(":sha"),
		git.RawDiffFormat(c.Params(":ext")),
		c.Resp,
	); err != nil {
		c.NotFoundOrServerError("get raw diff", gitutil.IsErrRevisionNotExist, err)
		return
	}
}

func CompareDiff(c *context.Context) {
	c.Data["IsDiffCompare"] = true
	userName := c.Repo.Owner.Name
	repoName := c.Repo.Repository.Name
	beforeCommitID := c.Params(":before")
	afterCommitID := c.Params(":after")

	commit, err := c.Repo.GitRepo.CatFileCommit(afterCommitID)
	if err != nil {
		c.Handle(404, "GetCommit", err)
		return
	}

	diff, err := gitutil.RepoDiff(c.Repo.GitRepo,
		afterCommitID, conf.Git.MaxDiffFiles, conf.Git.MaxDiffLines, conf.Git.MaxDiffLineChars,
		git.DiffOptions{Base: beforeCommitID},
	)
	if err != nil {
		c.ServerError("get diff", err)
		return
	}

	commits, err := commit.CommitsAfter(beforeCommitID)
	if err != nil {
		c.ServerError("get commits after", err)
		return
	}

	c.Data["IsSplitStyle"] = c.Query("style") == "split"
	c.Data["CommitRepoLink"] = c.Repo.RepoLink
	c.Data["Commits"] = db.ValidateCommitsWithEmails(commits)
	c.Data["CommitsCount"] = len(commits)
	c.Data["BeforeCommitID"] = beforeCommitID
	c.Data["AfterCommitID"] = afterCommitID
	c.Data["Username"] = userName
	c.Data["Reponame"] = repoName
	c.Data["IsImageFile"] = commit.IsImageFile
	c.Data["Title"] = "Comparing " + tool.ShortSHA1(beforeCommitID) + "..." + tool.ShortSHA1(afterCommitID) + " · " + userName + "/" + repoName
	c.Data["Commit"] = commit
	c.Data["Diff"] = diff
	c.Data["DiffNotAvailable"] = diff.NumFiles() == 0
	c.Data["SourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", afterCommitID)
	c.Data["BeforeSourcePath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "src", beforeCommitID)
	c.Data["RawPath"] = conf.Server.Subpath + "/" + path.Join(userName, repoName, "raw", afterCommitID)
	c.HTML(200, DIFF)
}
