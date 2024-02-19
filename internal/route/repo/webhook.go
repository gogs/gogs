// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gogs/git-module"
	api "github.com/gogs/go-gogs-client"
	jsoniter "github.com/json-iterator/go"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/database/errors"
	"gogs.io/gogs/internal/form"
	"gogs.io/gogs/internal/netutil"
)

const (
	tmplRepoSettingsWebhooks   = "repo/settings/webhook/base"
	tmplRepoSettingsWebhookNew = "repo/settings/webhook/new"
	tmplOrgSettingsWebhooks    = "org/settings/webhooks"
	tmplOrgSettingsWebhookNew  = "org/settings/webhook_new"
)

func InjectOrgRepoContext() macaron.Handler {
	return func(c *context.Context) {
		orCtx, err := getOrgRepoContext(c)
		if err != nil {
			c.Error(err, "get organization or repository context")
			return
		}
		c.Map(orCtx)
	}
}

type orgRepoContext struct {
	OrgID    int64
	RepoID   int64
	Link     string
	TmplList string
	TmplNew  string
}

// getOrgRepoContext determines whether this is a repo context or organization context.
func getOrgRepoContext(c *context.Context) (*orgRepoContext, error) {
	if len(c.Repo.RepoLink) > 0 {
		c.PageIs("RepositoryContext")
		return &orgRepoContext{
			RepoID:   c.Repo.Repository.ID,
			Link:     c.Repo.RepoLink,
			TmplList: tmplRepoSettingsWebhooks,
			TmplNew:  tmplRepoSettingsWebhookNew,
		}, nil
	}

	if len(c.Org.OrgLink) > 0 {
		c.PageIs("OrganizationContext")
		return &orgRepoContext{
			OrgID:    c.Org.Organization.ID,
			Link:     c.Org.OrgLink,
			TmplList: tmplOrgSettingsWebhooks,
			TmplNew:  tmplOrgSettingsWebhookNew,
		}, nil
	}

	return nil, errors.New("unable to determine context")
}

func Webhooks(c *context.Context, orCtx *orgRepoContext) {
	c.Title("repo.settings.hooks")
	c.PageIs("SettingsHooks")
	c.Data["Types"] = conf.Webhook.Types

	var err error
	var ws []*database.Webhook
	if orCtx.RepoID > 0 {
		c.Data["Description"] = c.Tr("repo.settings.hooks_desc", "https://gogs.io/docs/features/webhook.html")
		ws, err = database.GetWebhooksByRepoID(orCtx.RepoID)
	} else {
		c.Data["Description"] = c.Tr("org.settings.hooks_desc")
		ws, err = database.GetWebhooksByOrgID(orCtx.OrgID)
	}
	if err != nil {
		c.Error(err, "get webhooks")
		return
	}
	c.Data["Webhooks"] = ws

	c.Success(orCtx.TmplList)
}

func WebhooksNew(c *context.Context, orCtx *orgRepoContext) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")

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

	c.Success(orCtx.TmplNew)
}

func validateWebhook(l macaron.Locale, w *database.Webhook) (field, msg string, ok bool) {
	// 🚨 SECURITY: Local addresses must not be allowed by non-admins to prevent SSRF,
	// see https://github.com/gogs/gogs/issues/5366 for details.
	payloadURL, err := url.Parse(w.URL)
	if err != nil {
		return "PayloadURL", l.Tr("repo.settings.webhook.err_cannot_parse_payload_url", err), false
	}

	if netutil.IsBlockedLocalHostname(payloadURL.Hostname(), conf.Security.LocalNetworkAllowlist) {
		return "PayloadURL", l.Tr("repo.settings.webhook.url_resolved_to_blocked_local_address"), false
	}
	return "", "", true
}

func validateAndCreateWebhook(c *context.Context, orCtx *orgRepoContext, w *database.Webhook) {
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.TmplNew)
		return
	}

	field, msg, ok := validateWebhook(c.Locale, w)
	if !ok {
		c.FormErr(field)
		c.RenderWithErr(msg, orCtx.TmplNew, nil)
		return
	}

	if err := w.UpdateEvent(); err != nil {
		c.Error(err, "update event")
		return
	} else if err := database.CreateWebhook(w); err != nil {
		c.Error(err, "create webhook")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.add_hook_success"))
	c.Redirect(orCtx.Link + "/settings/hooks")
}

func toHookEvent(f form.Webhook) *database.HookEvent {
	return &database.HookEvent{
		PushOnly:       f.PushOnly(),
		SendEverything: f.SendEverything(),
		ChooseEvents:   f.ChooseEvents(),
		HookEvents: database.HookEvents{
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

func WebhooksNewPost(c *context.Context, orCtx *orgRepoContext, f form.NewWebhook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "gogs"

	contentType := database.JSON
	if database.HookContentType(f.ContentType) == database.FORM {
		contentType = database.FORM
	}

	w := &database.Webhook{
		RepoID:       orCtx.RepoID,
		OrgID:        orCtx.OrgID,
		URL:          f.PayloadURL,
		ContentType:  contentType,
		Secret:       f.Secret,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: database.GOGS,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksSlackNewPost(c *context.Context, orCtx *orgRepoContext, f form.NewSlackHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "slack"

	meta := &database.SlackMeta{
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

	w := &database.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  database.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: database.SLACK,
		Meta:         string(p),
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksDiscordNewPost(c *context.Context, orCtx *orgRepoContext, f form.NewDiscordHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "discord"

	meta := &database.SlackMeta{
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

	w := &database.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  database.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: database.DISCORD,
		Meta:         string(p),
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func WebhooksDingtalkNewPost(c *context.Context, orCtx *orgRepoContext, f form.NewDingtalkHook) {
	c.Title("repo.settings.add_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksNew")
	c.Data["HookType"] = "dingtalk"

	w := &database.Webhook{
		RepoID:       orCtx.RepoID,
		URL:          f.PayloadURL,
		ContentType:  database.JSON,
		HookEvent:    toHookEvent(f.Webhook),
		IsActive:     f.Active,
		HookTaskType: database.DINGTALK,
		OrgID:        orCtx.OrgID,
	}
	validateAndCreateWebhook(c, orCtx, w)
}

func loadWebhook(c *context.Context, orCtx *orgRepoContext) *database.Webhook {
	c.RequireHighlightJS()

	var err error
	var w *database.Webhook
	if orCtx.RepoID > 0 {
		w, err = database.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	} else {
		w, err = database.GetWebhookByOrgID(c.Org.Organization.ID, c.ParamsInt64(":id"))
	}
	if err != nil {
		c.NotFoundOrError(err, "get webhook")
		return nil
	}
	c.Data["Webhook"] = w

	switch w.HookTaskType {
	case database.SLACK:
		c.Data["SlackMeta"] = w.SlackMeta()
		c.Data["HookType"] = "slack"
	case database.DISCORD:
		c.Data["SlackMeta"] = w.SlackMeta()
		c.Data["HookType"] = "discord"
	case database.DINGTALK:
		c.Data["HookType"] = "dingtalk"
	default:
		c.Data["HookType"] = "gogs"
	}
	c.Data["FormURL"] = fmt.Sprintf("%s/settings/hooks/%s/%d", orCtx.Link, c.Data["HookType"], w.ID)
	c.Data["DeleteURL"] = fmt.Sprintf("%s/settings/hooks/delete", orCtx.Link)

	c.Data["History"], err = w.History(1)
	if err != nil {
		c.Error(err, "get history")
		return nil
	}
	return w
}

func WebhooksEdit(c *context.Context, orCtx *orgRepoContext) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	loadWebhook(c, orCtx)
	if c.Written() {
		return
	}

	c.Success(orCtx.TmplNew)
}

func validateAndUpdateWebhook(c *context.Context, orCtx *orgRepoContext, w *database.Webhook) {
	c.Data["Webhook"] = w

	if c.HasError() {
		c.Success(orCtx.TmplNew)
		return
	}

	field, msg, ok := validateWebhook(c.Locale, w)
	if !ok {
		c.FormErr(field)
		c.RenderWithErr(msg, orCtx.TmplNew, nil)
		return
	}

	if err := w.UpdateEvent(); err != nil {
		c.Error(err, "update event")
		return
	} else if err := database.UpdateWebhook(w); err != nil {
		c.Error(err, "update webhook")
		return
	}

	c.Flash.Success(c.Tr("repo.settings.update_hook_success"))
	c.Redirect(fmt.Sprintf("%s/settings/hooks/%d", orCtx.Link, w.ID))
}

func WebhooksEditPost(c *context.Context, orCtx *orgRepoContext, f form.NewWebhook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	w := loadWebhook(c, orCtx)
	if c.Written() {
		return
	}

	contentType := database.JSON
	if database.HookContentType(f.ContentType) == database.FORM {
		contentType = database.FORM
	}

	w.URL = f.PayloadURL
	w.ContentType = contentType
	w.Secret = f.Secret
	w.HookEvent = toHookEvent(f.Webhook)
	w.IsActive = f.Active
	validateAndUpdateWebhook(c, orCtx, w)
}

func WebhooksSlackEditPost(c *context.Context, orCtx *orgRepoContext, f form.NewSlackHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	w := loadWebhook(c, orCtx)
	if c.Written() {
		return
	}

	meta, err := jsoniter.Marshal(&database.SlackMeta{
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

func WebhooksDiscordEditPost(c *context.Context, orCtx *orgRepoContext, f form.NewDiscordHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	w := loadWebhook(c, orCtx)
	if c.Written() {
		return
	}

	meta, err := jsoniter.Marshal(&database.SlackMeta{
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

func WebhooksDingtalkEditPost(c *context.Context, orCtx *orgRepoContext, f form.NewDingtalkHook) {
	c.Title("repo.settings.update_webhook")
	c.PageIs("SettingsHooks")
	c.PageIs("SettingsHooksEdit")

	w := loadWebhook(c, orCtx)
	if c.Written() {
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
		ghost := database.NewGhostUser()
		author = &git.Signature{
			Name:  ghost.DisplayName(),
			Email: ghost.Email,
			When:  time.Now(),
		}
		committer = author
		authorUsername = ghost.Name
		committerUsername = ghost.Name
		nameStatus = &git.NameStatus{}

	} else {
		commitID = c.Repo.Commit.ID.String()
		commitMessage = c.Repo.Commit.Message
		author = c.Repo.Commit.Author
		committer = c.Repo.Commit.Committer

		// Try to match email with a real user.
		author, err := database.Users.GetByEmail(c.Req.Context(), c.Repo.Commit.Author.Email)
		if err == nil {
			authorUsername = author.Name
		} else if !database.IsErrUserNotExist(err) {
			c.Error(err, "get user by email")
			return
		}

		user, err := database.Users.GetByEmail(c.Req.Context(), c.Repo.Commit.Committer.Email)
		if err == nil {
			committerUsername = user.Name
		} else if !database.IsErrUserNotExist(err) {
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
		Repo:   c.Repo.Repository.APIFormatLegacy(nil),
		Pusher: apiUser,
		Sender: apiUser,
	}
	if err := database.TestWebhook(c.Repo.Repository, database.HOOK_EVENT_PUSH, p, c.ParamsInt64("id")); err != nil {
		c.Error(err, "test webhook")
		return
	}

	c.Flash.Info(c.Tr("repo.settings.webhook.test_delivery_success"))
	c.Status(http.StatusOK)
}

func RedeliveryWebhook(c *context.Context) {
	webhook, err := database.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get webhook")
		return
	}

	hookTask, err := database.GetHookTaskOfWebhookByUUID(webhook.ID, c.Query("uuid"))
	if err != nil {
		c.NotFoundOrError(err, "get hook task by UUID")
		return
	}

	hookTask.IsDelivered = false
	if err = database.UpdateHookTask(hookTask); err != nil {
		c.Error(err, "update hook task")
		return
	}

	go database.HookQueue.Add(c.Repo.Repository.ID)
	c.Flash.Info(c.Tr("repo.settings.webhook.redelivery_success", hookTask.UUID))
	c.Status(http.StatusOK)
}

func DeleteWebhook(c *context.Context, orCtx *orgRepoContext) {
	var err error
	if orCtx.RepoID > 0 {
		err = database.DeleteWebhookOfRepoByID(orCtx.RepoID, c.QueryInt64("id"))
	} else {
		err = database.DeleteWebhookOfOrgByID(orCtx.OrgID, c.QueryInt64("id"))
	}
	if err != nil {
		c.Error(err, "delete webhook")
		return
	}
	c.Flash.Success(c.Tr("repo.settings.webhook_deletion_success"))

	c.JSONSuccess(map[string]any{
		"redirect": orCtx.Link + "/settings/hooks",
	})
}
