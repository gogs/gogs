// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Unknwon/com"

	git "github.com/gogits/git-module"
	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/context"
	"github.com/gogits/gogs/pkg/form"
	"github.com/gogits/gogs/pkg/setting"
)

const (
	WEBHOOKS        = "repo/settings/webhook/base"
	WEBHOOK_NEW     = "repo/settings/webhook/new"
	ORG_WEBHOOK_NEW = "org/settings/webhook_new"
)

func Webhooks(ctx *context.Context) {
	ctx.Data["Title"] = ctx.Tr("repo.settings.hooks")
	ctx.Data["PageIsSettingsHooks"] = true
	ctx.Data["BaseLink"] = ctx.Repo.RepoLink
	ctx.Data["Description"] = ctx.Tr("repo.settings.hooks_desc", "https://github.com/gogits/go-gogs-client/wiki/Repositories-Webhooks")
	ctx.Data["Types"] = setting.Webhook.Types

	ws, err := models.GetWebhooksByRepoID(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.Handle(500, "GetWebhooksByRepoID", err)
		return
	}
	ctx.Data["Webhooks"] = ws

	ctx.HTML(200, WEBHOOKS)
}

type OrgRepoCtx struct {
	OrgID       int64
	RepoID      int64
	Link        string
	NewTemplate string
}

// getOrgRepoCtx determines whether this is a repo context or organization context.
func getOrgRepoCtx(ctx *context.Context) (*OrgRepoCtx, error) {
	if len(ctx.Repo.RepoLink) > 0 {
		ctx.Data["PageIsRepositoryContext"] = true
		return &OrgRepoCtx{
			RepoID:      ctx.Repo.Repository.ID,
			Link:        ctx.Repo.RepoLink,
			NewTemplate: WEBHOOK_NEW,
		}, nil
	}

	if len(ctx.Org.OrgLink) > 0 {
		ctx.Data["PageIsOrganizationContext"] = true
		return &OrgRepoCtx{
			OrgID:       ctx.Org.Organization.ID,
			Link:        ctx.Org.OrgLink,
			NewTemplate: ORG_WEBHOOK_NEW,
		}, nil
	}

	return nil, errors.New("Unable to set OrgRepo context")
}

func checkHookType(ctx *context.Context) string {
	hookType := strings.ToLower(ctx.Params(":type"))
	if !com.IsSliceContainsStr(setting.Webhook.Types, hookType) {
		ctx.Handle(404, "checkHookType", nil)
		return ""
	}
	return hookType
}

func WebhooksNew(ctx *context.Context) {
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

func ParseHookEvent(f form.Webhook) *models.HookEvent {
	return &models.HookEvent{
		PushOnly:       f.PushOnly(),
		SendEverything: f.SendEverything(),
		ChooseEvents:   f.ChooseEvents(),
		HookEvents: models.HookEvents{
			Create:       f.Create,
			Delete:       f.Delete,
			Fork:         f.Fork,
			Push:         f.Push,
			Issues:       f.Issues,
			IssueComment: f.IssueComment,
			PullRequest:  f.PullRequest,
			Release:      f.Release,
		},
	}
}

func WebHooksNewPost(ctx *context.Context, f form.NewWebhook) {
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
	if models.HookContentType(f.ContentType) == models.FORM {
		contentType = models.FORM
	}

	w := &models.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  contentType,
		Secret:       f.Secret,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
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

func SlackHooksNewPost(ctx *context.Context, f form.NewSlackHook) {
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
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w := &models.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  models.JSON,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
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

// FIXME: merge logic to Slack
func DiscordHooksNewPost(ctx *context.Context, f form.NewDiscordHook) {
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
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w := &models.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  models.JSON,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: models.DISCORD,
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

func checkWebhook(ctx *context.Context) (*OrgRepoCtx, *models.Webhook) {
	ctx.Data["RequireHighlightJS"] = true

	orCtx, err := getOrgRepoCtx(ctx)
	if err != nil {
		ctx.Handle(500, "getOrgRepoCtx", err)
		return nil, nil
	}
	ctx.Data["BaseLink"] = orCtx.Link

	var w *models.Webhook
	if orCtx.RepoID > 0 {
		w, err = models.GetWebhookOfRepoByID(ctx.Repo.Repository.ID, ctx.ParamsInt64(":id"))
	} else {
		w, err = models.GetWebhookByOrgID(ctx.Org.Organization.ID, ctx.ParamsInt64(":id"))
	}
	if err != nil {
		ctx.NotFoundOrServerError("GetWebhookOfRepoByID/GetWebhookByOrgID", errors.IsWebhookNotExist, err)
		return nil, nil
	}

	switch w.HookTaskType {
	case models.SLACK:
		ctx.Data["SlackHook"] = w.GetSlackHook()
		ctx.Data["HookType"] = "slack"
	case models.DISCORD:
		ctx.Data["SlackHook"] = w.GetSlackHook()
		ctx.Data["HookType"] = "discord"
	default:
		ctx.Data["HookType"] = "gogs"
	}

	ctx.Data["History"], err = w.History(1)
	if err != nil {
		ctx.Handle(500, "History", err)
	}
	return orCtx, w
}

func WebHooksEdit(ctx *context.Context) {
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

func WebHooksEditPost(ctx *context.Context, f form.NewWebhook) {
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
	if models.HookContentType(f.ContentType) == models.FORM {
		contentType = models.FORM
	}

	w.URL = f.PayloadURL
	w.ContentType = contentType
	w.Secret = f.Secret
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
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

func SlackHooksEditPost(ctx *context.Context, f form.NewSlackHook) {
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
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
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

// FIXME: merge logic to Slack
func DiscordHooksEditPost(ctx *context.Context, f form.NewDiscordHook) {
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
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		ctx.Handle(500, "Marshal", err)
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
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

func TestWebhook(ctx *context.Context) {
	var authorUsername, committerUsername string

	// Grab latest commit or fake one if it's empty repository.
	commit := ctx.Repo.Commit
	if commit == nil {
		ghost := models.NewGhostUser()
		commit = &git.Commit{
			ID:            git.MustIDFromString(git.EMPTY_SHA),
			Author:        ghost.NewGitSig(),
			Committer:     ghost.NewGitSig(),
			CommitMessage: "This is a fake commit",
		}
		authorUsername = ghost.Name
		committerUsername = ghost.Name
	} else {
		// Try to match email with a real user.
		author, err := models.GetUserByEmail(commit.Author.Email)
		if err == nil {
			authorUsername = author.Name
		} else if !errors.IsUserNotExist(err) {
			ctx.Handle(500, "GetUserByEmail.(author)", err)
			return
		}

		committer, err := models.GetUserByEmail(commit.Committer.Email)
		if err == nil {
			committerUsername = committer.Name
		} else if !errors.IsUserNotExist(err) {
			ctx.Handle(500, "GetUserByEmail.(committer)", err)
			return
		}
	}

	fileStatus, err := commit.FileStatus()
	if err != nil {
		ctx.Handle(500, "FileStatus", err)
		return
	}

	apiUser := ctx.User.APIFormat()
	p := &api.PushPayload{
		Ref:    git.BRANCH_PREFIX + ctx.Repo.Repository.DefaultBranch,
		Before: commit.ID.String(),
		After:  commit.ID.String(),
		Commits: []*api.PayloadCommit{
			{
				ID:      commit.ID.String(),
				Message: commit.Message(),
				URL:     ctx.Repo.Repository.HTMLURL() + "/commit/" + commit.ID.String(),
				Author: &api.PayloadUser{
					Name:     commit.Author.Name,
					Email:    commit.Author.Email,
					UserName: authorUsername,
				},
				Committer: &api.PayloadUser{
					Name:     commit.Committer.Name,
					Email:    commit.Committer.Email,
					UserName: committerUsername,
				},
				Added:    fileStatus.Added,
				Removed:  fileStatus.Removed,
				Modified: fileStatus.Modified,
			},
		},
		Repo:   ctx.Repo.Repository.APIFormat(nil),
		Pusher: apiUser,
		Sender: apiUser,
	}
	if err := models.TestWebhook(ctx.Repo.Repository, models.HOOK_EVENT_PUSH, p, ctx.ParamsInt64("id")); err != nil {
		ctx.Handle(500, "TestWebhook", err)
	} else {
		ctx.Flash.Info(ctx.Tr("repo.settings.webhook.test_delivery_success"))
		ctx.Status(200)
	}
}

func RedeliveryWebhook(ctx *context.Context) {
	webhook, err := models.GetWebhookOfRepoByID(ctx.Repo.Repository.ID, ctx.ParamsInt64(":id"))
	if err != nil {
		ctx.NotFoundOrServerError("GetWebhookOfRepoByID/GetWebhookByOrgID", errors.IsWebhookNotExist, err)
		return
	}

	hookTask, err := models.GetHookTaskOfWebhookByUUID(webhook.ID, ctx.Query("uuid"))
	if err != nil {
		ctx.NotFoundOrServerError("GetHookTaskOfWebhookByUUID/GetWebhookByOrgID", errors.IsHookTaskNotExist, err)
		return
	}

	hookTask.IsDelivered = false
	if err = models.UpdateHookTask(hookTask); err != nil {
		ctx.Handle(500, "UpdateHookTask", err)
	} else {
		go models.HookQueue.Add(ctx.Repo.Repository.ID)
		ctx.Flash.Info(ctx.Tr("repo.settings.webhook.redelivery_success", hookTask.UUID))
		ctx.Status(200)
	}
}

func DeleteWebhook(ctx *context.Context) {
	if err := models.DeleteWebhookOfRepoByID(ctx.Repo.Repository.ID, ctx.QueryInt64("id")); err != nil {
		ctx.Flash.Error("DeleteWebhookByRepoID: " + err.Error())
	} else {
		ctx.Flash.Success(ctx.Tr("repo.settings.webhook_deletion_success"))
	}

	ctx.JSON(200, map[string]interface{}{
		"redirect": ctx.Repo.RepoLink + "/settings/hooks",
	})
}
