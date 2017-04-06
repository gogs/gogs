// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"
	"time"

	log "gopkg.in/clog.v1"

	"github.com/gogits/git-module"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/mailer"
	"github.com/gogits/gogs/pkg/setting"
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

func Settings(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.HTML(200, SETTINGS_OPTIONS)
}

func SettingsPost(ctx *context.Context, f form.RepoSetting) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsOptions"] = true

	repo := ctx.Repo.Repository

	switch ctx.Query("action") {
	case "update":
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_OPTIONS)
			return
		}

		isNameChanged := false
		oldRepoName := repo.Name
		newRepoName := f.RepoName
		// Check if repository name has been changed.
		if repo.LowerName != strings.ToLower(newRepoName) {
			isNameChanged = true
			if err := models.ChangeRepositoryName(ctx.Repo.Owner, repo.Name, newRepoName); err != nil {
				ctx.Data["Err_RepoName"] = true
				switch {
				case models.IsErrRepoAlreadyExist(err):
					ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), SETTINGS_OPTIONS, &f)
				case models.IsErrNameReserved(err):
					ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), SETTINGS_OPTIONS, &f)
				case models.IsErrNamePatternNotAllowed(err):
					ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), SETTINGS_OPTIONS, &f)
				default:
					ctx.Handle(500, "ChangeRepositoryName", err)
				}
				return
			}

			log.Trace("Repository name changed: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newRepoName)
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
			ctx.Handle(500, "UpdateRepository", err)
			return
		}
		log.Trace("Repository basic settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)

		if isNameChanged {
			if err := models.RenameRepoAction(ctx.User, oldRepoName, repo); err != nil {
				log.Error(4, "RenameRepoAction: %v", err)
			}
		}

		ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
		ctx.Redirect(repo.Link() + "/settings")

	case "mirror":
		if !repo.IsMirror {
			ctx.Handle(404, "", nil)
			return
		}

		if f.Interval > 0 {
			ctx.Repo.Mirror.EnablePrune = f.EnablePrune
			ctx.Repo.Mirror.Interval = f.Interval
			ctx.Repo.Mirror.NextUpdate = time.Now().Add(time.Duration(f.Interval) * time.Hour)
			if err := models.UpdateMirror(ctx.Repo.Mirror); err != nil {
				ctx.Handle(500, "UpdateMirror", err)
				return
			}
		}
		if err := ctx.Repo.Mirror.SaveAddress(f.MirrorAddress); err != nil {
			ctx.Handle(500, "SaveAddress", err)
			return
		}

		ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
		ctx.Redirect(repo.Link() + "/settings")

	case "mirror-sync":
		if !repo.IsMirror {
			ctx.Handle(404, "", nil)
			return
		}

		go models.MirrorQueue.Add(repo.ID)
		ctx.Flash.Info(ctx.Tr("repo.settings.mirror_sync_in_progress"))
		ctx.Redirect(repo.Link() + "/settings")

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

		if err := models.UpdateRepository(repo, false); err != nil {
			ctx.Handle(500, "UpdateRepository", err)
			return
		}
		log.Trace("Repository advanced settings updated: %s/%s", ctx.Repo.Owner.Name, repo.Name)

		ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")

	case "convert":
		if !ctx.Repo.IsOwner() {
			ctx.Error(404)
			return
		}
		if repo.Name != f.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() {
			if !ctx.Repo.Owner.IsOwnedBy(ctx.User.ID) {
				ctx.Error(404)
				return
			}
		}

		if !repo.IsMirror {
			ctx.Error(404)
			return
		}
		repo.IsMirror = false

		if _, err := models.CleanUpMigrateInfo(repo); err != nil {
			ctx.Handle(500, "CleanUpMigrateInfo", err)
			return
		} else if err = models.DeleteMirrorByRepoID(ctx.Repo.Repository.ID); err != nil {
			ctx.Handle(500, "DeleteMirrorByRepoID", err)
			return
		}
		log.Trace("Repository converted from mirror to regular: %s/%s", ctx.Repo.Owner.Name, repo.Name)
		ctx.Flash.Success(ctx.Tr("repo.settings.convert_succeed"))
		ctx.Redirect(setting.AppSubURL + "/" + ctx.Repo.Owner.Name + "/" + repo.Name)

	case "transfer":
		if !ctx.Repo.IsOwner() {
			ctx.Error(404)
			return
		}
		if repo.Name != f.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() && !ctx.User.IsAdmin {
			if !ctx.Repo.Owner.IsOwnedBy(ctx.User.ID) {
				ctx.Error(404)
				return
			}
		}

		newOwner := ctx.Query("new_owner_name")
		isExist, err := models.IsUserExist(0, newOwner)
		if err != nil {
			ctx.Handle(500, "IsUserExist", err)
			return
		} else if !isExist {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_owner_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if err = models.TransferOwnership(ctx.User, newOwner, repo); err != nil {
			if models.IsErrRepoAlreadyExist(err) {
				ctx.RenderWithErr(ctx.Tr("repo.settings.new_owner_has_same_repo"), SETTINGS_OPTIONS, nil)
			} else {
				ctx.Handle(500, "TransferOwnership", err)
			}
			return
		}
		log.Trace("Repository transfered: %s/%s -> %s", ctx.Repo.Owner.Name, repo.Name, newOwner)
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer_succeed"))
		ctx.Redirect(setting.AppSubURL + "/" + newOwner + "/" + repo.Name)

	case "delete":
		if !ctx.Repo.IsOwner() {
			ctx.Error(404)
			return
		}
		if repo.Name != f.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() && !ctx.User.IsAdmin {
			if !ctx.Repo.Owner.IsOwnedBy(ctx.User.ID) {
				ctx.Error(404)
				return
			}
		}

		if err := models.DeleteRepository(ctx.Repo.Owner.ID, repo.ID); err != nil {
			ctx.Handle(500, "DeleteRepository", err)
			return
		}
		log.Trace("Repository deleted: %s/%s", ctx.Repo.Owner.Name, repo.Name)

		ctx.Flash.Success(ctx.Tr("repo.settings.deletion_success"))
		ctx.Redirect(ctx.Repo.Owner.DashboardLink())

	case "delete-wiki":
		if !ctx.Repo.IsOwner() {
			ctx.Error(404)
			return
		}
		if repo.Name != f.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() && !ctx.User.IsAdmin {
			if !ctx.Repo.Owner.IsOwnedBy(ctx.User.ID) {
				ctx.Error(404)
				return
			}
		}

		repo.DeleteWiki()
		log.Trace("Repository wiki deleted: %s/%s", ctx.Repo.Owner.Name, repo.Name)

		repo.EnableWiki = false
		if err := models.UpdateRepository(repo, false); err != nil {
			ctx.Handle(500, "UpdateRepository", err)
			return
		}

		ctx.Flash.Success(ctx.Tr("repo.settings.wiki_deletion_success"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings")

	default:
		ctx.Handle(404, "", nil)
	}
}

func SettingsCollaboration(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsCollaboration"] = true

	users, err := ctx.Repo.Repository.GetCollaborators()
	if err != nil {
		ctx.Handle(500, "GetCollaborators", err)
		return
	}
	ctx.Data["Collaborators"] = users

	ctx.HTML(200, SETTINGS_COLLABORATION)
}

func SettingsCollaborationPost(ctx *context.Context) {
	name := strings.ToLower(ctx.Query("collaborator"))
	if len(name) == 0 || ctx.Repo.Owner.LowerName == name {
		ctx.Redirect(setting.AppSubURL + ctx.Req.URL.Path)
		return
	}

	u, err := models.GetUserByName(name)
	if err != nil {
		if errors.IsUserNotExist(err) {
			ctx.Flash.Error(ctx.Tr("form.user_not_exist"))
			ctx.Redirect(setting.AppSubURL + ctx.Req.URL.Path)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return
	}

	// Organization is not allowed to be added as a collaborator
	if u.IsOrganization() {
		ctx.Flash.Error(ctx.Tr("repo.settings.org_not_allowed_to_be_collaborator"))
		ctx.Redirect(setting.AppSubURL + ctx.Req.URL.Path)
		return
	}

	if err = ctx.Repo.Repository.AddCollaborator(u); err != nil {
		ctx.Handle(500, "AddCollaborator", err)
		return
	}

	if setting.Service.EnableNotifyMail {
		mailer.SendCollaboratorMail(models.NewMailerUser(u), models.NewMailerUser(ctx.User), models.NewMailerRepo(ctx.Repo.Repository))
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.add_collaborator_success"))
	ctx.Redirect(setting.AppSubURL + ctx.Req.URL.Path)
}

func ChangeCollaborationAccessMode(ctx *context.Context) {
	if err := ctx.Repo.Repository.ChangeCollaborationAccessMode(
		ctx.QueryInt64("uid"),
		models.AccessMode(ctx.QueryInt("mode"))); err != nil {
		log.Error(2, "ChangeCollaborationAccessMode: %v", err)
		return
	}

	ctx.Status(204)
}

func DeleteCollaboration(ctx *context.Context) {
	if err := ctx.Repo.Repository.DeleteCollaboration(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteCollaboration: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.remove_collaborator_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/settings/collaboration",
	})
}

func SettingsBranches(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.branches")
	ctx.Data["PageIsSettingsBranches"] = true

	if ctx.Repo.Repository.IsBare {
		ctx.Flash.Info(ctx.Tr("repo.settings.branches_bare"), true)
		ctx.HTML(200, SETTINGS_BRANCHES)
		return
	}

	protectBranches, err := models.GetProtectBranchesByRepoID(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "GetProtectBranchesByRepoID", err)
		return
	}

	// Filter out deleted branches
	branches := make([]string, 0, len(protectBranches))
	for i := range protectBranches {
		if ctx.Repo.GitRepo.IsBranchExist(protectBranches[i].Name) {
			branches = append(branches, protectBranches[i].Name)
		}
	}
	ctx.Data["ProtectBranches"] = branches

	ctx.HTML(200, SETTINGS_BRANCHES)
}

func UpdateDefaultBranch(ctx *context.Context) {
	branch := ctx.Query("branch")
	if ctx.Repo.GitRepo.IsBranchExist(branch) &&
		ctx.Repo.Repository.DefaultBranch != branch {
		ctx.Repo.Repository.DefaultBranch = branch
		if err := ctx.Repo.GitRepo.SetDefaultBranch(branch); err != nil {
			if !git.IsErrUnsupportedVersion(err) {
				ctx.Handle(500, "SetDefaultBranch", err)
				return
			}

			ctx.Flash.Warning(ctx.Tr("repo.settings.update_default_branch_unsupported"))
			ctx.Redirect(ctx.Repo.RepoLink + "/settings/branches")
			return
		}
	}

	if err := models.UpdateRepository(ctx.Repo.Repository, false); err != nil {
		ctx.Handle(500, "UpdateRepository", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_default_branch_success"))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings/branches")
}

func SettingsProtectedBranch(ctx *context.Context) {
	branch := ctx.Params("*")
	if !ctx.Repo.GitRepo.IsBranchExist(branch) {
		ctx.NotFound()
		return
	}

	ctx.Data["Title"] = ctx.Tr("repo.settings.protected_branches") + " - " + branch
	ctx.Data["PageIsSettingsBranches"] = true

	protectBranch, err := models.GetProtectBranchOfRepoByName(ctx.Repo.Repository.ID, branch)
	if err != nil {
		if !models.IsErrBranchNotExist(err) {
			ctx.Handle(500, "GetProtectBranchOfRepoByName", err)
			return
		}

		// No options found, create defaults.
		protectBranch = &models.ProtectBranch{
			Name: branch,
		}
	}

	if ctx.Repo.Owner.IsOrganization() {
		users, err := ctx.Repo.Repository.GetWriters()
		if err != nil {
			ctx.Handle(500, "Repo.Repository.GetPushers", err)
			return
		}
		ctx.Data["Users"] = users
		ctx.Data["whitelist_users"] = protectBranch.WhitelistUserIDs

		teams, err := ctx.Repo.Owner.TeamsHaveAccessToRepo(ctx.Repo.Repository.ID, models.ACCESS_MODE_WRITE)
		if err != nil {
			ctx.Handle(500, "Repo.Owner.TeamsHaveAccessToRepo", err)
			return
		}
		ctx.Data["Teams"] = teams
		ctx.Data["whitelist_teams"] = protectBranch.WhitelistTeamIDs
	}

	ctx.Data["Branch"] = protectBranch
	ctx.HTML(200, SETTINGS_PROTECTED_BRANCH)
}

func SettingsProtectedBranchPost(ctx *context.Context, f form.ProtectBranch) {
	branch := ctx.Params("*")
	if !ctx.Repo.GitRepo.IsBranchExist(branch) {
		ctx.NotFound()
		return
	}

	protectBranch, err := models.GetProtectBranchOfRepoByName(ctx.Repo.Repository.ID, branch)
	if err != nil {
		if !models.IsErrBranchNotExist(err) {
			ctx.Handle(500, "GetProtectBranchOfRepoByName", err)
			return
		}

		// No options found, create defaults.
		protectBranch = &models.ProtectBranch{
			RepoID: ctx.Repo.Repository.ID,
			Name:   branch,
		}
	}

	protectBranch.Protected = f.Protected
	protectBranch.RequirePullRequest = f.RequirePullRequest
	protectBranch.EnableWhitelist = f.EnableWhitelist
	if ctx.Repo.Owner.IsOrganization() {
		err = models.UpdateOrgProtectBranch(ctx.Repo.Repository, protectBranch, f.WhitelistUsers, f.WhitelistTeams)
	} else {
		err = models.UpdateProtectBranch(protectBranch)
	}
	if err != nil {
		ctx.Handle(500, "UpdateOrgProtectBranch/UpdateProtectBranch", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_protect_branch_success"))
	ctx.Redirect(fmt.Sprintf("%s/settings/branches/%s", ctx.Repo.RepoLink, branch))
}

func SettingsGitHooks(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.githooks")
	ctx.Data["PageIsSettingsGitHooks"] = true

	hooks, err := ctx.Repo.GitRepo.Hooks()
	if err != nil {
		ctx.Handle(500, "Hooks", err)
		return
	}
	ctx.Data["Hooks"] = hooks

	ctx.HTML(200, SETTINGS_GITHOOKS)
}

func SettingsGitHooksEdit(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.githooks")
	ctx.Data["PageIsSettingsGitHooks"] = true
	ctx.Data["RequireSimpleMDE"] = true

	name := ctx.Params(":name")
	hook, err := ctx.Repo.GitRepo.GetHook(name)
	if err != nil {
		if err == git.ErrNotValidHook {
			ctx.Handle(404, "GetHook", err)
		} else {
			ctx.Handle(500, "GetHook", err)
		}
		return
	}
	ctx.Data["Hook"] = hook
	ctx.HTML(200, SETTINGS_GITHOOK_EDIT)
}

func SettingsGitHooksEditPost(ctx *context.Context) {
	name := ctx.Params(":name")
	hook, err := ctx.Repo.GitRepo.GetHook(name)
	if err != nil {
		if err == git.ErrNotValidHook {
			ctx.Handle(404, "GetHook", err)
		} else {
			ctx.Handle(500, "GetHook", err)
		}
		return
	}
	hook.Content = ctx.Query("content")
	if err = hook.Update(); err != nil {
		ctx.Handle(500, "hook.Update", err)
		return
	}
	ctx.Redirect(ctx.Data["Link"].(string))
}

func SettingsDeployKeys(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.deploy_keys")
	ctx.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "ListDeployKeys", err)
		return
	}
	ctx.Data["Deploykeys"] = keys

	ctx.HTML(200, SETTINGS_DEPLOY_KEYS)
}

func SettingsDeployKeysPost(ctx *context.Context, f form.AddSSHKey) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.deploy_keys")
	ctx.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "ListDeployKeys", err)
		return
	}
	ctx.Data["Deploykeys"] = keys

	if ctx.HasError() {
		ctx.HTML(200, SETTINGS_DEPLOY_KEYS)
		return
	}

	content, err := models.CheckPublicKeyString(f.Content)
	if err != nil {
		if models.IsErrKeyUnableVerify(err) {
			ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
		} else {
			ctx.Data["HasError"] = true
			ctx.Data["Err_Content"] = true
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
			ctx.Redirect(ctx.Repo.RepoLink + "/settings/keys")
			return
		}
	}

	key, err := models.AddDeployKey(ctx.Repo.Repository.ID, f.Title, content)
	if err != nil {
		ctx.Data["HasError"] = true
		switch {
		case models.IsErrKeyAlreadyExist(err):
			ctx.Data["Err_Content"] = true
			ctx.RenderWithErr(ctx.Tr("repo.settings.key_been_used"), SETTINGS_DEPLOY_KEYS, &f)
		case models.IsErrKeyNameAlreadyUsed(err):
			ctx.Data["Err_Title"] = true
			ctx.RenderWithErr(ctx.Tr("repo.settings.key_name_used"), SETTINGS_DEPLOY_KEYS, &f)
		default:
			ctx.Handle(500, "AddDeployKey", err)
		}
		return
	}

	log.Trace("Deploy key added: %d", ctx.Repo.Repository.ID)
	ctx.Flash.Success(ctx.Tr("repo.settings.add_key_success", key.Name))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings/keys")
}

func DeleteDeployKey(ctx *context.Context) {
	if err := models.DeleteDeployKey(ctx.User, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteDeployKey: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.deploy_key_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/settings/keys",
	})
}
