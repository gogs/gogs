package repo

import (
	"slices"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

const (
	tmplRepoSettingsGithooks    = "repo/settings/githooks"
	tmplRepoSettingsGithookEdit = "repo/settings/githook_edit"
)

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
