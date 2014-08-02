// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTINGS_OPTIONS base.TplName = "repo/settings/options"
	COLLABORATION    base.TplName = "repo/collaboration"

	HOOKS     base.TplName = "repo/hooks"
	HOOK_ADD  base.TplName = "repo/hook_add"
	HOOK_EDIT base.TplName = "repo/hook_edit"
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
			isExist, err := models.IsRepositoryExist(ctx.Repo.Owner, newRepoName)
			if err != nil {
				ctx.Handle(500, "IsRepositoryExist", err)
				return
			} else if isExist {
				ctx.Data["Err_RepoName"] = true
				ctx.RenderWithErr(ctx.Tr("form.repo_name_been_taken"), SETTINGS_OPTIONS, nil)
				return
			} else if err = models.ChangeRepositoryName(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName); err != nil {
				ctx.Handle(500, "ChangeRepositoryName", err)
				return
			}
			log.Trace("Repository name changed: %s/%s -> %s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName)
			ctx.Repo.Repository.Name = newRepoName
		}

		br := form.Branch

		if ctx.Repo.GitRepo.IsBranchExist(br) {
			ctx.Repo.Repository.DefaultBranch = br
		}
		ctx.Repo.Repository.Description = form.Description
		ctx.Repo.Repository.Website = form.Website
		ctx.Repo.Repository.IsPrivate = form.Private
		ctx.Repo.Repository.IsGoget = form.GoGet
		if err := models.UpdateRepository(ctx.Repo.Repository); err != nil {
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
		ctx.Redirect(fmt.Sprintf("/%s/%s/settings", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
	case "transfer":
		if ctx.Repo.Repository.Name != form.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		}

		newOwner := ctx.Query("new_owner_name")
		isExist, err := models.IsUserExist(newOwner)
		if err != nil {
			ctx.Handle(500, "IsUserExist", err)
			return
		} else if !isExist {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_owner_name"), SETTINGS_OPTIONS, nil)
			return
		} else if err = models.TransferOwnership(ctx.Repo.Owner, newOwner, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "TransferOwnership", err)
			return
		}
		log.Trace("Repository transfered: %s/%s -> %s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newOwner)
		ctx.Redirect("/")
	case "delete":
		if ctx.Repo.Repository.Name != form.RepoName {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_repo_name"), SETTINGS_OPTIONS, nil)
			return
		} else if !ctx.Repo.Owner.ValidtePassword(ctx.Query("password")) {
			ctx.RenderWithErr(ctx.Tr("form.enterred_invalid_password"), SETTINGS_OPTIONS, nil)
			return
		}

		if err := models.DeleteRepository(ctx.Repo.Owner.Id, ctx.Repo.Repository.Id, ctx.Repo.Owner.Name); err != nil {
			ctx.Handle(500, "DeleteRepository", err)
			return
		}
		log.Trace("Repository deleted: %s/%s", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)
		if ctx.Repo.Owner.IsOrganization() {
			ctx.Redirect("/org/" + ctx.Repo.Owner.Name + "/dashboard")
		} else {
			ctx.Redirect("/")
		}
	}
}

func Collaboration(ctx *middleware.Context) {
	repoLink := strings.TrimPrefix(ctx.Repo.RepoLink, "/")
	ctx.Data["IsRepoToolbarCollaboration"] = true
	ctx.Data["Title"] = repoLink + " - collaboration"

	// Delete collaborator.
	remove := strings.ToLower(ctx.Query("remove"))
	if len(remove) > 0 && remove != ctx.Repo.Owner.LowerName {
		if err := models.DeleteAccess(&models.Access{UserName: remove, RepoName: repoLink}); err != nil {
			ctx.Handle(500, "setting.Collaboration(DeleteAccess)", err)
			return
		}
		ctx.Flash.Success("Collaborator has been removed.")
		ctx.Redirect(ctx.Repo.RepoLink + "/settings/collaboration")
		return
	}

	names, err := models.GetCollaboratorNames(repoLink)
	if err != nil {
		ctx.Handle(500, "setting.Collaboration(GetCollaborators)", err)
		return
	}

	us := make([]*models.User, len(names))
	for i, name := range names {
		us[i], err = models.GetUserByName(name)
		if err != nil {
			ctx.Handle(500, "setting.Collaboration(GetUserByName)", err)
			return
		}
	}

	ctx.Data["Collaborators"] = us
	ctx.HTML(200, COLLABORATION)
}

func CollaborationPost(ctx *middleware.Context) {
	repoLink := strings.TrimPrefix(ctx.Repo.RepoLink, "/")
	name := strings.ToLower(ctx.Query("collaborator"))
	if len(name) == 0 || ctx.Repo.Owner.LowerName == name {
		ctx.Redirect(ctx.Req.RequestURI)
		return
	}
	has, err := models.HasAccess(name, repoLink, models.WRITABLE)
	if err != nil {
		ctx.Handle(500, "setting.CollaborationPost(HasAccess)", err)
		return
	} else if has {
		ctx.Redirect(ctx.Req.RequestURI)
		return
	}

	u, err := models.GetUserByName(name)
	if err != nil {
		if err == models.ErrUserNotExist {
			ctx.Flash.Error("Given user does not exist.")
			ctx.Redirect(ctx.Req.RequestURI)
		} else {
			ctx.Handle(500, "setting.CollaborationPost(GetUserByName)", err)
		}
		return
	}

	if err = models.AddAccess(&models.Access{UserName: name, RepoName: repoLink,
		Mode: models.WRITABLE}); err != nil {
		ctx.Handle(500, "setting.CollaborationPost(AddAccess)", err)
		return
	}

	if setting.Service.EnableNotifyMail {
		if err = mailer.SendCollaboratorMail(ctx.Render, u, ctx.User, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "setting.CollaborationPost(SendCollaboratorMail)", err)
			return
		}
	}

	ctx.Flash.Success("New collaborator has been added.")
	ctx.Redirect(ctx.Req.RequestURI)
}

func WebHooks(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Webhooks"

	// Delete webhook.
	remove := com.StrTo(ctx.Query("remove")).MustInt64()
	if remove > 0 {
		if err := models.DeleteWebhook(remove); err != nil {
			ctx.Handle(500, "setting.WebHooks(DeleteWebhook)", err)
			return
		}
		ctx.Flash.Success("Webhook has been removed.")
		ctx.Redirect(ctx.Repo.RepoLink + "/settings/hooks")
		return
	}

	ws, err := models.GetWebhooksByRepoId(ctx.Repo.Repository.Id)
	if err != nil {
		ctx.Handle(500, "setting.WebHooks(GetWebhooksByRepoId)", err)
		return
	}

	ctx.Data["Webhooks"] = ws
	ctx.HTML(200, HOOKS)
}

func WebHooksAdd(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Add Webhook"
	ctx.HTML(200, HOOK_ADD)
}

func WebHooksAddPost(ctx *middleware.Context, form auth.NewWebhookForm) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Add Webhook"

	if ctx.HasError() {
		ctx.HTML(200, HOOK_ADD)
		return
	}

	ct := models.JSON
	if form.ContentType == "2" {
		ct = models.FORM
	}

	w := &models.Webhook{
		RepoId:      ctx.Repo.Repository.Id,
		Url:         form.Url,
		ContentType: ct,
		Secret:      form.Secret,
		HookEvent: &models.HookEvent{
			PushOnly: form.PushOnly,
		},
		IsActive: form.Active,
	}
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "setting.WebHooksAddPost(UpdateEvent)", err)
		return
	} else if err := models.CreateWebhook(w); err != nil {
		ctx.Handle(500, "setting.WebHooksAddPost(CreateWebhook)", err)
		return
	}

	ctx.Flash.Success("New webhook has been added.")
	ctx.Redirect(ctx.Repo.RepoLink + "/settings/hooks")
}

func WebHooksEdit(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Webhook"

	hookId := com.StrTo(ctx.Params(":id")).MustInt64()
	if hookId == 0 {
		ctx.Handle(404, "setting.WebHooksEdit", nil)
		return
	}

	w, err := models.GetWebhookById(hookId)
	if err != nil {
		if err == models.ErrWebhookNotExist {
			ctx.Handle(404, "setting.WebHooksEdit(GetWebhookById)", nil)
		} else {
			ctx.Handle(500, "setting.WebHooksEdit(GetWebhookById)", err)
		}
		return
	}

	w.GetEvent()
	ctx.Data["Webhook"] = w
	ctx.HTML(200, HOOK_EDIT)
}

func WebHooksEditPost(ctx *middleware.Context, form auth.NewWebhookForm) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Webhook"

	hookId := com.StrTo(ctx.Params(":id")).MustInt64()
	if hookId == 0 {
		ctx.Handle(404, "setting.WebHooksEditPost", nil)
		return
	}

	w, err := models.GetWebhookById(hookId)
	if err != nil {
		if err == models.ErrWebhookNotExist {
			ctx.Handle(404, "GetWebhookById", nil)
		} else {
			ctx.Handle(500, "GetWebhookById", err)
		}
		return
	}

	if ctx.HasError() {
		ctx.HTML(200, HOOK_EDIT)
		return
	}

	ct := models.JSON
	if form.ContentType == "2" {
		ct = models.FORM
	}

	w.Url = form.Url
	w.ContentType = ct
	w.Secret = form.Secret
	w.HookEvent = &models.HookEvent{
		PushOnly: form.PushOnly,
	}
	w.IsActive = form.Active
	if err := w.UpdateEvent(); err != nil {
		ctx.Handle(500, "UpdateEvent", err)
		return
	} else if err := models.UpdateWebhook(w); err != nil {
		ctx.Handle(500, "WebHooksEditPost", err)
		return
	}

	ctx.Flash.Success("Webhook has been updated.")
	ctx.Redirect(fmt.Sprintf("%s/settings/hooks/%d", ctx.Repo.RepoLink, hookId))
}
