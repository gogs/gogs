// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/gogs/git-module"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/email"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/tool"
)

const (
	SETTINGS_OPTIONS          = "repo/settings/options"
	SETTINGS_REPO_AVATAR      = "repo/settings/avatar"
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
	c.RequireAutosize()
	c.Success(SETTINGS_OPTIONS)
}

func SettingsPost(c *context.Context, f form.RepoSetting) {
	c.Title("repo.settings")
	c.PageIs("SettingsOptions")
	c.RequireAutosize()

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
			if err := db.ChangeRepositoryName(c.Repo.Owner, repo.Name, newRepoName); err != nil {
				c.FormErr("RepoName")
				switch {
				case db.IsErrRepoAlreadyExist(err):
					c.RenderWithErr(c.Tr("form.repo_name_been_taken"), SETTINGS_OPTIONS, &f)
				case db.IsErrNameNotAllowed(err):
					c.RenderWithErr(c.Tr("repo.form.name_not_allowed", err.(db.ErrNameNotAllowed).Value()), SETTINGS_OPTIONS, &f)
				default:
					c.Error(err, "change repository name")
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
			f.Unlisted = repo.BaseRepo.IsUnlisted
		}

		visibilityChanged := repo.IsPrivate != f.Private || repo.IsUnlisted != f.Unlisted
		repo.IsPrivate = f.Private
		repo.IsUnlisted = f.Unlisted
		if err := db.UpdateRepository(repo, visibilityChanged); err != nil {
			c.Error(err, "update repository")
			return
		}
		log.Trace("Repository basic settings updated: %s/%s", c.Repo.Owner.Name, repo.Name)

		if isNameChanged {
			if err := db.RenameRepoAction(c.User, oldRepoName, repo); err != nil {
				log.Error("RenameRepoAction: %v", err)
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
			if err := db.UpdateMirror(c.Repo.Mirror); err != nil {
				c.Error(err, "update mirror")
				return
			}
		}
		if err := c.Repo.Mirror.SaveAddress(f.MirrorAddress); err != nil {
			c.Error(err, "save address")
			return
		}

		c.Flash.Success(c.Tr("repo.settings.update_settings_success"))
		c.Redirect(repo.Link() + "/settings")

	case "mirror-sync":
		if !repo.IsMirror {
			c.NotFound()
			return
		}

		go db.MirrorQueue.Add(repo.ID)
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

		if !repo.EnableWiki || repo.EnableExternalWiki {
			repo.AllowPublicWiki = false
		}
		if !repo.EnableIssues || repo.EnableExternalTracker {
			repo.AllowPublicIssues = false
		}

		if err := db.UpdateRepository(repo, false); err != nil {
			c.Error(err, "update repository")
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

		if _, err := db.CleanUpMigrateInfo(repo); err != nil {
			c.Error(err, "clean up migrate info")
			return
		} else if err = db.DeleteMirrorByRepoID(c.Repo.Repository.ID); err != nil {
			c.Error(err, "delete mirror by repository ID")
			return
		}
		log.Trace("Repository converted from mirror to regular: %s/%s", c.Repo.Owner.Name, repo.Name)
		c.Flash.Success(c.Tr("repo.settings.convert_succeed"))
		c.Redirect(conf.Server.Subpath + "/" + c.Repo.Owner.Name + "/" + repo.Name)

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
		isExist, err := db.IsUserExist(0, newOwner)
		if err != nil {
			c.Error(err, "check if user exists")
			return
		} else if !isExist {
			c.RenderWithErr(c.Tr("form.enterred_invalid_owner_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if err = db.TransferOwnership(c.User, newOwner, repo); err != nil {
			if db.IsErrRepoAlreadyExist(err) {
				c.RenderWithErr(c.Tr("repo.settings.new_owner_has_same_repo"), SETTINGS_OPTIONS, nil)
			} else {
				c.Error(err, "transfer ownership")
			}
			return
		}
		log.Trace("Repository transfered: %s/%s -> %s", c.Repo.Owner.Name, repo.Name, newOwner)
		c.Flash.Success(c.Tr("repo.settings.transfer_succeed"))
		c.Redirect(conf.Server.Subpath + "/" + newOwner + "/" + repo.Name)

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

		if err := db.DeleteRepository(c.Repo.Owner.ID, repo.ID); err != nil {
			c.Error(err, "delete repository")
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
		if err := db.UpdateRepository(repo, false); err != nil {
			c.Error(err, "update repository")
			return
		}

		c.Flash.Success(c.Tr("repo.settings.wiki_deletion_success"))
		c.Redirect(c.Repo.RepoLink + "/settings")

	default:
		c.NotFound()
	}
}

func SettingsAvatar(c *context.Context) {
	c.Title("settings.avatar")
	c.PageIs("SettingsAvatar")
	c.Success(SETTINGS_REPO_AVATAR)
}

func SettingsAvatarPost(c *context.Context, f form.Avatar) {
	f.Source = form.AVATAR_LOCAL
	if err := UpdateAvatarSetting(c, f, c.Repo.Repository); err != nil {
		c.Flash.Error(err.Error())
	} else {
		c.Flash.Success(c.Tr("settings.update_avatar_success"))
	}
	c.RedirectSubpath(c.Repo.RepoLink + "/settings")
}

func SettingsDeleteAvatar(c *context.Context) {
	if err := c.Repo.Repository.DeleteAvatar(); err != nil {
		c.Flash.Error(fmt.Sprintf("Failed to delete avatar: %v", err))
	}
	c.RedirectSubpath(c.Repo.RepoLink + "/settings")
}

// FIXME: limit upload size
func UpdateAvatarSetting(c *context.Context, f form.Avatar, ctxRepo *db.Repository) error {
	ctxRepo.UseCustomAvatar = true
	if f.Avatar != nil {
		r, err := f.Avatar.Open()
		if err != nil {
			return fmt.Errorf("open avatar reader: %v", err)
		}
		defer r.Close()

		data, err := ioutil.ReadAll(r)
		if err != nil {
			return fmt.Errorf("read avatar content: %v", err)
		}
		if !tool.IsImageFile(data) {
			return errors.New(c.Tr("settings.uploaded_avatar_not_a_image"))
		}
		if err = ctxRepo.UploadAvatar(data); err != nil {
			return fmt.Errorf("upload avatar: %v", err)
		}
	} else {
		// No avatar is uploaded and reset setting back.
		if !com.IsFile(ctxRepo.CustomAvatarPath()) {
			ctxRepo.UseCustomAvatar = false
		}
	}

	if err := db.UpdateRepository(ctxRepo, false); err != nil {
		return fmt.Errorf("update repository: %v", err)
	}

	return nil
}

func SettingsCollaboration(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsCollaboration"] = true

	users, err := c.Repo.Repository.GetCollaborators()
	if err != nil {
		c.Error(err, "get collaborators")
		return
	}
	c.Data["Collaborators"] = users

	c.Success(SETTINGS_COLLABORATION)
}

func SettingsCollaborationPost(c *context.Context) {
	name := strings.ToLower(c.Query("collaborator"))
	if len(name) == 0 || c.Repo.Owner.LowerName == name {
		c.Redirect(conf.Server.Subpath + c.Req.URL.Path)
		return
	}

	u, err := db.GetUserByName(name)
	if err != nil {
		if db.IsErrUserNotExist(err) {
			c.Flash.Error(c.Tr("form.user_not_exist"))
			c.Redirect(conf.Server.Subpath + c.Req.URL.Path)
		} else {
			c.Error(err, "get user by name")
		}
		return
	}

	// Organization is not allowed to be added as a collaborator
	if u.IsOrganization() {
		c.Flash.Error(c.Tr("repo.settings.org_not_allowed_to_be_collaborator"))
		c.Redirect(conf.Server.Subpath + c.Req.URL.Path)
		return
	}

	if err = c.Repo.Repository.AddCollaborator(u); err != nil {
		c.Error(err, "add collaborator")
		return
	}

	if conf.User.EnableEmailNotification {
		email.SendCollaboratorMail(db.NewMailerUser(u), db.NewMailerUser(c.User), db.NewMailerRepo(c.Repo.Repository))
	}

	c.Flash.Success(c.Tr("repo.settings.add_collaborator_success"))
	c.Redirect(conf.Server.Subpath + c.Req.URL.Path)
}

func ChangeCollaborationAccessMode(c *context.Context) {
	if err := c.Repo.Repository.ChangeCollaborationAccessMode(
		c.QueryInt64("uid"),
		db.AccessMode(c.QueryInt("mode"))); err != nil {
		log.Error("ChangeCollaborationAccessMode: %v", err)
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

	c.JSONSuccess(map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/collaboration",
	})
}

func SettingsBranches(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.branches")
	c.Data["PageIsSettingsBranches"] = true

	if c.Repo.Repository.IsBare {
		c.Flash.Info(c.Tr("repo.settings.branches_bare"), true)
		c.Success(SETTINGS_BRANCHES)
		return
	}

	protectBranches, err := db.GetProtectBranchesByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get protect branch by repository ID")
		return
	}

	// Filter out deleted branches
	branches := make([]string, 0, len(protectBranches))
	for i := range protectBranches {
		if c.Repo.GitRepo.HasBranch(protectBranches[i].Name) {
			branches = append(branches, protectBranches[i].Name)
		}
	}
	c.Data["ProtectBranches"] = branches

	c.Success(SETTINGS_BRANCHES)
}

func UpdateDefaultBranch(c *context.Context) {
	branch := c.Query("branch")
	if c.Repo.GitRepo.HasBranch(branch) &&
		c.Repo.Repository.DefaultBranch != branch {
		c.Repo.Repository.DefaultBranch = branch
		if _, err := c.Repo.GitRepo.SymbolicRef(git.SymbolicRefOptions{
			Ref: git.RefsHeads + branch,
		}); err != nil {
			c.Flash.Warning(c.Tr("repo.settings.update_default_branch_unsupported"))
			c.Redirect(c.Repo.RepoLink + "/settings/branches")
			return
		}
	}

	if err := db.UpdateRepository(c.Repo.Repository, false); err != nil {
		c.Error(err, "update repository")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_default_branch_success"))
	c.Redirect(c.Repo.RepoLink + "/settings/branches")
}

func SettingsProtectedBranch(c *context.Context) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.HasBranch(branch) {
		c.NotFound()
		return
	}

	c.Data["Title"] = c.Tr("repo.settings.protected_branches") + " - " + branch
	c.Data["PageIsSettingsBranches"] = true

	protectBranch, err := db.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !db.IsErrBranchNotExist(err) {
			c.Error(err, "get protect branch of repository by name")
			return
		}

		// No options found, create defaults.
		protectBranch = &db.ProtectBranch{
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

		teams, err := c.Repo.Owner.TeamsHaveAccessToRepo(c.Repo.Repository.ID, db.AccessModeWrite)
		if err != nil {
			c.Error(err, "get teams have access to the repository")
			return
		}
		c.Data["Teams"] = teams
		c.Data["whitelist_teams"] = protectBranch.WhitelistTeamIDs
	}

	c.Data["Branch"] = protectBranch
	c.Success(SETTINGS_PROTECTED_BRANCH)
}

func SettingsProtectedBranchPost(c *context.Context, f form.ProtectBranch) {
	branch := c.Params("*")
	if !c.Repo.GitRepo.HasBranch(branch) {
		c.NotFound()
		return
	}

	protectBranch, err := db.GetProtectBranchOfRepoByName(c.Repo.Repository.ID, branch)
	if err != nil {
		if !db.IsErrBranchNotExist(err) {
			c.Error(err, "get protect branch of repository by name")
			return
		}

		// No options found, create defaults.
		protectBranch = &db.ProtectBranch{
			RepoID: c.Repo.Repository.ID,
			Name:   branch,
		}
	}

	protectBranch.Protected = f.Protected
	protectBranch.RequirePullRequest = f.RequirePullRequest
	protectBranch.EnableWhitelist = f.EnableWhitelist
	if c.Repo.Owner.IsOrganization() {
		err = db.UpdateOrgProtectBranch(c.Repo.Repository, protectBranch, f.WhitelistUsers, f.WhitelistTeams)
	} else {
		err = db.UpdateProtectBranch(protectBranch)
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

	c.Success(SETTINGS_GITHOOKS)
}

func SettingsGitHooksEdit(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.githooks")
	c.Data["PageIsSettingsGitHooks"] = true
	c.Data["RequireSimpleMDE"] = true

	name := c.Params(":name")
	hook, err := c.Repo.GitRepo.Hook("custom_hooks", git.HookName(name))
	if err != nil {
		c.NotFoundOrError(osutil.NewError(err), "get hook")
		return
	}
	c.Data["Hook"] = hook
	c.Success(SETTINGS_GITHOOK_EDIT)
}

func SettingsGitHooksEditPost(c *context.Context) {
	name := c.Params(":name")
	hook, err := c.Repo.GitRepo.Hook("custom_hooks", git.HookName(name))
	if err != nil {
		c.NotFoundOrError(osutil.NewError(err), "get hook")
		return
	}
	if err = hook.Update(c.Query("content")); err != nil {
		c.Error(err, "update hook")
		return
	}
	c.Redirect(c.Data["Link"].(string))
}

func SettingsDeployKeys(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.deploy_keys")
	c.Data["PageIsSettingsKeys"] = true

	keys, err := db.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "list deploy keys")
		return
	}
	c.Data["Deploykeys"] = keys

	c.Success(SETTINGS_DEPLOY_KEYS)
}

func SettingsDeployKeysPost(c *context.Context, f form.AddSSHKey) {
	c.Data["Title"] = c.Tr("repo.settings.deploy_keys")
	c.Data["PageIsSettingsKeys"] = true

	keys, err := db.ListDeployKeys(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "list deploy keys")
		return
	}
	c.Data["Deploykeys"] = keys

	if c.HasError() {
		c.Success(SETTINGS_DEPLOY_KEYS)
		return
	}

	content, err := db.CheckPublicKeyString(f.Content)
	if err != nil {
		if db.IsErrKeyUnableVerify(err) {
			c.Flash.Info(c.Tr("form.unable_verify_ssh_key"))
		} else {
			c.Data["HasError"] = true
			c.Data["Err_Content"] = true
			c.Flash.Error(c.Tr("form.invalid_ssh_key", err.Error()))
			c.Redirect(c.Repo.RepoLink + "/settings/keys")
			return
		}
	}

	key, err := db.AddDeployKey(c.Repo.Repository.ID, f.Title, content)
	if err != nil {
		c.Data["HasError"] = true
		switch {
		case db.IsErrKeyAlreadyExist(err):
			c.Data["Err_Content"] = true
			c.RenderWithErr(c.Tr("repo.settings.key_been_used"), SETTINGS_DEPLOY_KEYS, &f)
		case db.IsErrKeyNameAlreadyUsed(err):
			c.Data["Err_Title"] = true
			c.RenderWithErr(c.Tr("repo.settings.key_name_used"), SETTINGS_DEPLOY_KEYS, &f)
		default:
			c.Error(err, "add deploy key")
		}
		return
	}

	log.Trace("Deploy key added: %d", c.Repo.Repository.ID)
	c.Flash.Success(c.Tr("repo.settings.add_key_success", key.Name))
	c.Redirect(c.Repo.RepoLink + "/settings/keys")
}

func DeleteDeployKey(c *context.Context) {
	if err := db.DeleteDeployKey(c.User, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteDeployKey: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.deploy_key_deletion_success"))
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/keys",
	})
}
