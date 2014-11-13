// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package v1

import (
	"github.com/gogits/gogs/models"
	"github.com/gogits/gogs/modules/middleware"
)

type apiHookConfig struct {
	Url         string `json:"url"`
	ContentType string `json:"content_type"`
}

type ApiHook struct {
	Id     int64         `json:"id"`
	Type   string        `json:"type"`
	Events []string      `json:"events"`
	Active bool          `json:"active"`
	Config apiHookConfig `json:"config"`
}

// /repos/:username/:reponame/hooks: https://developer.github.com/v3/repos/hooks/#list-hooks
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
		apiHooks[i] = &ApiHook{
			Id:     hooks[i].Id,
			Type:   hooks[i].HookTaskType.Name(),
			Active: hooks[i].IsActive,
			Config: apiHookConfig{hooks[i].Url, hooks[i].ContentType.Name()},
		}

		// Currently, onle have push event.
		apiHooks[i].Events = []string{"push"}
	}

	ctx.JSON(200, &apiHooks)
}
