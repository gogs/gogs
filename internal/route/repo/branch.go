// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"time"

	log "unknwon.dev/clog/v2"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/tool"
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
		c.Error(err, "get branches")
		return nil
	}

	protectBranches, err := db.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get protect branches by repository ID")
		return nil
	}

	branches := make([]*Branch, len(rawBranches))
	for i := range rawBranches {
		commit, err := rawBranches[i].GetCommit()
		if err != nil {
			c.Error(err, "get commit")
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
	c.Success(BRANCHES_OVERVIEW)
}

func AllBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.git_branches")
	c.Data["PageIsBranchesAll"] = true

	branches := loadBranches(c)
	if c.Written() {
		return
	}
	c.Data["Branches"] = branches

	c.Success(BRANCHES_ALL)
}

func DeleteBranchPost(c *context.Context) {
	branchName := c.Params("*")
	commitID := c.Query("commit")

	defer func() {
		redirectTo := c.Query("redirect_to")
		if !tool.IsSameSiteURLPath(redirectTo) {
			redirectTo = c.Repo.RepoLink
		}
		c.Redirect(redirectTo)
	}()

	if !c.Repo.GitRepo.HasBranch(branchName) {
		return
	}
	if len(commitID) > 0 {
		branchCommitID, err := c.Repo.GitRepo.BranchCommitID(branchName)
		if err != nil {
			log.Error("Failed to get commit ID of branch %q: %v", branchName, err)
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
		log.Error("Failed to delete branch %q: %v", branchName, err)
		return
	}

	if err := db.PrepareWebhooks(c.Repo.Repository, db.HOOK_EVENT_DELETE, &api.DeletePayload{
		Ref:        branchName,
		RefType:    "branch",
		PusherType: api.PUSHER_TYPE_USER,
		Repo:       c.Repo.Repository.APIFormat(nil),
		Sender:     c.User.APIFormat(),
	}); err != nil {
		log.Error("Failed to prepare webhooks for %q: %v", db.HOOK_EVENT_DELETE, err)
		return
	}
}
