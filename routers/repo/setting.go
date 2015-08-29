// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/git"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTINGS_OPTIONS base.TplName = "repo/settings/options"
	COLLABORATION    base.TplName = "repo/settings/collaboration"
	HOOKS            base.TplName = "repo/settings/hooks"
	HOOK_NEW         base.TplName = "repo/settings/hook_new"
	ORG_HOOK_NEW     base.TplName = "org/settings/hook_new"
	GITHOOKS         base.TplName = "repo/settings/githooks"
	GITHOOK_EDIT     base.TplName = "repo/settings/githook_edit"
	DEPLOY_KEYS      base.TplName = "repo/settings/deploy_keys"
)

func Settings(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsOptions"] = true
	ctx.HTML(200, SETTINGS_OPTIONS)
}

func SettingsPost(ctx *middleware.Context, form auth.RepoSettingForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsOptions"] = true

	switch ctx.Query("action") {
	case "update":
		if ctx.HasError() {
			ctx.HTML(200, SETTINGS_OPTIONS)
			return
		}

		newRepoName := form.RepoName
		// Check if repository name has been changed.
		if ctx.Repo.Repository.Name != newRepoName {
			if err := models.ChangeRepositoryName(ctx.Repo.Owner, ctx.Repo.Repository.Name, newRepoName); err != nil {
				switch {
				case models.IsErrRepoAlreadyExist(err):
					ctx.Data["Err_RepoName"] = true
					ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), SETTINGS_OPTIONS, &form)
				case models.IsErrNameReserved(err):
					ctx.Data["Err_RepoName"] = true
					ctx.RenderWithErr(ctx.Tr("repo.form.name_reserved", err.(models.ErrNameReserved).Name), SETTINGS_OPTIONS, &form)
				case models.IsErrNamePatternNotAllowed(err):
					ctx.Data["Err_RepoName"] = true
					ctx.RenderWithErr(ctx.Tr("repo.form.name_pattern_not_allowed", err.(models.ErrNamePatternNotAllowed).Pattern), SETTINGS_OPTIONS, &form)
				default:
					ctx.Handle(500, "ChangeRepositoryName", err)
				}
				return
			}
			log.Trace("Repository name changed: %s/%s -> %s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName)
			ctx.Repo.Repository.Name = newRepoName
			ctx.Repo.Repository.LowerName = strings.ToLower(newRepoName)
		}

		br := form.Branch

		if ctx.Repo.GitRepo.IsBranchExist(br) {
			ctx.Repo.Repository.DefaultBranch = br
		}
		ctx.Repo.Repository.Description = form.Description
		ctx.Repo.Repository.Website = form.Website
		visibilityChanged := ctx.Repo.Repository.IsPrivate != form.Private
		ctx.Repo.Repository.IsPrivate = form.Private
		if err := models.UpdateRepository(ctx.Repo.Repository, visibilityChanged); err != nil {
			ctx.Handle(404, "UpdateRepository", err)
			return
		}
		log.Trace("Repository updated: %s/%s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)

		if ctx.Repo.Repository.IsMirror {
			if form.Interval > 0 {
				ctx.Repo.Mirror.Interval = form.Interval
				ctx.Repo.Mirror.NextUpdate = time.Now().Add(time.Duration(form.Interval) * time.Hour)
				if err := models.UpdateMirror(ctx.Repo.Mirror); err != nil {
					log.Error(4, "UpdateMirror: %v", err)
				}
			}
		}

		ctx.Flash.Success(ctx.Tr("repo.settings.update_settings_success"))
		ctx.Redirect(fmt.Sprintf("%s/%s/%s/settings", setting.AppSubUrl, ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
	case "transfer":
		if ctx.Repo.Repository.Name != form.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
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

		if _, err = models.UserSignIn(ctx.User.Name, ctx.Query("password")); err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_password"), SETTINGS_OPTIONS, nil)
			} else {
				ctx.Handle(500, "UserSignIn", err)
			}
			return
		}

		if err = models.TransferOwnership(ctx.User, newOwner, ctx.Repo.Repository); err != nil {
			if models.IsErrRepoAlreadyExist(err) {
				ctx.RenderWithErr(ctx.Tr("repo.settings.new_owner_has_same_repo"), SETTINGS_OPTIONS, nil)
			} else {
				ctx.Handle(500, "TransferOwnership", err)
			}
			return
		}
		log.Trace("Repository transfered: %s/%s -> %s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newOwner)
		ctx.Flash.Success(ctx.Tr("repo.settings.transfer_succeed"))
		ctx.Redirect(setting.AppSubUrl + "/")
	case "delete":
		if ctx.Repo.Repository.Name != form.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() {
			if !ctx.Repo.Owner.IsOwnedBy(ctx.User.Id) {
				ctx.Error(404)
				return
			}
		}

		if _, err := models.UserSignIn(ctx.User.Name, ctx.Query("password")); err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_password"), SETTINGS_OPTIONS, nil)
			} else {
				ctx.Handle(500, "UserSignIn", err)
			}
			return
		}

		if err := models.DeleteRepository(ctx.Repo.Owner.Id, ctx.Repo.Repository.ID, ctx.Repo.Owner.Name); err != nil {
			ctx.Handle(500, "DeleteRepository", err)
			return
		}
		log.Trace("Repository deleted: %s/%s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
		if ctx.Repo.Owner.IsOrganization() {
			ctx.Redirect(setting.AppSubUrl + "/org/" + ctx.Repo.Owner.Name + "/dashboard")
		} else {
			ctx.Redirect(setting.AppSubUrl + "/")
		}
	}
}

func Collaboration(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsCollaboration"] = true

	if ctx.Req.Method == "POST" {
		name := strings.ToLower(ctx.Query("collaborator"))
		if len(name) == 0 || ctx.Repo.Owner.LowerName == name {
			ctx.Redirect(setting.AppSubUrl + ctx.Req.URL.Path)
			return
		}

		u, err := models.GetUserByName(name)
		if err != nil {
			if models.IsErrUserNotExist(err) {
				ctx.Flash.Error(ctx.Tr("form.user_not_exist"))
				ctx.Redirect(setting.AppSubUrl + ctx.Req.URL.Path)
			} else {
				ctx.Handle(500, "GetUserByName", err)
			}
			return
		}

		// Check if user is organization member.
		if ctx.Repo.Owner.IsOrganization() && ctx.Repo.Owner.IsOrgMember(u.Id) {
			ctx.Flash.Info(ctx.Tr("repo.settings.user_is_org_member"))
			ctx.Redirect(ctx.Repo.RepoLink + "/settings/collaboration")
			return
		}

		if err = ctx.Repo.Repository.AddCollaborator(u); err != nil {
			ctx.Handle(500, "AddCollaborator", err)
			return
		}

		if setting.Service.EnableNotifyMail {
			if err = mailer.SendCollaboratorMail(ctx.Render, u, ctx.User, ctx.Repo.Repository); err != nil {
				ctx.Handle(500, "SendCollaboratorMail", err)
				return
			}
		}

		ctx.Flash.Success(ctx.Tr("repo.settings.add_collaborator_success"))
		ctx.Redirect(setting.AppSubUrl + ctx.Req.URL.Path)
		return
	}

	// Delete collaborator.
	remove := strings.ToLower(ctx.Query("remove"))
	if len(remove) > 0 && remove != ctx.Repo.Owner.LowerName {
		u, err := models.GetUserByName(remove)
		if err != nil {
			ctx.Handle(500, "GetUserByName", err)
			return
		}
		if err := ctx.Repo.Repository.DeleteCollaborator(u); err != nil {
			ctx.Handle(500, "DeleteCollaborator", err)
			return
		}
		ctx.Flash.Success(ctx.Tr("repo.settings.remove_collaborator_success"))
		ctx.Redirect(ctx.Repo.RepoLink + "/settings/collaboration")
		return
	}

	users, err := ctx.Repo.Repository.GetCollaborators()
	if err != nil {
		ctx.Handle(500, "GetCollaborators", err)
		return
	}

	ctx.Data["Collaborators"] = users
	ctx.HTML(200, COLLABORATION)
}

func Webhooks(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.hooks")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["BaseLink"] = ctx.Repo.RepoLink
	ctx.Data["Description"] = ctx.Tr("repo.settings.hooks_desc", "http://gogs.io/docs/features/webhook.html")

	ws, err := models.GetWebhooksByRepoId(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "GetWebhooksByRepoId", err)
		return
	}
	ctx.Data["Webhooks"] = ws

	ctx.HTML(200, HOOKS)
}

type OrgRepoCtx struct {
	OrgID       int64
	RepoID      int64
	Link        string
	NewTemplate base.TplName
}

// getOrgRepoCtx determines whether this is a repo context or organization context.
func getOrgRepoCtx(ctx *middleware.Context) (*OrgRepoCtx, error) {
	if len(ctx.Repo.RepoLink) > 0 {
		return &OrgRepoCtx{
			RepoID:      ctx.Repo.Repository.ID,
			Link:        ctx.Repo.RepoLink,
			NewTemplate: HOOK_NEW,
		}, nil
	}

	if len(ctx.Org.OrgLink) > 0 {
		return &OrgRepoCtx{
			OrgID:       ctx.Org.Organization.Id,
			Link:        ctx.Org.OrgLink,
			NewTemplate: ORG_HOOK_NEW,
		}, nil
	}

	return nil, errors.New("Unable to set OrgRepo context")
}

func checkHookType(ctx *middleware.Context) string {
	hookType := strings.ToLower(ctx.Params(":type"))
	if !com.IsSliceContainsStr(setting.Webhook.Types, hookType) {
		ctx.Handle(404, "checkHookType", nil)
		return ""
	}
	return hookType
}

func WebhooksNew(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.add_webhook")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksNew"] = true
	ctx.Data["Webhook"] = models.Webhook{HookEvent: &models.HookEvent{}}

	orCtx, err := getOrgRepoCtx(ctx)
	if err != nil {
		ctx.Handle(500, "getOrgRepoCtx", err)
		return
	}

	ctx.Data["HookType"] = checkHookType(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["BaseLink"] = orCtx.Link

	ctx.HTML(200, orCtx.NewTemplate)
}

func ParseHookEvent(form auth.WebhookForm) *models.HookEvent {
	return &models.HookEvent{
		PushOnly:       form.PushOnly(),
		SendEverything: form.SendEverything(),
		ChooseEvents:   form.ChooseEvents(),
		HookEvents: models.HookEvents{
			Create: form.Create,
			Push:   form.Push,
		},
	}
}

func WebHooksNewPost(ctx *middleware.Context, form auth.NewWebhookForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.add_webhook")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksNew"] = true
	ctx.Data["Webhook"] = models.Webhook{HookEvent: &models.HookEvent{}}
	ctx.Data["HookType"] = "gogs"

	orCtx, err := getOrgRepoCtx(ctx)
	if err != nil {
		ctx.Handle(500, "getOrgRepoCtx", err)
		return
	}
	ctx.Data["BaseLink"] = orCtx.Link

	if ctx.HasError() {
		ctx.HTML(200, orCtx.NewTemplate)
		return
	}

	contentType := models.JSON
	if models.HookContentType(form.ContentType) == models.FORM {
		contentType = models.FORM
	}

	w := &models.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          form.PayloadURL,
		ContentType:  contentType,
		Secret:       form.Secret,
		HookEvent:    ParseHookEvent(form.WebhookForm),
		IsActive:     form.Active,
		HookTaskType: models.GOGS,
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "UpdateEvent", err)
		return
	} else if err := models.CreateWebhook(w); err != nil {
		ctx.Handle(500, "CreateWebhook", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.add_hook_success"))
	ctx.Redirect(orCtx.Link + "/settings/hooks")
}

func SlackHooksNewPost(ctx *middleware.Context, form auth.NewSlackHookForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksNew"] = true
	ctx.Data["Webhook"] = models.Webhook{HookEvent: &models.HookEvent{}}

	orCtx, err := getOrgRepoCtx(ctx)
	if err != nil {
		ctx.Handle(500, "getOrgRepoCtx", err)
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := json.Marshal(&models.SlackMeta{
		Channel:  form.Channel,
		Username: form.Username,
		IconURL:  form.IconURL,
		Color:    form.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w := &models.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          form.PayloadURL,
		ContentType:  models.JSON,
		HookEvent:    ParseHookEvent(form.WebhookForm),
		IsActive:     form.Active,
		HookTaskType: models.SLACK,
		Meta:         string(meta),
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "UpdateEvent", err)
		return
	} else if err := models.CreateWebhook(w); err != nil {
		ctx.Handle(500, "CreateWebhook", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.add_hook_success"))
	ctx.Redirect(orCtx.Link + "/settings/hooks")
}

func checkWebhook(ctx *middleware.Context) (*OrgRepoCtx, *models.Webhook) {
	ctx.Data["RequireHighlightJS"] = true

	orCtx, err := getOrgRepoCtx(ctx)
	if err != nil {
		ctx.Handle(500, "getOrgRepoCtx", err)
		return nil, nil
	}
	ctx.Data["BaseLink"] = orCtx.Link

	w, err := models.GetWebhookByID(ctx.ParamsInt64(":id"))
	if err != nil {
		if models.IsErrWebhookNotExist(err) {
			ctx.Handle(404, "GetWebhookByID", nil)
		} else {
			ctx.Handle(500, "GetWebhookByID", err)
		}
		return nil, nil
	}

	switch w.HookTaskType {
	case models.SLACK:
		ctx.Data["SlackHook"] = w.GetSlackHook()
		ctx.Data["HookType"] = "slack"
	default:
		ctx.Data["HookType"] = "gogs"
	}

	ctx.Data["History"], err = w.History(1)
	if err != nil {
		ctx.Handle(500, "History", err)
	}
	return orCtx, w
}

func WebHooksEdit(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.update_webhook")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["Webhook"] = w

	ctx.HTML(200, orCtx.NewTemplate)
}

func WebHooksEditPost(ctx *middleware.Context, form auth.NewWebhookForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.update_webhook")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["Webhook"] = w

	if ctx.HasError() {
		ctx.HTML(200, orCtx.NewTemplate)
		return
	}

	contentType := models.JSON
	if models.HookContentType(form.ContentType) == models.FORM {
		contentType = models.FORM
	}

	w.URL = form.PayloadURL
	w.ContentType = contentType
	w.Secret = form.Secret
	w.HookEvent = ParseHookEvent(form.WebhookForm)
	w.IsActive = form.Active
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "UpdateEvent", err)
		return
	} else if err := models.UpdateWebhook(w); err != nil {
		ctx.Handle(500, "WebHooksEditPost", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_hook_success"))
	ctx.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func SlackHooksEditPost(ctx *middleware.Context, form auth.NewSlackHookForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(ctx)
	if ctx.Written() {
		return
	}
	ctx.Data["Webhook"] = w

	if ctx.HasError() {
		ctx.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := json.Marshal(&models.SlackMeta{
		Channel:  form.Channel,
		Username: form.Username,
		IconURL:  form.IconURL,
		Color:    form.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w.URL = form.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = ParseHookEvent(form.WebhookForm)
	w.IsActive = form.Active
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "UpdateEvent", err)
		return
	} else if err := models.UpdateWebhook(w); err != nil {
		ctx.Handle(500, "UpdateWebhook", err)
		return
	}

	ctx.Flash.Success(ctx.Tr("repo.settings.update_hook_success"))
	ctx.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func DeleteWebhook(ctx *middleware.Context) {
	if err := models.DeleteWebhook(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteWebhook: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.webhook_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/settings/hooks",
	})
}

func TriggerHook(ctx *middleware.Context) {
	u, err := models.GetUserByName(ctx.Params(":username"))
	if err != nil {
		if models.IsErrUserNotExist(err) {
			ctx.Handle(404, "GetUserByName", err)
		} else {
			ctx.Handle(500, "GetUserByName", err)
		}
		return
	}

	repo, err := models.GetRepositoryByName(u.Id, ctx.Params(":reponame"))
	if err != nil {
		if models.IsErrRepoNotExist(err) {
			ctx.Handle(404, "GetRepositoryByName", err)
		} else {
			ctx.Handle(500, "GetRepositoryByName", err)
		}
		return
	}
	models.HookQueue.AddRepoID(repo.ID)
	ctx.Status(200)
}

func GitHooks(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.githooks")
	ctx.Data["PageIsSettingsGitHooks"] = true

	hooks, err := ctx.Repo.GitRepo.Hooks()
	if err != nil {
		ctx.Handle(500, "Hooks", err)
		return
	}
	ctx.Data["Hooks"] = hooks

	ctx.HTML(200, GITHOOKS)
}

func GitHooksEdit(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.githooks")
	ctx.Data["PageIsSettingsGitHooks"] = true

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
	ctx.HTML(200, GITHOOK_EDIT)
}

func GitHooksEditPost(ctx *middleware.Context) {
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
	ctx.Redirect(ctx.Repo.RepoLink + "/settings/hooks/git")
}

func DeployKeys(ctx *middleware.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.deploy_keys")
	ctx.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "ListDeployKeys", err)
		return
	}
	ctx.Data["Deploykeys"] = keys

	ctx.HTML(200, DEPLOY_KEYS)
}

func DeployKeysPost(ctx *middleware.Context, form auth.AddSSHKeyForm) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.deploy_keys")
	ctx.Data["PageIsSettingsKeys"] = true

	keys, err := models.ListDeployKeys(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "ListDeployKeys", err)
		return
	}
	ctx.Data["Deploykeys"] = keys

	if ctx.HasError() {
		ctx.HTML(200, DEPLOY_KEYS)
		return
	}

	content, err := models.CheckPublicKeyString(form.Content)
	if err != nil {
		if err == models.ErrKeyUnableVerify {
			ctx.Flash.Info(ctx.Tr("form.unable_verify_ssh_key"))
		} else {
			ctx.Data["HasError"] = true
			ctx.Data["Err_Content"] = true
			ctx.Flash.Error(ctx.Tr("form.invalid_ssh_key", err.Error()))
			ctx.Redirect(ctx.Repo.RepoLink + "/settings/keys")
			return
		}
	}

	if err = models.AddDeployKey(ctx.Repo.Repository.ID, form.Title, content); err != nil {
		ctx.Data["HasError"] = true
		switch {
		case models.IsErrKeyAlreadyExist(err):
			ctx.Data["Err_Content"] = true
			ctx.RenderWithErr(ctx.Tr("repo.settings.key_been_used"), DEPLOY_KEYS, &form)
		case models.IsErrKeyNameAlreadyUsed(err):
			ctx.Data["Err_Title"] = true
			ctx.RenderWithErr(ctx.Tr("repo.settings.key_name_used"), DEPLOY_KEYS, &form)
		default:
			ctx.Handle(500, "AddDeployKey", err)
		}
		return
	}

	log.Trace("Deploy key added: %d", ctx.Repo.Repository.ID)
	ctx.Flash.Success(ctx.Tr("repo.settings.add_key_success", form.Title))
	ctx.Redirect(ctx.Repo.RepoLink + "/settings/keys")
}

func DeleteDeployKey(ctx *middleware.Context) {
	if err := models.DeleteDeployKey(ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteDeployKey: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.deploy_key_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/settings/keys",
	})
}
