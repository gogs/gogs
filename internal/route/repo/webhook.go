// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"
	jsoniter "github.com/json-iterator/go"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/db"
	"gogs.io/gogs/internal/db/errors"
	"gogs.io/gogs/internal/form"
)

const (
	tmplRepoSettingsWebhooks   = "repo/settings/webhook/base"
	tmplRepoSettingsWebhookNew = "repo/settings/webhook/new"
	tmplOrgSettingsWebhookNew  = "org/settings/webhook_new"
)

func Webhooks(c *context.Context) {
	c.Title("repo.settings.hooks")
	c.PageIs("SettingsHooks")
	c.Data["Description"] = c.Tr("repo.settings.hooks_desc")
	c.Data["Types"] = conf.Webhook.Types

	ws, err := db.GetWebhooksByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Error(err, "get webhooks by repository ID")
		return
	}
	c.Data["Webhooks"] = ws

	c.Success(tmplRepoSettingsWebhooks)
}

func WebhooksNew(c *context.Context) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return
	}

	allowed := false
	hookType := strings.ToLower(c.Params(":type"))
	for _, typ := range conf.Webhook.Types {
		if hookType == typ {
			allowed = true
			c.Data["HookType"] = typ
			break
		}
	}
	if !allowed {
		c.NotFound()
		return
	}

	c.Success(orCtx.Tmpl)
}

var localHostnames = []string{
	"localhost",
	"127.0.0.1",
	"::1",
	"0:0:0:0:0:0:0:1",
}

// isLocalHostname returns true if given hostname is a well-known local address.
func isLocalHostname(hostname string) bool {
	for _, local := range localHostnames {
		if hostname == local {
			return true
		}
	}
	return false
}

func validateAndCreateWebhook(c *context.Context, orCtx *orgRepoContext, w *db.Webhook) {
	c.Data["Webhook"] = w

	if !c.User.IsAdmin {
		// 🚨 SECURITY: Local addresses must not be allowed by non-admins to prevent SSRF,
		// see https://github.com/gogs/gogs/issues/5366 for details.
		payloadURL, err := url.Parse(w.URL)
		if err != nil {
			c.FormErr("PayloadURL")
			c.RenderWithErr(c.Tr("repo.settings.webhook.err_cannot_parse_payload_url", err), orCtx.Tmpl, nil)
			return
		}

		if isLocalHostname(payloadURL.Hostname()) {
			c.FormErr("PayloadURL")
			c.RenderWithErr(c.Tr("repo.settings.webhook.err_cannot_use_local_addresses"), orCtx.Tmpl, nil)
			return
		}
	}

	if err := w.UpdateEvent(); err != nil {
		c.Error(err, "update event")
		return
	} else if err := db.CreateWebhook(w); err != nil {
		c.Error(err, "create webhook")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

type orgRepoContext struct {
	OrgID  int64
	RepoID int64
	Link   string
	Tmpl   string
}

// getOrgRepoContext determines whether this is a repo context or organization context.
func getOrgRepoContext(c *context.Context) (*orgRepoContext, error) {
	if len(c.Repo.RepoLink) > 0 {
		c.PageIs("RepositoryContext")
		return &orgRepoContext{
			RepoID: c.Repo.Repository.ID,
			Link:   c.Repo.RepoLink,
			Tmpl:   tmplRepoSettingsWebhookNew,
		}, nil
	}

	if len(c.Org.OrgLink) > 0 {
		c.PageIs("OrganizationContext")
		return &orgRepoContext{
			OrgID: c.Org.Organization.ID,
			Link:  c.Org.OrgLink,
			Tmpl:  tmplOrgSettingsWebhookNew,
		}, nil
	}

	return nil, errors.New("unable to determine context")
}

func toHookEvent(f form.Webhook) *db.HookEvent {
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

func WebhooksNewPost(c *context.Context, f form.NewWebhook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "gogs"

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return
	}

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	contentType := db.JSON
	if db.HookContentType(f.ContentType) == db.FORM {
		contentType = db.FORM
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		OrgID:        orCtx.OrgID,
		URL:          f.PayloadURL,
		ContentType:  contentType,
		Secret:       f.Secret,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.GOGS,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksSlackNewPost(c *context.Context, f form.NewSlackHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "slack"

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return
	}

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}
	meta := &db.SlackMeta{
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	}
	c.Data["SlackMeta"] = meta

	p, err := jsoniter.Marshal(meta)
	if err != nil {
		c.Error(err, "marshal JSON")
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.SLACK,
		Meta:         string(p),
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksDiscordNewPost(c *context.Context, f form.NewDiscordHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "discord"

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return
	}

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}
	meta := &db.SlackMeta{
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	}
	c.Data["SlackMeta"] = meta

	p, err := jsoniter.Marshal(meta)
	if err != nil {
		c.Error(err, "marshal JSON")
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.DISCORD,
		Meta:         string(p),
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksDingtalkNewPost(c *context.Context, f form.NewDingtalkHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "dingtalk"

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return
	}

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	w := &db.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  db.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: db.DINGTALK,
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func checkWebhook(c *context.Context) (*orgRepoContext, *db.Webhook) {
	c.RequireHighlightJS()

	orCtx, err := getOrgRepoContext(c)
	if err != nil {
		c.Error(err, "get organization or repository context")
		return nil, nil
	}

	var w *db.Webhook
	if orCtx.RepoID > 0 {
		w, err = db.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	} else {
		w, err = db.GetWebhookByOrgID(c.Org.Organization.ID, c.ParamsInt64(":id"))
	}
	if err != nil {
		c.NotFoundOrError(err, "get webhook")
		return nil, nil
	}

	switch w.HookTaskType {
	case db.SLACK:
		c.Data["SlackMeta"] = w.SlackMeta()
		c.Data["HookType"] = "slack"
	case db.DISCORD:
		c.Data["SlackMeta"] = w.SlackMeta()
		c.Data["HookType"] = "discord"
	case db.DINGTALK:
		c.Data["HookType"] = "dingtalk"
	default:
		c.Data["HookType"] = "gogs"
	}
	c.Data["FormURL"] = fmt.Sprintf("%s/settings/hooks/%s/%d", orCtx.Link, c.Data["HookType"], w.ID)
	c.Data["DeleteURL"] = fmt.Sprintf("%s/settings/hooks/delete", orCtx.Link)

	c.Data["History"], err = w.History(1)
	if err != nil {
		c.Error(err, "get history")
		return nil, nil
	}
	return orCtx, w
}

func WebhooksEdit(c *context.Context) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	c.Success(orCtx.Tmpl)
}

func validateAndUpdateWebhook(c *context.Context, orCtx *orgRepoContext, w *db.Webhook) {
	c.Data["Webhook"] = w

	if !c.User.IsAdmin {
		// 🚨 SECURITY: Well-known address to localhost must not be allowed to
		// prevent SSRF, see https://github.com/gogs/gogs/issues/5366 for details.
		payloadURL, err := url.Parse(w.URL)
		if err != nil {
			c.FormErr("PayloadURL")
			c.RenderWithErr(c.Tr("repo.settings.webhook.err_cannot_parse_payload_url", err), orCtx.Tmpl, nil)
			return
		}

		if isLocalHostname(payloadURL.Hostname()) {
			c.FormErr("PayloadURL")
			c.RenderWithErr(c.Tr("repo.settings.webhook.err_cannot_use_local_addresses"), orCtx.Tmpl, nil)
			return
		}
	}

	if err := w.UpdateEvent(); err != nil {
		c.Error(err, "update event")
		return
	} else if err := db.UpdateWebhook(w); err != nil {
		c.Error(err, "update webhook")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func WebhooksEditPost(c *context.Context, f form.NewWebhook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	contentType := db.JSON
	if db.HookContentType(f.ContentType) == db.FORM {
		contentType = db.FORM
	}

	w.URL = f.PayloadURL
	w.ContentType = contentType
	w.Secret = f.Secret
	w.HookEvent = toHookEvent(f.Webhook)
	w.IsActive = f.Active
	validateAndUpdateWebhook(c, orCtx, w)
}

func WebhooksSlackEditPost(c *context.Context, f form.NewSlackHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Channel:  f.Channel,
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Error(err, "marshal JSON")
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = toHookEvent(f.Webhook)
	w.IsActive = f.Active
	validateAndUpdateWebhook(c, orCtx, w)
}

func WebhooksDiscordEditPost(c *context.Context, f form.NewDiscordHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	meta, err := jsoniter.Marshal(&db.SlackMeta{
		Username: f.Username,
		IconURL:  f.IconURL,
		Color:    f.Color,
	})
	if err != nil {
		c.Error(err, "marshal JSON")
		return
	}

	w.URL = f.PayloadURL
	w.Meta = string(meta)
	w.HookEvent = toHookEvent(f.Webhook)
	w.IsActive = f.Active
	validateAndUpdateWebhook(c, orCtx, w)
}

func WebhooksDingtalkEditPost(c *context.Context, f form.NewDingtalkHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	orCtx, w := checkWebhook(c)
	if c.Written() {
		return
	}
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.Tmpl)
		return
	}

	w.URL = f.PayloadURL
	w.HookEvent = toHookEvent(f.Webhook)
	w.IsActive = f.Active
	validateAndUpdateWebhook(c, orCtx, w)
}

func TestWebhook(c *context.Context) {
	var (
		commitID          string
		commitMessage     string
		author            *git.Signature
		committer         *git.Signature
		authorUsername    string
		committerUsername string
		nameStatus        *git.NameStatus
	)

	// Grab latest commit or fake one if it's empty repository.

	if c.Repo.Commit == nil {
		commitID = git.EmptyID
		commitMessage = "This is a fake commit"
		ghost := db.NewGhostUser()
		author = ghost.NewGitSig()
		committer = ghost.NewGitSig()
		authorUsername = ghost.Name
		committerUsername = ghost.Name
		nameStatus = &git.NameStatus{}

	} else {
		commitID = c.Repo.Commit.ID.String()
		commitMessage = c.Repo.Commit.Message
		author = c.Repo.Commit.Author
		committer = c.Repo.Commit.Committer

		// Try to match email with a real user.
		author, err := db.GetUserByEmail(c.Repo.Commit.Author.Email)
		if err == nil {
			authorUsername = author.Name
		} else if !db.IsErrUserNotExist(err) {
			c.Error(err, "get user by email")
			return
		}

		user, err := db.GetUserByEmail(c.Repo.Commit.Committer.Email)
		if err == nil {
			committerUsername = user.Name
		} else if !db.IsErrUserNotExist(err) {
			c.Error(err, "get user by email")
			return
		}

		nameStatus, err = c.Repo.Commit.ShowNameStatus()
		if err != nil {
			c.Error(err, "get changed files")
			return
		}
	}

	apiUser := c.User.APIFormat()
	p := &api.PushPayload{
		Ref:    git.RefsHeads + c.Repo.Repository.DefaultBranch,
		Before: commitID,
		After:  commitID,
		Commits: []*api.PayloadCommit{
			{
				ID:      commitID,
				Message: commitMessage,
				URL:     c.Repo.Repository.HTMLURL() + "/commit/" + commitID,
				Author: &api.PayloadUser{
					Name:     author.Name,
					Email:    author.Email,
					UserName: authorUsername,
				},
				Committer: &api.PayloadUser{
					Name:     committer.Name,
					Email:    committer.Email,
					UserName: committerUsername,
				},
				Added:    nameStatus.Added,
				Removed:  nameStatus.Removed,
				Modified: nameStatus.Modified,
			},
		},
		Repo:   c.Repo.Repository.APIFormat(nil),
		Pusher: apiUser,
		Sender: apiUser,
	}
	if err := db.TestWebhook(c.Repo.Repository, db.HOOK_EVENT_PUSH, p, c.ParamsInt64("id")); err != nil {
		c.Error(err, "test webhook")
		return
	}

	c.Flash.Info(c.Tr("repo.settings.webhook.test_delivery_success"))
	c.Status(http.StatusOK)
}

func RedeliveryWebhook(c *context.Context) {
	webhook, err := db.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get webhook")
		return
	}

	hookTask, err := db.GetHookTaskOfWebhookByUUID(webhook.ID, c.Query("uuid"))
	if err != nil {
		c.NotFoundOrError(err, "get hook task by UUID")
		return
	}

	hookTask.IsDelivered = false
	if err = db.UpdateHookTask(hookTask); err != nil {
		c.Error(err, "update hook task")
		return
	}

	go db.HookQueue.Add(c.Repo.Repository.ID)
	c.Flash.Info(c.Tr("repo.settings.webhook.redelivery_success", hookTask.UUID))
	c.Status(http.StatusOK)
}

func DeleteWebhook(c *context.Context) {
	if err := db.DeleteWebhookOfRepoByID(c.Repo.Repository.ID, c.QueryInt64("id")); err != nil {
		c.Flash.Error("DeleteWebhookByRepoID: " + err.Error())
	} else {
		c.Flash.Success(c.Tr("repo.settings.webhook_deletion_success"))
	}

	c.JSONSuccess(map[string]interface{}{
		"redirect": c.Repo.RepoLink + "/settings/hooks",
	})
}
