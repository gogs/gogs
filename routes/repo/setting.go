// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"
	"time"

	log "gopkg.in/clog.v1"

	"github.com/gogs/git-module"

	"github.com/gogs/gogs/models"
	"github.com/gogs/gogs/models/errors"
	"github.com/gogs/gogs/pkg/context"
	"github.com/gogs/gogs/pkg/form"
	"github.com/gogs/gogs/pkg/mailer"
	"github.com/gogs/gogs/pkg/setting"
)

const (
	SETTINGS_OPTIONS          = "repo/settings/options"
	SETTINGS_COLLABORATION    = "repo/settings/collaboration"
	SETTINGS_BRANCHES         = "repo/settings/branches"
	SETTINGS_PROTECTED_BRANCH = "repo/settings/protected_branch"
	SETTINGS_GITHOOKS         = "repo/settings/githooks"
	SETTINGS_GITHOOK_EDIT     = "repo/settings/githook_edit"
	SETTINGS_DEPLOY_KEYS      = "repo/settings/deploy_keys"
)

func Settings(c *context.Context) {
	c.Title("repo.settings")
	c.PageIs("SettingsOptions")
	c.Success(SETTINGS_OPTIONS)
}

func SettingsPost(c *context.Context, f form.RepoSetting) {
	c.Title("repo.settings")
	c.PageIs("SettingsOptions")

	repo := c.Repo.Repository

	switch c.Query("action") {
	case "update":
		if c.HasError() {
			c.Success(SETTINGS_OPTIONS)
			return
		}

		isNameChanged := false
		oldRepoName := repo.Name
		newRepoName := f.RepoName
		// Check if repository name has been changed.
		if repo.LowerName != strings.ToLower(newRepoName) {
			isNameChanged = true
			if err := models.ChangeRepositoryName(c.Repo.Owner, repo.Name, newRepoName); err != nil {
				c.FormErr("RepoName")
				switch {
				case models.IsErrRepoAlreadyExist(err):
					c.RenderWithErr(c.Tr("form.repo_name_been_taken"), SETTINGS_OPTIONS, &f)
				case models.IsErrNameReserved(err):
					c.RenderWithErr(c.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), SETTINGS_OPTIONS, &f)
				case models.IsErrNamePatternNotAllowed(err):
					c.RenderWithErr(c.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), SETTINGS_OPTIONS, &f)
				default:
					c.ServerError("ChangeRepositoryName", err)
				}
				return
			}

			log.Trace("Repository name changed: %s/%s -> %s", c.Repo.Owner.Name, repo.Name, newRepoName)
		}
		// In case it's just a case change.
		repo.Name = newRepoName
		repo.LowerName = strings.ToLower(newRepoName)

		repo.Description = f.Description
		repo.Website = f.Website

		// Visibility of forked repository is forced sync with base repository.
		if repo.IsFork {
			f.Private = repo.BaseRepo.IsPrivate
		}

		visibilityChanged := repo.IsPrivate != f.Private
		repo.IsPrivate = f.Private
		if err := models.UpdateRepository(repo, visibilityChanged); err != nil {
			c.ServerError("UpdateRepository", err)
			return
		}
		log.Trace("Repository basic settings updated: %s/%s", c.Repo.Owner.Name, repo.Name)

		if isNameChanged {
			if err := models.RenameRepoAction(c.User, oldRepoName, repo); err != nil {
				log.Error(2, "RenameRepoAction: %v", err)
			}
		}

		c.Flash.Success(c.Tr("repo.settings.update_settings_success"))
		c.Redirect(repo.Link() + "/settings")

	case "mirror":
		if !repo.IsMirror {
			c.NotFound()
			return
		}

		if f.Interval > 0 {
			c.Repo.Mirror.EnablePrune = f.EnablePrune
			c.Repo.Mirror.Interval = f.Interval
			c.Repo.Mirror.NextSync = time.Now().Add(time.Duration(f.Interval) * time.Hour)
			if err := models.UpdateMirror(c.Repo.Mirror); err != nil {
				c.ServerError("UpdateMirror", err)
				return
			}
		}
		if err := c.Repo.Mirror.SaveAddress(f.MirrorAddress); err != nil {
			c.ServerError("SaveAddress", err)
			return
		}

		c.Flash.Success(c.Tr("repo.settings.update_settings_success"))
		c.Redirect(repo.Link() + "/settings")

	case "mirror-sync":
		if !repo.IsMirror {
			c.NotFound()
			return
		}

		go models.MirrorQueue.Add(repo.ID)
		c.Flash.Info(c.Tr("repo.settings.mirror_sync_in_progress"))
		c.Redirect(repo.Link() + "/settings")

	case "advanced":
		repo.EnableWiki = f.EnableWiki
		repo.AllowPublicWiki = f.AllowPublicWiki
		repo.EnableExternalWiki = f.EnableExternalWiki
		repo.ExternalWikiURL = f.ExternalWikiURL
		repo.EnableIssues = f.EnableIssues
		repo.AllowPublicIssues = f.AllowPublicIssues
		repo.EnableExternalTracker = f.EnableExternalTracker
		repo.ExternalTrackerURL = f.ExternalTrackerURL
		repo.ExternalTrackerFormat = f.TrackerURLFormat
		repo.ExternalTrackerStyle = f.TrackerIssueStyle
		repo.EnablePulls = f.EnablePulls
		repo.PullsIgnoreWhitespace = f.PullsIgnoreWhitespace
		repo.PullsAllowRebase = f.PullsAllowRebase

		if err := models.UpdateRepository(repo, false); err != nil {
			c.ServerError("UpdateRepository", err)
			return
		}
		log.Trace("Repository advanced settings updated: %s/%s", c.Repo.Owner.Name, repo.Name)

		c.Flash.Success(c.Tr("repo.settings.update_settings_success"))
		c.Redirect(c.Repo.RepoLink + "/settings")

	case "convert":
		if !c.Repo.IsOwner() {
			c.NotFound()
			return
		}
		if repo.Name != f.RepoName {
			c.RenderWithErr(c.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if c.Repo.Owner.IsOrganization() {
			if !c.Repo.Owner.IsOwnedBy(c.User.ID) {
				c.NotFound()
				return
			}
		}

		if !repo.IsMirror {
			c.NotFound()
			return
		}
		repo.IsMirror = false

		if _, err := models.CleanUpMigrateInfo(repo); err != nil {
			c.ServerError("CleanUpMigrateInfo", err)
			return
		} else if err = models.DeleteMirrorByRepoID(c.Repo.Repository.ID); err != nil {
			c.ServerError("DeleteMirrorByRepoID", err)
			return
		}
		log.Trace("Repository converted from mirror to regular: %s/%s", c.Repo.Owner.Name, repo.Name)
		c.Flash.Success(c.Tr("repo.settings.convert_succeed"))
		c.Redirect(setting.AppSubURL + "/" + c.Repo.Owner.Name + "/" + repo.Name)

	case "transfer":
		if !c.Repo.IsOwner() {
			c.NotFound()
			return
		}
		if repo.Name != f.RepoName {
			c.RenderWithErr(c.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if c.Repo.Owner.IsOrganization() && !c.User.IsAdmin {
			if !c.Repo.Owner.IsOwnedBy(c.User.ID) {
				c.NotFound()
				return
			}
		}

		newOwner := c.Query("new_owner_name")
		isExist, err := models.IsUserExist(0, newOwner)
		if err != nil {
			c.ServerError("IsUserExist", err)
			return
		} else if !isExist {
			c.RenderWithErr(c.Tr("form.enterred_invalid_owner_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if err = models.TransferOwnership(c.User, newOwner, repo); err != nil {
			if models.IsErrRepoAlreadyExist(err) {
				c.RenderWithErr(c.Tr("repo.settings.new_owner_has_same_repo"), SETTINGS_OPTIONS, nil)
			} else {
				c.ServerError("TransferOwnership", err)
			}
			return
		}
		log.Trace("Repository transfered: %s/%s -> %s", c.Repo.Owner.Name, repo.Name, newOwner)
		c.Flash.Success(c.Tr("repo.settings.transfer_succeed"))
		c.Redirect(setting.AppSubURL + "/" + newOwner + "/" + repo.Name)

	case "delete":
		if !c.Repo.IsOwner() {
			c.NotFound()
			return
		}
		if repo.Name != f.RepoName {
			c.RenderWithErr(c.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if c.Repo.Owner.IsOrganization() && !c.User.IsAdmin {
			if !c.Repo.Owner.IsOwnedBy(c.User.ID) {
				c.NotFound()
				return
			}
		}

		if err := models.DeleteRepository(c.Repo.Owner.ID, repo.ID); err != nil {
			c.ServerError("DeleteRepository", err)
			return
		}
		log.Trace("Repository deleted: %s/%s", c.Repo.Owner.Name, repo.Name)

		c.Flash.Success(c.Tr("repo.settings.deletion_success"))
		c.Redirect(c.Repo.Owner.DashboardLink())

	case "delete-wiki":
		if !c.Repo.IsOwner() {
			c.NotFound()
			return
		}
		if repo.Name != f.RepoName {
			c.RenderWithErr(c.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if c.Repo.Owner.IsOrganization() && !c.User.IsAdmin {
			if !c.Repo.Owner.IsOwnedBy(c.User.ID) {
				c.NotFound()
				return
			}
		}

		repo.DeleteWiki()
		log.Trace("Repository wiki deleted: %s/%s", c.Repo.Owner.Name, repo.Name)

		repo.EnableWiki = false
		if err := models.UpdateRepository(repo, false); err != nil {
			c.ServerError("UpdateRepository", err)
			return
		}

		c.Flash.Success(c.Tr("repo.settings.wiki_deletion_success"))
		c.Redirect(c.Repo.RepoLink + "/settings")

	default:
		c.NotFound()
	}
}

func SettingsCollaboration(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsCollaboration"] = true

	users, err := c.Repo.Repository.GetCollaborators()
	if err != nil {
		c.Handle(500, "GetCollaborators", err)
		return
	}
	c.Data["Collaborators"] = users

	c.HTML(200, SETTINGS_COLLABORATION)
}

func SettingsCollaborationPost(c *context.Context) {
	name := strings.ToLower(c.Query("collaborator"))
	if len(name) == 0 || c.Repo.Owner.LowerName == name {
		c.Redirect(setting.AppSubURL + c.Req.URL.Path)
		return
	}

	u, err := models.GetUserByName(name)
	if err != nil {
		if errors.IsUserNotExist(err) {
			c.Flash.Error(c.Tr("form.user_not_exist"))
			c.Redirect(setting.AppSubURL + c.Req.URL.Path)
		} else {
			c.Handle(500, "GetUserByName", err)
		}
		return
	}

	// Organization is not allowed to be added as a collaborator
	if u.IsOrganization() {
		c.Flash.Error(c.Tr("repo.settings.org_not_allowed_to_be_collaborator"))
		c.Redirect(setting.AppSubURL + c.Req.URL.Path)
		return
	}

	if err = c.Repo.Repository.AddCollaborator(u); err != nil {
		c.Handle(500, "AddCollaborator", err)
		return
	}

	if setting.Service.EnableNotifyMail {
		mailer.SendCollaboratorMail(models.NewMailerUser(u), models.NewMailerUser(c.User), models.NewMailerRepo(c.Repo.Repository))
	}

	c.Flash.Success(c.Tr("repo.settings.add_collaborator_success"))
	c.Redirect(setting.AppSubURL + c.Req.URL.Path)
}

func ChangeCollaborationAccessMode(c *context.Context) {
	if err := c.Repo.Repository.ChangeCollaborationAccessMode(
		c.QueryInt64("uid"),
		models.AccessMode(c.QueryInt("mode"))); err != nil {
		log.Error(2, "ChangeCollaborationAccessMode: %v", err)
		return
	}

	c.Status(204)
}

func DeleteCollaboration(c *context.Context) {
	if err := c.Repo.Repository.DeleteCollaboration(c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteCollaboration: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.remove_collaborator_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/collaboration",
	})
}

func SettingsBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.branches")
	c.Data["PageIsSettingsBranches"] = true

	if c.Repo.Repository.IsBare {
		c.Flash.Info(c.Tr("repo.settings.branches_bare"), true)
		c.HTML(200, SETTINGS_BRANCHES)
		return
	}

	protectBranches, err := models.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "GetProtectBranchesByRepoID", err)
		return
	}

	// Filter out deleted branches
	branches := make([]string, 0, len(protectBranches))
	for i := range protectBranches {
		if c.Repo.GitRepo.IsBranchExist(protectBranches[i].Name) {
			branches = append(branches, protectBranches[i].Name)
		}
	}
	c.Data["ProtectBranches"] = branches

	c.HTML(200, SETTINGS_BRANCHES)
}

func UpdateDefaultBranch(c *context.Context) {
	branch := c.Query("branch")
	if c.Repo.GitRepo.IsBranchExist(branch) &&
		c.Repo.Repository.DefaultBranch != branch {
		c.Repo.Repository.DefaultBranch = branch
		if err := c.Repo.GitRepo.SetDefaultBranch(branch); err != nil {
			if !git.IsErrUnsupportedVersion(err) {
				c.Handle(500, "SetDefaultBranch", err)
				return
			}

			c.Flash.Warning(c.Tr("repo.settings.update_default_branch_unsupported"))
			c.Redirect(c.Repo.RepoLink + "/settings/branches")
			return
		}
	}

	if err := models.UpdateRepository(c.Repo.Repository, false); err != nil {
		c.Handle(500, "UpdateRepository", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_default_branch_success"))
	c.Redirect(c.Repo.RepoLink + "/settings/branches")
}

func SettingsProtectedBranch(c *context.Context) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.IsBranchExist(branch) {
		c.NotFound()
		return
	}

	c.Data["Title"] = c.Tr("repo.settings.protected_branches") + " - " + branch
	c.Data["PageIsSettingsBranches"] = true

	protectBranch, err := models.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !errors.IsErrBranchNotExist(err) {
			c.Handle(500, "GetProtectBranchOfRepoByName", err)
			return
		}

		// No options found, create defaults.
		protectBranch = &models.ProtectBranch{
			Name: branch,
		}
	}

	if c.Repo.Owner.IsOrganization() {
		users, err := c.Repo.Repository.GetWriters()
		if err != nil {
			c.Handle(500, "Repo.Repository.GetPushers", err)
			return
		}
		c.Data["Users"] = users
		c.Data["whitelist_users"] = protectBranch.WhitelistUserIDs

		teams, err := c.Repo.Owner.TeamsHaveAccessToRepo(c.Repo.Repository.ID, models.ACCESS_MODE_WRITE)
		if err != nil {
			c.Handle(500, "Repo.Owner.TeamsHaveAccessToRepo", err)
			return
		}
		c.Data["Teams"] = teams
		c.Data["whitelist_teams"] = protectBranch.WhitelistTeamIDs
	}

	c.Data["Branch"] = protectBranch
	c.HTML(200, SETTINGS_PROTECTED_BRANCH)
}

func SettingsProtectedBranchPost(c *context.Context, f form.ProtectBranch) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.IsBranchExist(branch) {
		c.NotFound()
		return
	}

	protectBranch, err := models.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !errors.IsErrBranchNotExist(err) {
			c.Handle(500, "GetProtectBranchOfRepoByName", err)
			return
		}

		// No options found, create defaults.
		protectBranch = &models.ProtectBranch{
			RepoID: c.Repo.Repository.ID,
			Name:   branch,
		}
	}

	protectBranch.Protected = f.Protected
	protectBranch.RequirePullRequest = f.RequirePullRequest
	protectBranch.EnableWhitelist = f.EnableWhitelist
	if c.Repo.Owner.IsOrganization() {
		err = models.UpdateOrgProtectBranch(c.Repo.Repository, protectBranch, f.WhitelistUsers, f.WhitelistTeams)
	} else {
		err = models.UpdateProtectBranch(protectBranch)
	}
	if err != nil {
		c.Handle(500, "UpdateOrgProtectBranch/UpdateProtectBranch", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_protect_branch_success"))
	c.Redirect(fmt.Sprintf("%s/settings/branches/%s", c.Repo.RepoLink, branch))
}

func SettingsGitHooks(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.githooks")
	c.Data["PageIsSettingsGitHooks"] = true

	hooks, err := c.Repo.GitRepo.Hooks()
	if err != nil {
		c.Handle(500, "Hooks", err)
		return
	}
	c.Data["Hooks"] = hooks

	c.HTML(200, SETTINGS_GITHOOKS)
}

func SettingsGitHooksEdit(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.githooks")
	c.Data["PageIsSettingsGitHooks"] = true
	c.Data["RequireSimpleMDE"] = true

	name := c.Params(":name")
	hook, err := c.Repo.GitRepo.GetHook(name)
	if err != nil {
		if err == git.ErrNotValidHook {
			c.Handle(404, "GetHook", err)
		} else {
			c.Handle(500, "GetHook", err)
		}
		return
	}
	c.Data["Hook"] = hook
	c.HTML(200, SETTINGS_GITHOOK_EDIT)
}

func SettingsGitHooksEditPost(c *context.Context) {
	name := c.Params(":name")
	hook, err := c.Repo.GitRepo.GetHook(name)
	if err != nil {
		if err == git.ErrNotValidHook {
			c.Handle(404, "GetHook", err)
		} else {
			c.Handle(500, "GetHook", err)
		}
		return
	}
	hook.Content = c.Query("content")
	if err = hook.Update(); err != nil {
		c.Handle(500, "hook.Update", err)
		return
	}
	c.Redirect(c.Data["Link"].(string))
}

func SettingsDeployKeys(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.deploy_keys")
	c.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "ListDeployKeys", err)
		return
	}
	c.Data["Deploykeys"] = keys

	c.HTML(200, SETTINGS_DEPLOY_KEYS)
}

func SettingsDeployKeysPost(c *context.Context, f form.AddSSHKey) {
	c.Data["Title"] = c.Tr("repo.settings.deploy_keys")
	c.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "ListDeployKeys", err)
		return
	}
	c.Data["Deploykeys"] = keys

	if c.HasError() {
		c.HTML(200, SETTINGS_DEPLOY_KEYS)
		return
	}

	content, err := models.CheckPublicKeyString(f.Content)
	if err != nil {
		if models.IsErrKeyUnableVerify(err) {
			c.Flash.Info(c.Tr("form.unable_verify_ssh_key"))
		} else {
			c.Data["HasError"] = true
			c.Data["Err_Content"] = true
			c.Flash.Error(c.Tr("form.invalid_ssh_key", err.Error()))
			c.Redirect(c.Repo.RepoLink + "/settings/keys")
			return
		}
	}

	key, err := models.AddDeployKey(c.Repo.Repository.ID, f.Title, content)
	if err != nil {
		c.Data["HasError"] = true
		switch {
		case models.IsErrKeyAlreadyExist(err):
			c.Data["Err_Content"] = true
			c.RenderWithErr(c.Tr("repo.settings.key_been_used"), SETTINGS_DEPLOY_KEYS, &f)
		case models.IsErrKeyNameAlreadyUsed(err):
			c.Data["Err_Title"] = true
			c.RenderWithErr(c.Tr("repo.settings.key_name_used"), SETTINGS_DEPLOY_KEYS, &f)
		default:
			c.Handle(500, "AddDeployKey", err)
		}
		return
	}

	log.Trace("Deploy key added: %d", c.Repo.Repository.ID)
	c.Flash.Success(c.Tr("repo.settings.add_key_success", key.Name))
	c.Redirect(c.Repo.RepoLink + "/settings/keys")
}

func DeleteDeployKey(c *context.Context) {
	if err := models.DeleteDeployKey(c.User, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteDeployKey: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.deploy_key_deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/keys",
	})
}
