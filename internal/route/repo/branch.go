package repo

import (
	"time"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/urlx"
)

const (
	tmplRepoBranchesOverview = "repo/branches/overview"
	tmplRepoBranchesAll      = "repo/branches/all"
)

type Branch struct {
	Name        string
	Commit      *git.Commit
	IsProtected bool
}

func loadBranches(c *context.Context) []*Branch {
	rawBranches, err := c.Repo.Repository.GetBranches(c.Req.Context())
	if err != nil {
		c.Error(err, "get branches")
		return nil
	}

	protectBranches, err := database.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get protect branches by repository ID")
		return nil
	}

	branches := make([]*Branch, len(rawBranches))
	for i := range rawBranches {
		commit, err := rawBranches[i].GetCommit(c.Req.Context())
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
	c.Success(tmplRepoBranchesOverview)
}

func AllBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.git_branches")
	c.Data["PageIsBranchesAll"] = true

	branches := loadBranches(c)
	if c.Written() {
		return
	}
	c.Data["Branches"] = branches

	c.Success(tmplRepoBranchesAll)
}

func DeleteBranchPost(c *context.Context) {
	branchName := c.Params("*")
	commitID := c.Query("commit")

	defer func() {
		redirectTo := c.Query("redirect_to")
		if !urlx.IsSameSite(redirectTo) {
			redirectTo = c.Repo.RepoLink
		}
		c.Redirect(redirectTo)
	}()

	if !c.Repo.GitRepo.HasBranch(c.Req.Context(), branchName) {
		return
	}
	if branchName == c.Repo.Repository.DefaultBranch {
		c.Flash.Error(c.Tr("repo.branches.default_deletion_not_allowed"))
		return
	}

	protectBranch, err := database.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branchName)
	if err != nil && !database.IsErrBranchNotExist(err) {
		c.Logger.Error("Failed to get protected branch", "branch", branchName, "error", err)
		return
	}
	if protectBranch != nil && protectBranch.Protected {
		c.Flash.Error(c.Tr("repo.branches.protected_deletion_not_allowed"))
		return
	}

	if len(commitID) > 0 {
		branchCommitID, err := c.Repo.GitRepo.BranchCommitID(c.Req.Context(), branchName)
		if err != nil {
			c.Logger.Error("Failed to get commit ID of branch", "branch", branchName, "error", err)
			return
		}

		if branchCommitID != commitID {
			c.Flash.Error(c.Tr("repo.pulls.delete_branch_has_new_commits"))
			return
		}
	}

	if err := c.Repo.GitRepo.DeleteBranch(c.Req.Context(), branchName, git.DeleteBranchOptions{
		Force: true,
	}); err != nil {
		c.Logger.Error("Failed to delete branch", "branch", branchName, "error", err)
		return
	}

	if err := database.PrepareWebhooks(c.Logger, c.Repo.Repository, database.HookEventTypeDelete, &apiv1types.WebhookDeletePayload{
		Ref:        branchName,
		RefType:    "branch",
		PusherType: apiv1types.WebhookPusherTypeUser,
		Repo:       c.Repo.Repository.APIFormatLegacy(c.Logger, nil),
		Sender:     c.User.APIFormat(),
	}); err != nil {
		c.Logger.Error("Failed to prepare webhooks", "eventType", database.HookEventTypeDelete, "error", err)
		return
	}
}
