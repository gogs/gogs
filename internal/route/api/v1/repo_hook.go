package v1

import (
	"encoding/json"
	"net/http"
	"slices"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/context"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/route/api/v1/types"
)

// https://github.com/gogs/go-gogs-client/wiki/Repositories#list-hooks
func listHooks(c *context.APIContext) {
	hooks, err := database.GetWebhooksByRepoID(c.Repo.Repository.ID)
	if err != nil {
		c.Errorf(err, "get webhooks by repository ID")
		return
	}

	apiHooks := make([]*types.RepositoryHook, len(hooks))
	for i := range hooks {
		apiHooks[i] = toRepositoryHook(c.Repo.RepoLink, hooks[i])
	}
	c.JSONSuccess(&apiHooks)
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#create-a-hook
type createHookRequest struct {
	Type   string            `json:"type" binding:"Required"`
	Config map[string]string `json:"config" binding:"Required"`
	Events []string          `json:"events"`
	Active bool              `json:"active"`
}

func createHook(c *context.APIContext, form createHookRequest) {
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
				Create:       slices.Contains(form.Events, string(database.HookEventTypeCreate)),
				Delete:       slices.Contains(form.Events, string(database.HookEventTypeDelete)),
				Fork:         slices.Contains(form.Events, string(database.HookEventTypeFork)),
				Push:         slices.Contains(form.Events, string(database.HookEventTypePush)),
				Issues:       slices.Contains(form.Events, string(database.HookEventTypeIssues)),
				IssueComment: slices.Contains(form.Events, string(database.HookEventTypeIssueComment)),
				PullRequest:  slices.Contains(form.Events, string(database.HookEventTypePullRequest)),
				Release:      slices.Contains(form.Events, string(database.HookEventTypeRelease)),
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
		meta, err := json.Marshal(&database.SlackMeta{
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

	c.JSON(http.StatusCreated, toRepositoryHook(c.Repo.RepoLink, w))
}

// https://github.com/gogs/go-gogs-client/wiki/Repositories#edit-a-hook
type editHookRequest struct {
	Config map[string]string `json:"config"`
	Events []string          `json:"events"`
	Active *bool             `json:"active"`
}

func editHook(c *context.APIContext, form editHookRequest) {
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
				meta, err := json.Marshal(&database.SlackMeta{
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
	w.Create = slices.Contains(form.Events, string(database.HookEventTypeCreate))
	w.Delete = slices.Contains(form.Events, string(database.HookEventTypeDelete))
	w.Fork = slices.Contains(form.Events, string(database.HookEventTypeFork))
	w.Push = slices.Contains(form.Events, string(database.HookEventTypePush))
	w.Issues = slices.Contains(form.Events, string(database.HookEventTypeIssues))
	w.IssueComment = slices.Contains(form.Events, string(database.HookEventTypeIssueComment))
	w.PullRequest = slices.Contains(form.Events, string(database.HookEventTypePullRequest))
	w.Release = slices.Contains(form.Events, string(database.HookEventTypeRelease))
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

	c.JSONSuccess(toRepositoryHook(c.Repo.RepoLink, w))
}

func deleteHook(c *context.APIContext) {
	if err := database.DeleteWebhookOfRepoByID(c.Repo.Repository.ID, c.ParamsInt64(":id")); err != nil {
		c.Errorf(err, "delete webhook of repository by ID")
		return
	}

	c.NoContent()
}
