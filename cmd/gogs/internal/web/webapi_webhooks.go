package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/flamego/flamego"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/i18n"
	"gogs.io/gogs/internal/netx"
	"gogs.io/gogs/internal/repox"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// webhookScope is the repository or the organization whose webhooks are
// managed. Exactly one of the IDs is set.
type webhookScope struct {
	RepoID int64
	OrgID  int64
}

func withRepoWebhooks(c flamego.Context, rc *repoAdminContext) {
	c.Map(&webhookScope{RepoID: rc.Repo.ID})
}

// withOrgWebhooks lets only the owners of the organization manage its
// webhooks.
func withOrgWebhooks(c flamego.Context, m *orgMembership) {
	if !m.IsOwner {
		writeErrorResponse(c.ResponseWriter(), http.StatusForbidden, errors.New("organization owner required"))
		return
	}
	c.Map(&webhookScope{OrgID: m.Org.ID})
}

// withWebhook injects the webhook named in the route. Webhooks of other
// repositories and organizations are not found.
func withWebhook(c flamego.Context, scope *webhookScope) {
	// An ID of 0 would match any webhook of the repository or organization.
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if id <= 0 {
		writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("webhook does not exist"))
		return
	}
	var w *database.Webhook
	var err error
	if scope.RepoID > 0 {
		w, err = database.GetWebhookOfRepoByID(scope.RepoID, id)
	} else {
		w, err = database.GetWebhookByOrgID(scope.OrgID, id)
	}
	if err != nil {
		if database.IsErrWebhookNotExist(err) {
			writeErrorResponse(c.ResponseWriter(), http.StatusNotFound, errors.New("webhook does not exist"))
			return
		}
		log.Error("withWebhook: get webhook %d: %v", id, err)
		writeErrorResponse(c.ResponseWriter(), http.StatusInternalServerError, errors.Wrap(err, "get webhook"))
		return
	}
	c.Map(w)
}

type webhookListItem struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
	URL  string `json:"url"`
	// LastStatus is "succeeded" or "failed" for the last delivery, or "none"
	// before the first one.
	LastStatus string `json:"lastStatus"`
	Active     bool   `json:"active"`
}

type webhookList struct {
	Items []webhookListItem `json:"items"`
	// Types are the kinds of webhooks the site allows to add.
	Types []string `json:"types"`
}

func webhookLastStatus(s database.HookStatus) string {
	switch s {
	case database.HookStatusSucceed:
		return "succeeded"
	case database.HookStatusFailed:
		return "failed"
	}
	return "none"
}

func getWebhooks(scope *webhookScope) (statusCode int, resp any, err error) {
	var ws []*database.Webhook
	if scope.RepoID > 0 {
		ws, err = database.GetWebhooksByRepoID(scope.RepoID)
	} else {
		ws, err = database.GetWebhooksByOrgID(scope.OrgID)
	}
	if err != nil {
		log.Error("getWebhooks: list webhooks of repository %d or organization %d: %v", scope.RepoID, scope.OrgID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list webhooks")
	}
	items := make([]webhookListItem, 0, len(ws))
	for _, w := range ws {
		items = append(items, webhookListItem{
			ID:         w.ID,
			Type:       w.HookTaskType.Name(),
			URL:        w.URL,
			LastStatus: webhookLastStatus(w.LastStatus),
			Active:     w.IsActive,
		})
	}
	return http.StatusOK, &webhookList{Items: items, Types: conf.Webhook.Types}, nil
}

type webhookEvents struct {
	Create       bool `json:"create"`
	Delete       bool `json:"delete"`
	Fork         bool `json:"fork"`
	Push         bool `json:"push"`
	Issues       bool `json:"issues"`
	IssueComment bool `json:"issueComment"`
	PullRequest  bool `json:"pullRequest"`
	Release      bool `json:"release"`
}

// webhookRequest is the settings of a webhook. Which of them apply depends on
// its type: only Gogs webhooks have a content type and a secret, only Slack
// webhooks have a channel, and Dingtalk webhooks have none of the others.
type webhookRequest struct {
	// Type is only read when the webhook is added.
	Type        string `json:"type"`
	PayloadURL  string `json:"url"`
	ContentType string `json:"contentType" validate:"omitempty,oneof=json form"`
	Secret      string `json:"secret"`
	Channel     string `json:"channel"`
	Username    string `json:"username"`
	IconURL     string `json:"iconURL"`
	Color       string `json:"color"`
	// Events is "push_only", "send_everything" or "choose_events", which sends
	// ChosenEvents.
	Events       string        `json:"events" validate:"required,oneof=push_only send_everything choose_events"`
	ChosenEvents webhookEvents `json:"chosenEvents"`
	Active       bool          `json:"active"`
}

type webhookResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

type webhookDelivery struct {
	UUID string `json:"uuid"`
	// DeliveredAt is not sent while the delivery waits in the queue.
	DeliveredAt    *time.Time        `json:"deliveredAt,omitempty"`
	Succeeded      bool              `json:"succeeded"`
	URL            string            `json:"url"`
	RequestHeaders map[string]string `json:"requestHeaders,omitempty"`
	Payload        string            `json:"payload"`
	// Response is not sent before the delivery is attempted.
	Response *webhookResponse `json:"response,omitempty"`
}

type webhookDetail struct {
	ID       int64          `json:"id"`
	Settings webhookRequest `json:"settings"`
	// Deliveries are the most recent deliveries, newest first.
	Deliveries []webhookDelivery `json:"deliveries"`
}

func getWebhook(w *database.Webhook) (statusCode int, resp any, err error) {
	settings := webhookRequest{
		Type:       w.HookTaskType.Name(),
		PayloadURL: w.URL,
		Active:     w.IsActive,
		Events:     "push_only",
	}
	if w.HookTaskType == database.GOGS {
		settings.ContentType = w.ContentType.Name()
		settings.Secret = w.Secret
	}
	if w.HookTaskType == database.SLACK || w.HookTaskType == database.DISCORD {
		meta := w.SlackMeta()
		settings.Channel = meta.Channel
		settings.Username = meta.Username
		settings.IconURL = meta.IconURL
		settings.Color = meta.Color
	}
	if w.HookEvent != nil {
		switch {
		case w.SendEverything:
			settings.Events = "send_everything"
		case w.ChooseEvents:
			settings.Events = "choose_events"
		}
		settings.ChosenEvents = webhookEvents{
			Create:       w.Create,
			Delete:       w.Delete,
			Fork:         w.Fork,
			Push:         w.Push,
			Issues:       w.Issues,
			IssueComment: w.IssueComment,
			PullRequest:  w.PullRequest,
			Release:      w.Release,
		}
	}

	tasks, err := w.History(1)
	if err != nil {
		log.Error("getWebhook: list deliveries of webhook %d: %v", w.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "list deliveries")
	}
	deliveries := make([]webhookDelivery, 0, len(tasks))
	for _, t := range tasks {
		d := webhookDelivery{UUID: t.UUID, Succeeded: t.IsSucceed, URL: t.URL, Payload: t.PayloadContent}
		if t.IsDelivered {
			at := time.Unix(0, t.Delivered)
			d.DeliveredAt = &at
		}
		if t.RequestInfo != nil {
			d.RequestHeaders = t.RequestInfo.Headers
		}
		if t.ResponseInfo != nil {
			d.Response = &webhookResponse{Status: t.ResponseInfo.Status, Headers: t.ResponseInfo.Headers, Body: t.ResponseInfo.Body}
		}
		deliveries = append(deliveries, d)
	}
	return http.StatusOK, &webhookDetail{ID: w.ID, Settings: settings, Deliveries: deliveries}, nil
}

// validateWebhookURL returns the status code and the message for a payload
// URL that cannot be used, or http.StatusOK.
func validateWebhookURL(l i18n.Locale, rawURL string) (statusCode int, msg string) {
	// 🚨 SECURITY: Local addresses must not be allowed by non-admins to prevent SSRF,
	// see https://github.com/gogs/gogs/issues/5366 for details.
	payloadURL, err := url.Parse(rawURL)
	if err != nil {
		return http.StatusBadRequest, l.Tr("repo.settings.webhook.err_cannot_parse_payload_url", err)
	}
	if netx.IsBlockedLocalHostname(payloadURL.Hostname(), conf.Security.LocalNetworkAllowlist) {
		return http.StatusForbidden, l.Tr("repo.settings.webhook.url_resolved_to_blocked_local_address")
	}
	return http.StatusOK, ""
}

// applyWebhookRequest copies the settings that apply to the type of the
// webhook from the request, or returns the status code and the response for
// settings that cannot be used.
func applyWebhookRequest(l i18n.Locale, w *database.Webhook, req webhookRequest) (statusCode int, resp *bindingErrorResponse, err error) {
	// The fields are named after their labels on the page, which the shared
	// validation messages cannot do.
	fieldError := func(code int, field, msg string) (int, *bindingErrorResponse, error) {
		return code, &bindingErrorResponse{Fields: fieldErrors{field: &msg}}, nil
	}
	switch {
	case strings.TrimSpace(req.PayloadURL) == "":
		return fieldError(http.StatusBadRequest, "url", l.Tr("repo.settings.payload_url")+l.Tr("form.require_error"))
	case webAPIValidator.Var(req.PayloadURL, "url") != nil:
		return fieldError(http.StatusBadRequest, "url", l.Tr("repo.settings.payload_url")+l.Tr("form.url_error"))
	case w.HookTaskType == database.SLACK && strings.TrimSpace(req.Channel) == "":
		return fieldError(http.StatusBadRequest, "channel", l.Tr("repo.settings.slack_channel")+l.Tr("form.require_error"))
	}
	if code, msg := validateWebhookURL(l, req.PayloadURL); code != http.StatusOK {
		return fieldError(code, "url", msg)
	}

	w.URL = req.PayloadURL
	w.IsActive = req.Active
	w.HookEvent = &database.HookEvent{
		PushOnly:       req.Events == "push_only",
		SendEverything: req.Events == "send_everything",
		ChooseEvents:   req.Events == "choose_events",
		HookEvents: database.HookEvents{
			Create:       req.ChosenEvents.Create,
			Delete:       req.ChosenEvents.Delete,
			Fork:         req.ChosenEvents.Fork,
			Push:         req.ChosenEvents.Push,
			Issues:       req.ChosenEvents.Issues,
			IssueComment: req.ChosenEvents.IssueComment,
			PullRequest:  req.ChosenEvents.PullRequest,
			Release:      req.ChosenEvents.Release,
		},
	}
	switch w.HookTaskType {
	case database.GOGS:
		w.ContentType = database.JSON
		if req.ContentType == "form" {
			w.ContentType = database.FORM
		}
		w.Secret = req.Secret
	case database.SLACK, database.DISCORD:
		meta := &database.SlackMeta{Username: req.Username, IconURL: req.IconURL, Color: req.Color}
		if w.HookTaskType == database.SLACK {
			meta.Channel = req.Channel
		}
		p, err := json.Marshal(meta)
		if err != nil {
			return 0, nil, errors.Wrap(err, "marshal meta")
		}
		w.Meta = string(p)
	}
	if err = w.UpdateEvent(); err != nil {
		return 0, nil, errors.Wrap(err, "encode events")
	}
	return http.StatusOK, nil, nil
}

type webhookIDResponse struct {
	ID int64 `json:"id"`
}

func postWebhook(l i18n.Locale, scope *webhookScope, req webhookRequest) (statusCode int, resp any, err error) {
	hookType := database.ToHookTaskType(req.Type)
	if hookType == 0 || !slices.Contains(conf.Webhook.Types, req.Type) {
		return http.StatusBadRequest, nil, errors.Newf("webhook type %q is not allowed", req.Type)
	}
	w := &database.Webhook{RepoID: scope.RepoID, OrgID: scope.OrgID, HookTaskType: hookType, ContentType: database.JSON}
	code, errResp, err := applyWebhookRequest(l, w, req)
	if err != nil {
		log.Error("postWebhook: apply settings: %v", err)
		return http.StatusInternalServerError, nil, err
	} else if errResp != nil {
		return code, errResp, nil
	}
	if err = database.CreateWebhook(w); err != nil {
		log.Error("postWebhook: create webhook for repository %d or organization %d: %v", scope.RepoID, scope.OrgID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "create webhook")
	}
	return http.StatusCreated, &webhookIDResponse{ID: w.ID}, nil
}

func patchWebhook(l i18n.Locale, w *database.Webhook, req webhookRequest) (statusCode int, resp any, err error) {
	code, errResp, err := applyWebhookRequest(l, w, req)
	if err != nil {
		log.Error("patchWebhook: apply settings to webhook %d: %v", w.ID, err)
		return http.StatusInternalServerError, nil, err
	} else if errResp != nil {
		return code, errResp, nil
	}
	if err = database.UpdateWebhook(w); err != nil {
		log.Error("patchWebhook: update webhook %d: %v", w.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update webhook")
	}
	return http.StatusNoContent, nil, nil
}

func deleteWebhook(scope *webhookScope, w *database.Webhook) (statusCode int, resp any, err error) {
	if scope.RepoID > 0 {
		err = database.DeleteWebhookOfRepoByID(scope.RepoID, w.ID)
	} else {
		err = database.DeleteWebhookOfOrgByID(scope.OrgID, w.ID)
	}
	if err != nil {
		log.Error("deleteWebhook: delete webhook %d: %v", w.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "delete webhook")
	}
	return http.StatusNoContent, nil, nil
}

// usernameByEmail returns the name of the user with the email, or an empty
// string when no user has it.
func usernameByEmail(ctx context.Context, email string) (string, error) {
	u, err := database.Handle.Users().GetByEmail(ctx, email)
	if err != nil {
		if database.IsErrUserNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return u.Name, nil
}

// testPushPayload returns a push event for the latest commit on the default
// branch of the repository, or for a fake commit when the repository is
// empty.
func testPushPayload(ctx context.Context, user *database.User, rc *repoAdminContext) (*apiv1types.WebhookPushPayload, error) {
	repo := rc.Repo
	ghost := database.NewGhostUser()
	commit := &git.Commit{
		Author:  &git.Signature{Name: ghost.DisplayName(), Email: ghost.Email, When: time.Now()},
		Message: "This is a fake commit",
	}
	commit.Committer = commit.Author
	authorUsername, committerUsername := ghost.Name, ghost.Name
	nameStatus := &git.NameStatus{}

	repoPath := repox.RepositoryPath(rc.Owner.Name, repo.Name)
	var commitID string
	if repo.IsEmpty {
		objectFormat, err := git.RepoObjectFormat(ctx, repoPath)
		if err != nil {
			return nil, errors.Wrap(err, "get object format")
		}
		commitID = objectFormat.EmptyID()
	} else {
		gitRepo, err := git.Open(repoPath)
		if err != nil {
			return nil, errors.Wrap(err, "open repository")
		}
		if commit, err = gitRepo.BranchCommit(ctx, repo.DefaultBranch); err != nil {
			return nil, errors.Wrap(err, "get latest commit")
		}
		commitID = commit.ID.String()
		if authorUsername, err = usernameByEmail(ctx, commit.Author.Email); err != nil {
			return nil, errors.Wrap(err, "get author")
		}
		if committerUsername, err = usernameByEmail(ctx, commit.Committer.Email); err != nil {
			return nil, errors.Wrap(err, "get committer")
		}
		if nameStatus, err = commit.ShowNameStatus(ctx); err != nil {
			return nil, errors.Wrap(err, "get changed files")
		}
	}

	apiUser := user.APIFormat()
	return &apiv1types.WebhookPushPayload{
		Ref:    git.RefsHeads + repo.DefaultBranch,
		Before: commitID,
		After:  commitID,
		Commits: []*apiv1types.WebhookPayloadCommit{
			{
				ID:      commitID,
				Message: commit.Message,
				URL:     repo.HTMLURL() + "/commit/" + commitID,
				Author: &apiv1types.WebhookPayloadUser{
					Name:     commit.Author.Name,
					Email:    commit.Author.Email,
					UserName: authorUsername,
				},
				Committer: &apiv1types.WebhookPayloadUser{
					Name:     commit.Committer.Name,
					Email:    commit.Committer.Email,
					UserName: committerUsername,
				},
				Added:    nameStatus.Added,
				Removed:  nameStatus.Removed,
				Modified: nameStatus.Modified,
			},
		},
		Repo:   repo.APIFormatLegacy(nil),
		Pusher: apiUser,
		Sender: apiUser,
	}, nil
}

// postRepoWebhookTest queues a push event to the webhook of a repository.
func postRepoWebhookTest(c flamego.Context, user *database.User, rc *repoAdminContext, w *database.Webhook) (statusCode int, resp any, err error) {
	p, err := testPushPayload(c.Request().Context(), user, rc)
	if err != nil {
		log.Error("postRepoWebhookTest: build payload for repository %d: %v", rc.Repo.ID, err)
		return http.StatusInternalServerError, nil, err
	}
	if err = database.TestWebhook(rc.Repo, database.HookEventTypePush, p, w.ID); err != nil {
		log.Error("postRepoWebhookTest: queue delivery of webhook %d: %v", w.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "test webhook")
	}
	return http.StatusNoContent, nil, nil
}

// postRepoWebhookRedelivery queues a past delivery of the webhook of a
// repository again.
func postRepoWebhookRedelivery(c flamego.Context, rc *repoAdminContext, w *database.Webhook) (statusCode int, resp any, err error) {
	task, err := database.GetHookTaskOfWebhookByUUID(w.ID, c.Param("uuid"))
	if err != nil {
		if database.IsHookTaskNotExist(err) {
			return http.StatusNotFound, nil, errors.New("delivery does not exist")
		}
		log.Error("postRepoWebhookRedelivery: get delivery %q of webhook %d: %v", c.Param("uuid"), w.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "get delivery")
	}
	task.IsDelivered = false
	if err = database.UpdateHookTask(task); err != nil {
		log.Error("postRepoWebhookRedelivery: update delivery %d: %v", task.ID, err)
		return http.StatusInternalServerError, nil, errors.Wrap(err, "update delivery")
	}
	go database.HookQueue.Add(rc.Repo.ID)
	return http.StatusNoContent, nil, nil
}
