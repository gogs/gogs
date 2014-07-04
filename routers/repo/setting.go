// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"
	"time"

	"github.com/go-martini/martini"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/auth"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/mailer"
	"github.com/gogits/gogs/modules/middleware"
	"github.com/gogits/gogs/modules/setting"
)

const (
	SETTING       base.TplName = "repo/setting"
	COLLABORATION base.TplName = "repo/collaboration"

	HOOKS     base.TplName = "repo/hooks"
	HOOK_ADD  base.TplName = "repo/hook_add"
	HOOK_EDIT base.TplName = "repo/hook_edit"
)

func Setting(ctx *middleware.Context) {
	ctx.Data["IsRepoToolbarSetting"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - settings"
	ctx.HTML(200, SETTING)
}

func SettingPost(ctx *middleware.Context, form auth.RepoSettingForm) {
	ctx.Data["IsRepoToolbarSetting"] = true

	switch ctx.Query("action") {
	case "update":
		if ctx.HasError() {
			ctx.HTML(200, SETTING)
			return
		}

		newRepoName := form.RepoName
		// Check if repository name has been changed.
		if ctx.Repo.Repository.Name != newRepoName {
			isExist, err := models.IsRepositoryExist(ctx.Repo.Owner, newRepoName)
			if err != nil {
				ctx.Handle(500, "setting.SettingPost(update: check existence)", err)
				return
			} else if isExist {
				ctx.RenderWithErr("Repository name has been taken in your repositories.", SETTING, nil)
				return
			} else if err = models.ChangeRepositoryName(ctx.Repo.Owner.Name, ctx.Repo.Repository.Name, newRepoName); err != nil {
				ctx.Handle(500, "setting.SettingPost(change repository name)", err)
				return
			}
			log.Trace("%s Repository name changed: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newRepoName)

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
			ctx.Handle(404, "setting.SettingPost(update)", err)
			return
		}
		log.Trace("%s Repository updated: %s/%s", ctx.Req.RequestURI, ctx.Repo.Owner.Name, ctx.Repo.Repository.Name)

		if ctx.Repo.Repository.IsMirror {
			if form.Interval > 0 {
				ctx.Repo.Mirror.Interval = form.Interval
				ctx.Repo.Mirror.NextUpdate = time.Now().Add(time.Duration(form.Interval) * time.Hour)
				if err := models.UpdateMirror(ctx.Repo.Mirror); err != nil {
					log.Error("setting.SettingPost(UpdateMirror): %v", err)
				}
			}
		}

		ctx.Flash.Success("Repository options has been successfully updated.")
		ctx.Redirect(fmt.Sprintf("/%s/%s/settings", ctx.Repo.Owner.Name, ctx.Repo.Repository.Name))
	case "transfer":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", SETTING, nil)
			return
		} else if ctx.Repo.Repository.IsMirror {
			ctx.Error(404)
			return
		}

		newOwner := ctx.Query("owner")
		// Check if new owner exists.
		isExist, err := models.IsUserExist(newOwner)
		if err != nil {
			ctx.Handle(500, "setting.SettingPost(transfer: check existence)", err)
			return
		} else if !isExist {
			ctx.RenderWithErr("Please make sure you entered owner name is correct.", SETTING, nil)
			return
		} else if err = models.TransferOwnership(ctx.Repo.Owner, newOwner, ctx.Repo.Repository); err != nil {
			ctx.Handle(500, "setting.SettingPost(transfer repository)", err)
			return
		}
		log.Trace("%s Repository transfered: %s/%s -> %s", ctx.Req.RequestURI, ctx.User.Name, ctx.Repo.Repository.Name, newOwner)

		ctx.Redirect("/")
	case "delete":
		if len(ctx.Repo.Repository.Name) == 0 || ctx.Repo.Repository.Name != ctx.Query("repository") {
			ctx.RenderWithErr("Please make sure you entered repository name is correct.", SETTING, nil)
			return
		}

		if ctx.Repo.Owner.IsOrganization() &&
			!ctx.Repo.Owner.IsOrgOwner(ctx.User.Id) {
			ctx.Error(403)
			return
		}

		if err := models.DeleteRepository(ctx.Repo.Owner.Id, ctx.Repo.Repository.Id, ctx.Repo.Owner.Name); err != nil {
			ctx.Handle(500, "setting.Delete(DeleteRepository)", err)
			return
		}
		log.Trace("%s Repository deleted: %s/%s", ctx.Req.RequestURI, ctx.Repo.Owner.LowerName, ctx.Repo.Repository.LowerName)

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
	remove, _ := base.StrTo(ctx.Query("remove")).Int64()
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

func WebHooksEdit(ctx *middleware.Context, params martini.Params) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Webhook"

	hookId, _ := base.StrTo(params["id"]).Int64()
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

func WebHooksEditPost(ctx *middleware.Context, params martini.Params, form auth.NewWebhookForm) {
	ctx.Data["IsRepoToolbarWebHooks"] = true
	ctx.Data["Title"] = strings.TrimPrefix(ctx.Repo.RepoLink, "/") + " - Webhook"

	hookId, _ := base.StrTo(params["id"]).Int64()
	if hookId == 0 {
		ctx.Handle(404, "setting.WebHooksEditPost", nil)
		return
	}

	w, err := models.GetWebhookById(hookId)
	if err != nil {
		if err == models.ErrWebhookNotExist {
			ctx.Handle(404, "setting.WebHooksEditPost(GetWebhookById)", nil)
		} else {
			ctx.Handle(500, "setting.WebHooksEditPost(GetWebhookById)", err)
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
		ctx.Handle(500, "setting.WebHooksEditPost(UpdateEvent)", err)
		return
	} else if err := models.UpdateWebhook(w); err != nil {
		ctx.Handle(500, "setting.WebHooksEditPost(WebHooksEditPost)", err)
		return
	}

	ctx.Flash.Success("Webhook has been updated.")
	ctx.Redirect(fmt.Sprintf("%s/settings/hooks/%d", ctx.Repo.RepoLink, hookId))
}
