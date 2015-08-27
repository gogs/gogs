// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"encoding/json"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

// GET /repos/:username/:reponame/hooks
// https://developer.github.com/v3/repos/hooks/#list-hooks
func ListRepoHooks(ctx *middleware.Context) {
	hooks, err := models.GetWebhooksByRepoId(ctx.Repo.Repository.ID)
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetWebhooksByRepoId: " + err.Error(), base.DOC_URL})
		return
	}

	apiHooks := make([]*api.Hook, len(hooks))
	for i := range hooks {
		h := &api.Hook{
			ID:     hooks[i].ID,
			Type:   hooks[i].HookTaskType.Name(),
			Active: hooks[i].IsActive,
			Config: make(map[string]string),
		}

		// Currently, onle have push event.
		h.Events = []string{"push"}

		h.Config["url"] = hooks[i].URL
		h.Config["content_type"] = hooks[i].ContentType.Name()
		if hooks[i].HookTaskType == models.SLACK {
			s := hooks[i].GetSlackHook()
			h.Config["channel"] = s.Channel
		}

		apiHooks[i] = h
	}

	ctx.JSON(200, &apiHooks)
}

// POST /repos/:username/:reponame/hooks
// https://developer.github.com/v3/repos/hooks/#create-a-hook
func CreateRepoHook(ctx *middleware.Context, form api.CreateHookOption) {
	if !models.IsValidHookTaskType(form.Type) {
		ctx.JSON(422, &base.ApiJsonErr{"invalid hook type", base.DOC_URL})
		return
	}
	for _, name := range []string{"url", "content_type"} {
		if _, ok := form.Config[name]; !ok {
			ctx.JSON(422, &base.ApiJsonErr{"missing config option: " + name, base.DOC_URL})
			return
		}
	}
	if !models.IsValidHookContentType(form.Config["content_type"]) {
		ctx.JSON(422, &base.ApiJsonErr{"invalid content type", base.DOC_URL})
		return
	}

	w := &models.Webhook{
		RepoID:      ctx.Repo.Repository.ID,
		URL:         form.Config["url"],
		ContentType: models.ToHookContentType(form.Config["content_type"]),
		Secret:      form.Config["secret"],
		HookEvent: &models.HookEvent{
			PushOnly: true, // Only support it now.
		},
		IsActive:     form.Active,
		HookTaskType: models.ToHookTaskType(form.Type),
	}
	if w.HookTaskType == models.SLACK {
		channel, ok := form.Config["channel"]
		if !ok {
			ctx.JSON(422, &base.ApiJsonErr{"missing config option: channel", base.DOC_URL})
			return
		}
		meta, err := json.Marshal(&models.Slack{
			Channel: channel,
		})
		if err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"slack: JSON marshal failed: " + err.Error(), base.DOC_URL})
			return
		}
		w.Meta = string(meta)
	}

	if err := w.UpdateEvent(); err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"UpdateEvent: " + err.Error(), base.DOC_URL})
		return
	} else if err := models.CreateWebhook(w); err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"CreateWebhook: " + err.Error(), base.DOC_URL})
		return
	}

	apiHook := &api.Hook{
		ID:     w.ID,
		Type:   w.HookTaskType.Name(),
		Events: []string{"push"},
		Active: w.IsActive,
		Config: map[string]string{
			"url":          w.URL,
			"content_type": w.ContentType.Name(),
		},
	}
	if w.HookTaskType == models.SLACK {
		s := w.GetSlackHook()
		apiHook.Config["channel"] = s.Channel
	}
	ctx.JSON(201, apiHook)
}

// PATCH /repos/:username/:reponame/hooks/:id
// https://developer.github.com/v3/repos/hooks/#edit-a-hook
func EditRepoHook(ctx *middleware.Context, form api.EditHookOption) {
	w, err := models.GetWebhookByID(ctx.ParamsInt64(":id"))
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetWebhookById: " + err.Error(), base.DOC_URL})
		return
	}

	if form.Config != nil {
		if url, ok := form.Config["url"]; ok {
			w.URL = url
		}
		if ct, ok := form.Config["content_type"]; ok {
			if !models.IsValidHookContentType(ct) {
				ctx.JSON(422, &base.ApiJsonErr{"invalid content type", base.DOC_URL})
				return
			}
			w.ContentType = models.ToHookContentType(ct)
		}

		if w.HookTaskType == models.SLACK {
			if channel, ok := form.Config["channel"]; ok {
				meta, err := json.Marshal(&models.Slack{
					Channel: channel,
				})
				if err != nil {
					ctx.JSON(500, &base.ApiJsonErr{"slack: JSON marshal failed: " + err.Error(), base.DOC_URL})
					return
				}
				w.Meta = string(meta)
			}
		}
	}

	if form.Active != nil {
		w.IsActive = *form.Active
	}

	// FIXME: edit events
	if err := models.UpdateWebhook(w); err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"UpdateWebhook: " + err.Error(), base.DOC_URL})
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}
