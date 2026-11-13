package repo

import (
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/urlx"
)

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
		log.Error("Failed to get protected branch %q: %v", branchName, err)
		return
	}
	if protectBranch != nil && protectBranch.Protected {
		c.Flash.Error(c.Tr("repo.branches.protected_deletion_not_allowed"))
		return
	}

	if len(commitID) > 0 {
		branchCommitID, err := c.Repo.GitRepo.BranchCommitID(c.Req.Context(), branchName)
		if err != nil {
			log.Error("Failed to get commit ID of branch %q: %v", branchName, err)
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
		log.Error("Failed to delete branch %q: %v", branchName, err)
		return
	}

	if err := database.PrepareWebhooks(c.Repo.Repository, database.HookEventTypeDelete, &apiv1types.WebhookDeletePayload{
		Ref:        branchName,
		RefType:    "branch",
		PusherType: apiv1types.WebhookPusherTypeUser,
		Repo:       c.Repo.Repository.APIFormatLegacy(nil),
		Sender:     c.User.APIFormat(),
	}); err != nil {
		log.Error("Failed to prepare webhooks for %q: %v", database.HookEventTypeDelete, err)
		return
	}
}
