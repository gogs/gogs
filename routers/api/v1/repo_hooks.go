// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"encoding/json"

	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/middleware"
)

type ApiHook struct {
	Id     int64             `json:"id"`
	Type   string            `json:"type"`
	Events []string          `json:"events"`
	Active bool              `json:"active"`
	Config map[string]string `json:"config"`
}

// GET /repos/:username/:reponame/hooks
// https://developer.github.com/v3/repos/hooks/#list-hooks
func ListRepoHooks(ctx *middleware.Context) {
	hooks, err := models.GetWebhooksByRepoId(ctx.Repo.Repository.Id)
	if err != nil {
		ctx.JSON(500, map[string]interface{}{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}

	apiHooks := make([]*ApiHook, len(hooks))
	for i := range hooks {
		h := &ApiHook{
			Id:     hooks[i].Id,
			Type:   hooks[i].HookTaskType.Name(),
			Active: hooks[i].IsActive,
			Config: make(map[string]string),
		}

		// Currently, onle have push event.
		h.Events = []string{"push"}

		h.Config["url"] = hooks[i].Url
		h.Config["content_type"] = hooks[i].ContentType.Name()
		if hooks[i].HookTaskType == models.SLACK {
			s := hooks[i].GetSlackHook()
			h.Config["channel"] = s.Channel
		}

		apiHooks[i] = h
	}

	ctx.JSON(200, &apiHooks)
}

type CreateRepoHookForm struct {
	Type   string            `json:"type" binding:"Required"`
	Config map[string]string `json:"config" binding:"Required"`
	Active bool              `json:"active"`
}

// POST /repos/:username/:reponame/hooks
// https://developer.github.com/v3/repos/hooks/#create-a-hook
func CreateRepoHook(ctx *middleware.Context, form CreateRepoHookForm) {
	if !models.IsValidHookTaskType(form.Type) {
		ctx.JSON(422, &base.ApiJsonErr{"invalid hook type", DOC_URL})
		return
	}
	for _, name := range []string{"url", "content_type"} {
		if _, ok := form.Config[name]; !ok {
			ctx.JSON(422, &base.ApiJsonErr{"missing config option: " + name, DOC_URL})
			return
		}
	}
	if !models.IsValidHookContentType(form.Config["content_type"]) {
		ctx.JSON(422, &base.ApiJsonErr{"invalid content type", DOC_URL})
		return
	}

	w := &models.Webhook{
		RepoId:      ctx.Repo.Repository.Id,
		Url:         form.Config["url"],
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
			ctx.JSON(422, &base.ApiJsonErr{"missing config option: channel", DOC_URL})
			return
		}
		meta, err := json.Marshal(&models.Slack{
			Channel: channel,
		})
		if err != nil {
			ctx.JSON(500, &base.ApiJsonErr{"slack: JSON marshal failed: " + err.Error(), DOC_URL})
			return
		}
		w.Meta = string(meta)
	}

	if err := w.UpdateEvent(); err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"UpdateEvent: " + err.Error(), DOC_URL})
		return
	} else if err := models.CreateWebhook(w); err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"CreateWebhook: " + err.Error(), DOC_URL})
		return
	}

	ctx.JSON(201, map[string]interface{}{
		"ok": true,
	})
}

type EditRepoHookForm struct {
	Config map[string]string `json:"config"`
	Active *bool             `json:"active"`
}

// PATCH /repos/:username/:reponame/hooks/:id
// https://developer.github.com/v3/repos/hooks/#edit-a-hook
func EditRepoHook(ctx *middleware.Context, form EditRepoHookForm) {
	w, err := models.GetWebhookById(ctx.ParamsInt64(":id"))
	if err != nil {
		ctx.JSON(500, &base.ApiJsonErr{"GetWebhookById: " + err.Error(), DOC_URL})
		return
	}

	if form.Config != nil {
		if url, ok := form.Config["url"]; ok {
			w.Url = url
		}
		if ct, ok := form.Config["content_type"]; ok {
			if !models.IsValidHookContentType(ct) {
				ctx.JSON(422, &base.ApiJsonErr{"invalid content type", DOC_URL})
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
					ctx.JSON(500, &base.ApiJsonErr{"slack: JSON marshal failed: " + err.Error(), DOC_URL})
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
		ctx.JSON(500, &base.ApiJsonErr{"UpdateWebhook: " + err.Error(), DOC_URL})
		return
	}

	ctx.JSON(200, map[string]interface{}{
		"ok": true,
	})
}
