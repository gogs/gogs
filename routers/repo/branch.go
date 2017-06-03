// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"time"

	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/pkg/context"
)

const (
	BRANCHES_OVERVIEW = "repo/branches/overview"
	BRANCHES_ALL      = "repo/branches/all"
)

type Branch struct {
	Name        string
	Commit      *git.Commit
	IsProtected bool
}

func loadBranches(c *context.Context) []*Branch {
	rawBranches, err := c.Repo.Repository.GetBranches()
	if err != nil {
		c.Handle(500, "GetBranches", err)
		return nil
	}

	protectBranches, err := models.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "GetProtectBranchesByRepoID", err)
		return nil
	}

	branches := make([]*Branch, len(rawBranches))
	for i := range rawBranches {
		commit, err := rawBranches[i].GetCommit()
		if err != nil {
			c.Handle(500, "GetCommit", err)
			return nil
		}

		branches[i] = &Branch{
			Name:   rawBranches[i].Name,
			Commit: commit,
		}

		for j := range protectBranches {
			if branches[i].Name == protectBranches[j].Name {
				branches[i].IsProtected = true
				break
			}
		}
	}

	c.Data["AllowPullRequest"] = c.Repo.Repository.AllowsPulls()
	return branches
}

func Branches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.git_branches")
	c.Data["PageIsBranchesOverview"] = true

	branches := loadBranches(c)
	if c.Written() {
		return
	}

	now := time.Now()
	activeBranches := make([]*Branch, 0, 3)
	staleBranches := make([]*Branch, 0, 3)
	for i := range branches {
		switch {
		case branches[i].Name == c.Repo.BranchName:
			c.Data["DefaultBranch"] = branches[i]
		case branches[i].Commit.Committer.When.Add(30 * 24 * time.Hour).After(now): // 30 days
			activeBranches = append(activeBranches, branches[i])
		case branches[i].Commit.Committer.When.Add(3 * 30 * 24 * time.Hour).Before(now): // 90 days
			staleBranches = append(staleBranches, branches[i])
		}
	}

	c.Data["ActiveBranches"] = activeBranches
	c.Data["StaleBranches"] = staleBranches
	c.HTML(200, BRANCHES_OVERVIEW)
}

func AllBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.git_branches")
	c.Data["PageIsBranchesAll"] = true

	branches := loadBranches(c)
	if c.Written() {
		return
	}
	c.Data["Branches"] = branches

	c.HTML(200, BRANCHES_ALL)
}

func DeleteBranchPost(c *context.Context) {
	branchName := c.Params("*")
	commitID := c.Query("commit")

	defer func() {
		redirectTo := c.Query("redirect_to")
		if len(redirectTo) == 0 {
			redirectTo = c.Repo.RepoLink
		}
		c.Redirect(redirectTo)
	}()

	if !c.Repo.GitRepo.IsBranchExist(branchName) {
		return
	}
	if len(commitID) > 0 {
		branchCommitID, err := c.Repo.GitRepo.GetBranchCommitID(branchName)
		if err != nil {
			log.Error(2, "GetBranchCommitID: %v", err)
			return
		}

		if branchCommitID != commitID {
			c.Flash.Error(c.Tr("repo.pulls.delete_branch_has_new_commits"))
			return
		}
	}

	if err := c.Repo.GitRepo.DeleteBranch(branchName, git.DeleteBranchOptions{
		Force: true,
	}); err != nil {
		log.Error(2, "DeleteBranch '%s': %v", branchName, err)
		return
	}
}
