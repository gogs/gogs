// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"strings"

	jsoniter "github.com/json-iterator/go"
	"github.com/unknwon/com"

	git "github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/setting"
)

const (
	WEBHOOKS        = "repo/settings/webhook/base"
	WEBHOOK_NEW     = "repo/settings/webhook/new"
	ORG_WEBHOOK_NEW = "org/settings/webhook_new"
)

func Webhooks(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.hooks")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["BaseLink"] = c.Repo.RepoLink
	c.Data["Description"] = c.Tr("repo.settings.hooks_desc", "https://github.com/gogs/docs-api/blob/master/Repositories/Webhooks.md")
	c.Data["Types"] = setting.Webhook.Types

	ws, err := db.GetWebhooksByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Handle(500, "GetWebhooksByRepoID", err)
		return
	}
	c.Data["Webhooks"] = ws

	c.HTML(200, WEBHOOKS)
}

type OrgRepoCtx struct {
	OrgID       int64
	RepoID      int64
	Link        string
	NewTemplate string
}

// getOrgRepoCtx determines whether this is a repo context or organization context.
func getOrgRepoCtx(c *context.Context) (*OrgRepoCtx, error) {
	if len(c.Repo.RepoLink) > 0 {
		c.Data["PageIsRepositoryContext"] = true
		return &OrgRepoCtx{
			RepoID:      c.Repo.Repository.ID,
			Link:        c.Repo.RepoLink,
			NewTemplate: WEBHOOK_NEW,
		}, nil
	}

	if len(c.Org.OrgLink) > 0 {
		c.Data["PageIsOrganizationContext"] = true
		return &OrgRepoCtx{
			OrgID:       c.Org.Organization.ID,
			Link:        c.Org.OrgLink,
			NewTemplate: ORG_WEBHOOK_NEW,
		}, nil
	}

	return nil, errors.New("Unable to set OrgRepo context")
}

func checkHookType(c *context.Context) string {
	hookType := strings.ToLower(c.Params(":type"))
	if !com.IsSliceContainsStr(setting.Webhook.Types, hookType) {
		c.Handle(404, "checkHookType", nil)
		return ""
	}
	return hookType
}

func WebhooksNew(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.add_webhook")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksNew"] = true
	c.Data["Webhook"] = db.Webhook{HookEvent: &db.HookEvent{}}

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return
	}

	c.Data["HookType"] = checkHookType(c)
	if c.Written() {
		return
	}
	c.Data["BaseLink"] = orCtx.Link

	c.HTML(200, orCtx.NewTemplate)
}

func ParseHookEvent(f form.Webhook) *db.HookEvent {
	return &db.HookEvent{
		PushOnly:       f.PushOnly(),
		SendEverything: f.SendEverything(),
		ChooseEvents:   f.ChooseEvents(),
		HookEvents: db.HookEvents{
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

func WebHooksNewPost(c *context.Context, f form.NewWebhook) {
	c.Data["Title"] = c.Tr("repo.settings.add_webhook")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksNew"] = true
	c.Data["Webhook"] = db.Webhook{HookEvent: &db.HookEvent{}}
	c.Data["HookType"] = "gogs"

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return
	}
	c.Data["BaseLink"] = orCtx.Link

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	contentType := db.JSON
	if db.HookContentType(f.ContentType) == db.FORM {
		contentType = db.FORM
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  contentType,
		Secret:       f.Secret,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.GOGS,
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.CreateWebhook(w); err != nil {
		c.Handle(500, "CreateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

func SlackHooksNewPost(c *context.Context, f form.NewSlackHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksNew"] = true
	c.Data["Webhook"] = db.Webhook{HookEvent: &db.HookEvent{}}

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return
	}

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Handle(500, "Marshal", err)
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.SLACK,
		Meta:         string(meta),
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.CreateWebhook(w); err != nil {
		c.Handle(500, "CreateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

// FIXME: merge logic to Slack
func DiscordHooksNewPost(c *context.Context, f form.NewDiscordHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksNew"] = true
	c.Data["Webhook"] = db.Webhook{HookEvent: &db.HookEvent{}}

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return
	}

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Handle(500, "Marshal", err)
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.DISCORD,
		Meta:         string(meta),
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.CreateWebhook(w); err != nil {
		c.Handle(500, "CreateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

func DingtalkHooksNewPost(c *context.Context, f form.NewDingtalkHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksNew"] = true
	c.Data["Webhook"] = db.Webhook{HookEvent: &db.HookEvent{}}

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return
	}

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    ParseHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.DINGTALK,
		OrgID:        orCtx.OrgID,
	}
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.CreateWebhook(w); err != nil {
		c.Handle(500, "CreateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

func checkWebhook(c *context.Context) (*OrgRepoCtx, *db.Webhook) {
	c.Data["RequireHighlightJS"] = true

	orCtx, err := getOrgRepoCtx(c)
	if err != nil {
		c.Handle(500, "getOrgRepoCtx", err)
		return nil, nil
	}
	c.Data["BaseLink"] = orCtx.Link

	var w *db.Webhook
	if orCtx.RepoID > 0 {
		w, err = db.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	} else {
		w, err = db.GetWebhookByOrgID(c.Org.Organization.ID, c.ParamsInt64(":id"))
	}
	if err != nil {
		c.NotFoundOrServerError("GetWebhookOfRepoByID/GetWebhookByOrgID", errors.IsWebhookNotExist, err)
		return nil, nil
	}

	switch w.HookTaskType {
	case db.SLACK:
		c.Data["SlackHook"] = w.GetSlackHook()
		c.Data["HookType"] = "slack"
	case db.DISCORD:
		c.Data["SlackHook"] = w.GetSlackHook()
		c.Data["HookType"] = "discord"
	case db.DINGTALK:
		c.Data["HookType"] = "dingtalk"
	default:
		c.Data["HookType"] = "gogs"
	}

	c.Data["History"], err = w.History(1)
	if err != nil {
		c.Handle(500, "History", err)
	}
	return orCtx, w
}

func WebHooksEdit(c *context.Context) {
	c.Data["Title"] = c.Tr("repo.settings.update_webhook")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	c.HTML(200, orCtx.NewTemplate)
}

func WebHooksEditPost(c *context.Context, f form.NewWebhook) {
	c.Data["Title"] = c.Tr("repo.settings.update_webhook")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	contentType := db.JSON
	if db.HookContentType(f.ContentType) == db.FORM {
		contentType = db.FORM
	}

	w.URL = f.PayloadURL
	w.ContentType = contentType
	w.Secret = f.Secret
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.UpdateWebhook(w); err != nil {
		c.Handle(500, "WebHooksEditPost", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func SlackHooksEditPost(c *context.Context, f form.NewSlackHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Handle(500, "Marshal", err)
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.UpdateWebhook(w); err != nil {
		c.Handle(500, "UpdateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

// FIXME: merge logic to Slack
func DiscordHooksEditPost(c *context.Context, f form.NewDiscordHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Handle(500, "Marshal", err)
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.UpdateWebhook(w); err != nil {
		c.Handle(500, "UpdateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func DingtalkHooksEditPost(c *context.Context, f form.NewDingtalkHook) {
	c.Data["Title"] = c.Tr("repo.settings")
	c.Data["PageIsSettingsHooks"] = true
	c.Data["PageIsSettingsHooksEdit"] = true

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.HTML(200, orCtx.NewTemplate)
		return
	}

	w.URL = f.PayloadURL
	w.HookEvent = ParseHookEvent(f.Webhook)
	w.IsActive = f.Active
	if err := w.UpdateEvent(); err != nil {
		c.Handle(500, "UpdateEvent", err)
		return
	} else if err := db.UpdateWebhook(w); err != nil {
		c.Handle(500, "UpdateWebhook", err)
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func TestWebhook(c *context.Context) {
	var authorUsername, committerUsername string

	// Grab latest commit or fake one if it's empty repository.
	commit := c.Repo.Commit
	if commit == nil {
		ghost := db.NewGhostUser()
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
		author, err := db.GetUserByEmail(commit.Author.Email)
		if err == nil {
			authorUsername = author.Name
		} else if !errors.IsUserNotExist(err) {
			c.Handle(500, "GetUserByEmail.(author)", err)
			return
		}

		committer, err := db.GetUserByEmail(commit.Committer.Email)
		if err == nil {
			committerUsername = committer.Name
		} else if !errors.IsUserNotExist(err) {
			c.Handle(500, "GetUserByEmail.(committer)", err)
			return
		}
	}

	fileStatus, err := commit.FileStatus()
	if err != nil {
		c.Handle(500, "FileStatus", err)
		return
	}

	apiUser := c.User.APIFormat()
	p := &api.PushPayload{
		Ref:    git.BRANCH_PREFIX + c.Repo.Repository.DefaultBranch,
		Before: commit.ID.String(),
		After:  commit.ID.String(),
		Commits: []*api.PayloadCommit{
			{
				ID:      commit.ID.String(),
				Message: commit.Message(),
				URL:     c.Repo.Repository.HTMLURL() + "/commit/" + commit.ID.String(),
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
		Repo:   c.Repo.Repository.APIFormat(nil),
		Pusher: apiUser,
		Sender: apiUser,
	}
	if err := db.TestWebhook(c.Repo.Repository, db.HOOK_EVENT_PUSH, p, c.ParamsInt64("id")); err != nil {
		c.Handle(500, "TestWebhook", err)
	} else {
		c.Flash.Info(c.Tr("repo.settings.webhook.test_delivery_success"))
		c.Status(200)
	}
}

func RedeliveryWebhook(c *context.Context) {
	webhook, err := db.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrServerError("GetWebhookOfRepoByID/GetWebhookByOrgID", errors.IsWebhookNotExist, err)
		return
	}

	hookTask, err := db.GetHookTaskOfWebhookByUUID(webhook.ID, c.Query("uuid"))
	if err != nil {
		c.NotFoundOrServerError("GetHookTaskOfWebhookByUUID/GetWebhookByOrgID", errors.IsHookTaskNotExist, err)
		return
	}

	hookTask.IsDelivered = false
	if err = db.UpdateHookTask(hookTask); err != nil {
		c.Handle(500, "UpdateHookTask", err)
	} else {
		go db.HookQueue.Add(c.Repo.Repository.ID)
		c.Flash.Info(c.Tr("repo.settings.webhook.redelivery_success", hookTask.UUID))
		c.Status(200)
	}
}

func DeleteWebhook(c *context.Context) {
	if err := db.DeleteWebhookOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteWebhookByRepoID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.webhook_deletion_success"))
	}

	c.JSON(200, map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/hooks",
	})
}
