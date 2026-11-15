package repo

import (
	"fmt"
	"net/http"
	"slices"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

const (
	tmplRepoSettingsBranches        = "repo/settings/branches"
	tmplRepoSettingsProtectedBranch = "repo/settings/protected_branch"
	tmplRepoSettingsGithooks        = "repo/settings/githooks"
	tmplRepoSettingsGithookEdit     = "repo/settings/githook_edit"
)

func SettingsBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.branches")
	c.Data["PageIsSettingsBranches"] = true

	if c.Repo.Repository.IsEmpty {
		c.Flash.Info(c.Tr("repo.settings.branches_empty"), true)
		c.HTML(http.StatusUnprocessableEntity, tmplRepoSettingsBranches)
		return
	}

	protectBranches, err := database.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get protect branch by repository ID")
		return
	}

	// Filter out deleted branches
	branches := make([]string, 0, len(protectBranches))
	for i := range protectBranches {
		if c.Repo.GitRepo.HasBranch(c.Req.Context(), protectBranches[i].Name) {
			branches = append(branches, protectBranches[i].Name)
		}
	}
	c.Data["ProtectBranches"] = branches

	c.Success(tmplRepoSettingsBranches)
}

func UpdateDefaultBranch(c *context.Context) {
	branch := c.Query("branch")
	if c.Repo.GitRepo.HasBranch(c.Req.Context(), branch) &&
		c.Repo.Repository.DefaultBranch != branch {
		c.Repo.Repository.DefaultBranch = branch
		if _, err := c.Repo.GitRepo.SymbolicRef(c.Req.Context(), git.SymbolicRefOptions{
			Ref: git.RefsHeads + branch,
		}); err != nil {
			c.Flash.Warning(c.Tr("repo.settings.update_default_branch_unsupported"))
			c.Redirect(c.Repo.RepoLink + "/settings/branches")
			return
		}
	}

	if err := database.UpdateRepository(c.Repo.Repository, false); err != nil {
		c.Error(err, "update repository")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_default_branch_success"))
	c.Redirect(c.Repo.RepoLink + "/settings/branches")
}

func SettingsProtectedBranch(c *context.Context) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.HasBranch(c.Req.Context(), branch) {
		c.NotFound()
		return
	}

	c.Data["Title"] = c.Tr("repo.settings.protected_branches") + " - " + branch
	c.Data["PageIsSettingsBranches"] = true

	protectBranch, err := database.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !database.IsErrBranchNotExist(err) {
			c.Error(err, "get protect branch of repository by name")
			return
		}

		// No options found, create defaults.
		protectBranch = &database.ProtectBranch{
			Name: branch,
		}
	}

	if c.Repo.Owner.IsOrganization() {
		users, err := c.Repo.Repository.GetWriters()
		if err != nil {
			c.Error(err, "get writers")
			return
		}
		c.Data["Users"] = users
		c.Data["whitelist_users"] = protectBranch.WhitelistUserIDs

		teams, err := c.Repo.Owner.TeamsHaveAccessToRepo(c.Repo.Repository.ID, database.AccessModeWrite)
		if err != nil {
			c.Error(err, "get teams have access to the repository")
			return
		}
		c.Data["Teams"] = teams
		c.Data["whitelist_teams"] = protectBranch.WhitelistTeamIDs
	}

	c.Data["Branch"] = protectBranch
	c.Success(tmplRepoSettingsProtectedBranch)
}

func SettingsProtectedBranchPost(c *context.Context, f form.ProtectBranch) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.HasBranch(c.Req.Context(), branch) {
		c.NotFound()
		return
	}

	protectBranch, err := database.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !database.IsErrBranchNotExist(err) {
			c.Error(err, "get protect branch of repository by name")
			return
		}

		// No options found, create defaults.
		protectBranch = &database.ProtectBranch{
			RepoID: c.Repo.Repository.ID,
			Name:   branch,
		}
	}

	protectBranch.Protected = f.Protected
	protectBranch.RequirePullRequest = f.RequirePullRequest
	protectBranch.EnableWhitelist = f.EnableWhitelist
	if c.Repo.Owner.IsOrganization() {
		err = database.UpdateOrgProtectBranch(c.Repo.Repository, protectBranch, f.WhitelistUsers, f.WhitelistTeams)
	} else {
		err = database.UpdateProtectBranch(protectBranch)
	}
	if err != nil {
		c.Error(err, "update protect branch")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_protect_branch_success"))
	c.Redirect(fmt.Sprintf("%s/settings/branches/%s", c.Repo.RepoLink, branch))
}

func SettingsGitHooks(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.githooks")
	c.Data["PageIsSettingsGitHooks"] = true

	hooks, err := c.Repo.GitRepo.Hooks("custom_hooks")
	if err != nil {
		c.Error(err, "get hooks")
		return
	}
	c.Data["Hooks"] = hooks

	c.Success(tmplRepoSettingsGithooks)
}

func isValidHookName(name git.HookName) bool {
	return slices.Contains(git.ServerSideHooks, name)
}

func SettingsGitHooksEdit(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.githooks")
	c.Data["PageIsSettingsGitHooks"] = true
	c.Data["RequireSimpleMDE"] = true

	name := git.HookName(c.Params(":name"))
	if !isValidHookName(name) {
		c.NotFound()
		return
	}

	hook, err := c.Repo.GitRepo.Hook("custom_hooks", name)
	if err != nil {
		c.NotFoundOrError(osx.NewError(err), "get hook")
		return
	}
	c.Data["Hook"] = hook
	c.Success(tmplRepoSettingsGithookEdit)
}

func SettingsGitHooksEditPost(c *context.Context) {
	name := git.HookName(c.Params(":name"))
	if !isValidHookName(name) {
		c.NotFound()
		return
	}

	hook, err := c.Repo.GitRepo.Hook("custom_hooks", name)
	if err != nil {
		c.NotFoundOrError(osx.NewError(err), "get hook")
		return
	}
	if err = hook.Update(c.Query("content")); err != nil {
		c.Error(err, "update hook")
		return
	}
	c.Redirect(c.Data["Link"].(string))
}
