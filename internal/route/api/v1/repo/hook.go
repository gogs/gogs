// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repo

import (
	"net/http"

	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/unknwon/com"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/convert"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories#list-hooks
func ListHooks(c *context.APIContext) {
	hooks, err := database.GetWebhooksByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Errorf(err, "get webhooks by repository ID")
		return
	}

	apiHooks := make([]*api.Hook, len(hooks))
	for i := range hooks {
		apiHooks[i] = convert.ToHook(c.Repo.RepoLink, hooks[i])
	}
	c.JSONSuccess(&apiHooks)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#create-a-hook
func CreateHook(c *context.APIContext, form api.CreateHookOption) {
	if !database.IsValidHookTaskType(form.Type) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Invalid hook type."))
		return
	}
	for _, name := range []string{"url", "content_type"} {
		if _, ok := form.Config[name]; !ok {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Missing config option: "+name))
			return
		}
	}
	if !database.IsValidHookContentType(form.Config["content_type"]) {
		c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Invalid content type."))
		return
	}

	if len(form.Events) == 0 {
		form.Events = []string{"push"}
	}
	w := &database.Webhook{
		RepoID:      c.Repo.Repository.ID,
		URL:         form.Config["url"],
		ContentType: database.ToHookContentType(form.Config["content_type"]),
		Secret:      form.Config["secret"],
		HookEvent: &database.HookEvent{
			ChooseEvents: true,
			HookEvents: database.HookEvents{
				Create:       com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_CREATE)),
				Delete:       com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_DELETE)),
				Fork:         com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_FORK)),
				Push:         com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_PUSH)),
				Issues:       com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_ISSUES)),
				IssueComment: com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_ISSUE_COMMENT)),
				PullRequest:  com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_PULL_REQUEST)),
				Release:      com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_RELEASE)),
			},
		},
		IsActive:     form.Active,
		HookTaskType: database.ToHookTaskType(form.Type),
	}
	if w.HookTaskType == database.SLACK {
		channel, ok := form.Config["channel"]
		if !ok {
			c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Missing config option: channel"))
			return
		}
		meta, err := jsoniter.Marshal(&database.SlackMeta{
			Channel:  channel,
			Username: form.Config["username"],
			IconURL:  form.Config["icon_url"],
			Color:    form.Config["color"],
		})
		if err != nil {
			c.Errorf(err, "marshal JSON")
			return
		}
		w.Meta = string(meta)
	}

	if err := w.UpdateEvent(); err != nil {
		c.Errorf(err, "update event")
		return
	} else if err := database.CreateWebhook(w); err != nil {
		c.Errorf(err, "create webhook")
		return
	}

	c.JSON(http.StatusCreated, convert.ToHook(c.Repo.RepoLink, w))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#edit-a-hook
func EditHook(c *context.APIContext, form api.EditHookOption) {
	w, err := database.GetWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id"))
	if err != nil {
		c.NotFoundOrError(err, "get webhook of repository by ID")
		return
	}

	if form.Config != nil {
		if url, ok := form.Config["url"]; ok {
			w.URL = url
		}
		if ct, ok := form.Config["content_type"]; ok {
			if !database.IsValidHookContentType(ct) {
				c.ErrorStatus(http.StatusUnprocessableEntity, errors.New("Invalid content type."))
				return
			}
			w.ContentType = database.ToHookContentType(ct)
		}

		if w.HookTaskType == database.SLACK {
			if channel, ok := form.Config["channel"]; ok {
				meta, err := jsoniter.Marshal(&database.SlackMeta{
					Channel:  channel,
					Username: form.Config["username"],
					IconURL:  form.Config["icon_url"],
					Color:    form.Config["color"],
				})
				if err != nil {
					c.Errorf(err, "marshal JSON")
					return
				}
				w.Meta = string(meta)
			}
		}
	}

	// Update events
	if len(form.Events) == 0 {
		form.Events = []string{"push"}
	}
	w.PushOnly = false
	w.SendEverything = false
	w.ChooseEvents = true
	w.Create = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_CREATE))
	w.Delete = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_DELETE))
	w.Fork = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_FORK))
	w.Push = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_PUSH))
	w.Issues = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_ISSUES))
	w.IssueComment = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_ISSUE_COMMENT))
	w.PullRequest = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_PULL_REQUEST))
	w.Release = com.IsSliceContainsStr(form.Events, string(database.HOOK_EVENT_RELEASE))
	if err = w.UpdateEvent(); err != nil {
		c.Errorf(err, "update event")
		return
	}

	if form.Active != nil {
		w.IsActive = *form.Active
	}

	if err := database.UpdateWebhook(w); err != nil {
		c.Errorf(err, "update webhook")
		return
	}

	c.JSONSuccess(convert.ToHook(c.Repo.RepoLink, w))
}

func DeleteHook(c *context.APIContext) {
	if err := database.DeleteWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Errorf(err, "delete webhook of repository by ID")
		return
	}

	c.NoContent()
}
