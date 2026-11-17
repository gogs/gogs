package database

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/netx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/sync"
)

var HookQueue = sync.NewUniqueQueue(1000)

type HookContentType int

const (
	JSON HookContentType = iota + 1
	FORM
)

var hookContentTypes = map[string]HookContentType{
	"json": JSON,
	"form": FORM,
}

// ToHookContentType returns HookContentType by given name.
func ToHookContentType(name string) HookContentType {
	return hookContentTypes[name]
}

func (t HookContentType) Name() string {
	switch t {
	case JSON:
		return "json"
	case FORM:
		return "form"
	}
	return ""
}

// IsValidHookContentType returns true if given name is a valid hook content type.
func IsValidHookContentType(name string) bool {
	_, ok := hookContentTypes[name]
	return ok
}

type HookEvents struct {
	Create       bool `json:"create"`
	Delete       bool `json:"delete"`
	Fork         bool `json:"fork"`
	Push         bool `json:"push"`
	Issues       bool `json:"issues"`
	PullRequest  bool `json:"pull_request"`
	IssueComment bool `json:"issue_comment"`
	Release      bool `json:"release"`
}

// HookEvent represents events that will delivery hook.
type HookEvent struct {
	PushOnly       bool `json:"push_only"`
	SendEverything bool `json:"send_everything"`
	ChooseEvents   bool `json:"choose_events"`

	HookEvents `json:"events"`
}

type HookStatus int

const (
	HookStatusNone = iota
	HookStatusSucceed
	HookStatusFailed
)

// Webhook represents a web hook object.
type Webhook struct {
	ID           int64
	RepoID       int64
	OrgID        int64
	URL          string          `gorm:"type:TEXT"`
	ContentType  HookContentType `gorm:"size:32"`
	Secret       string          `gorm:"type:TEXT"`
	Events       string          `gorm:"type:TEXT"`
	*HookEvent   `gorm:"-"`      // LEGACY [1.0]: Cannot ignore JSON (i.e. json:"-") here, it breaks old backup archive
	IsSSL        bool
	IsActive     bool
	HookTaskType HookTaskType `gorm:"size:32"`
	Meta         string       `gorm:"type:TEXT"` // store hook-specific attributes
	LastStatus   HookStatus   `gorm:"size:32"`   // Last delivery status

	Created     time.Time `json:"-" gorm:"-"`
	CreatedUnix int64
	Updated     time.Time `json:"-" gorm:"-"`
	UpdatedUnix int64
}

// BeforeCreate implements the GORM create hook.
func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	if w.CreatedUnix == 0 {
		w.CreatedUnix = tx.NowFunc().Unix()
		w.UpdatedUnix = w.CreatedUnix
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (w *Webhook) BeforeUpdate(tx *gorm.DB) error {
	w.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (w *Webhook) AfterFind(_ *gorm.DB) error {
	w.HookEvent = &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		handleLogger().Error("Failed to unmarshal webhook events", "webhookID", w.ID, "error", err)
	}
	w.Created = time.Unix(w.CreatedUnix, 0).Local()
	w.Updated = time.Unix(w.UpdatedUnix, 0).Local()
	return nil
}

func (w *Webhook) SlackMeta() *SlackMeta {
	s := &SlackMeta{}
	if err := json.Unmarshal([]byte(w.Meta), s); err != nil {
		handleLogger().Error("Failed to get Slack meta", "webhookID", w.ID, "error", err)
	}
	return s
}

// UpdateEvent handles conversion from HookEvent to Events.
func (w *Webhook) UpdateEvent() error {
	data, err := json.Marshal(w.HookEvent)
	w.Events = string(data)
	return err
}

// HasCreateEvent returns true if hook enabled create event.
func (w *Webhook) HasCreateEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Create)
}

// HasDeleteEvent returns true if hook enabled delete event.
func (w *Webhook) HasDeleteEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Delete)
}

// HasForkEvent returns true if hook enabled fork event.
func (w *Webhook) HasForkEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Fork)
}

// HasPushEvent returns true if hook enabled push event.
func (w *Webhook) HasPushEvent() bool {
	return w.PushOnly || w.SendEverything ||
		(w.ChooseEvents && w.Push)
}

// HasIssuesEvent returns true if hook enabled issues event.
func (w *Webhook) HasIssuesEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Issues)
}

// HasPullRequestEvent returns true if hook enabled pull request event.
func (w *Webhook) HasPullRequestEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.PullRequest)
}

// HasIssueCommentEvent returns true if hook enabled issue comment event.
func (w *Webhook) HasIssueCommentEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.IssueComment)
}

// HasReleaseEvent returns true if hook enabled release event.
func (w *Webhook) HasReleaseEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.Release)
}

type eventChecker struct {
	checker func() bool
	typ     HookEventType
}

func (w *Webhook) EventsArray() []string {
	events := make([]string, 0, 8)
	eventCheckers := []eventChecker{
		{w.HasCreateEvent, HookEventTypeCreate},
		{w.HasDeleteEvent, HookEventTypeDelete},
		{w.HasForkEvent, HookEventTypeFork},
		{w.HasPushEvent, HookEventTypePush},
		{w.HasIssuesEvent, HookEventTypeIssues},
		{w.HasPullRequestEvent, HookEventTypePullRequest},
		{w.HasIssueCommentEvent, HookEventTypeIssueComment},
		{w.HasReleaseEvent, HookEventTypeRelease},
	}
	for _, c := range eventCheckers {
		if c.checker() {
			events = append(events, string(c.typ))
		}
	}
	return events
}

var _ errx.NotFound = (*ErrWebhookNotExist)(nil)

type ErrWebhookNotExist struct {
	args map[string]any
}

func IsErrWebhookNotExist(err error) bool {
	_, ok := err.(ErrWebhookNotExist)
	return ok
}

func (err ErrWebhookNotExist) Error() string {
	return fmt.Sprintf("webhook does not exist: %v", err.args)
}

func (ErrWebhookNotExist) NotFound() bool {
	return true
}

//   ___ ___                __   ___________              __
//  /   |   \  ____   ____ |  | _\__    ___/____    _____|  | __
// /    ~    \/  _ \ /  _ \|  |/ / |    |  \__  \  /  ___/  |/ /
// \    Y    (  <_> |  <_> )    <  |    |   / __ \_\___ \|    <
//  \___|_  / \____/ \____/|__|_ \ |____|  (____  /____  >__|_ \
//        \/                    \/              \/     \/     \/

type HookTaskType int

const (
	GOGS HookTaskType = iota + 1
	SLACK
	DISCORD
	DINGTALK
)

var hookTaskTypes = map[string]HookTaskType{
	"gogs":     GOGS,
	"slack":    SLACK,
	"discord":  DISCORD,
	"dingtalk": DINGTALK,
}

// ToHookTaskType returns HookTaskType by given name.
func ToHookTaskType(name string) HookTaskType {
	return hookTaskTypes[name]
}

func (t HookTaskType) Name() string {
	switch t {
	case GOGS:
		return "gogs"
	case SLACK:
		return "slack"
	case DISCORD:
		return "discord"
	case DINGTALK:
		return "dingtalk"
	}
	return ""
}

// IsValidHookTaskType returns true if given name is a valid hook task type.
func IsValidHookTaskType(name string) bool {
	_, ok := hookTaskTypes[name]
	return ok
}

type HookEventType string

const (
	HookEventTypeCreate       HookEventType = "create"
	HookEventTypeDelete       HookEventType = "delete"
	HookEventTypeFork         HookEventType = "fork"
	HookEventTypePush         HookEventType = "push"
	HookEventTypeIssues       HookEventType = "issues"
	HookEventTypePullRequest  HookEventType = "pull_request"
	HookEventTypeIssueComment HookEventType = "issue_comment"
	HookEventTypeRelease      HookEventType = "release"
)

// HookRequest represents hook task request information.
type HookRequest struct {
	Headers map[string]string `json:"headers"`
}

// HookResponse represents hook task response information.
type HookResponse struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    string            `json:"body"`
}

// HookTask represents a hook task.
type HookTask struct {
	ID                          int64
	RepoID                      int64 `gorm:"index:IDX_hook_task_repo_id"`
	HookID                      int64
	UUID                        string       `gorm:"size:255"`
	Type                        HookTaskType `gorm:"size:32"`
	URL                         string       `gorm:"type:TEXT"`
	Signature                   string       `gorm:"type:TEXT"`
	apiv1types.WebhookPayloader `json:"-" gorm:"-"`
	PayloadContent              string          `gorm:"type:TEXT"`
	ContentType                 HookContentType `gorm:"size:32"`
	EventType                   HookEventType   `gorm:"size:255"`
	IsSSL                       bool
	IsDelivered                 bool
	Delivered                   int64
	DeliveredString             string `json:"-" gorm:"-"`

	// History info.
	IsSucceed       bool
	RequestContent  string        `gorm:"type:TEXT"`
	RequestInfo     *HookRequest  `json:"-" gorm:"-"`
	ResponseContent string        `gorm:"type:TEXT"`
	ResponseInfo    *HookResponse `json:"-" gorm:"-"`
}

// BeforeUpdate implements the GORM update hook.
func (t *HookTask) BeforeUpdate(_ *gorm.DB) error {
	if t.RequestInfo != nil {
		t.RequestContent = t.ToJSON(t.RequestInfo)
	}
	if t.ResponseInfo != nil {
		t.ResponseContent = t.ToJSON(t.ResponseInfo)
	}
	return nil
}

// AfterFind implements the GORM query hook.
func (t *HookTask) AfterFind(_ *gorm.DB) error {
	t.DeliveredString = time.Unix(0, t.Delivered).Format("2006-01-02 15:04:05 MST")

	if t.RequestContent != "" {
		t.RequestInfo = &HookRequest{}
		if err := json.Unmarshal([]byte(t.RequestContent), t.RequestInfo); err != nil {
			handleLogger().Error("Failed to unmarshal hook task request", "hookTaskID", t.ID, "error", err)
		}
	}

	if t.ResponseContent != "" {
		t.ResponseInfo = &HookResponse{}
		if err := json.Unmarshal([]byte(t.ResponseContent), t.ResponseInfo); err != nil {
			handleLogger().Error("Failed to unmarshal hook task response", "hookTaskID", t.ID, "error", err)
		}
	}
	return nil
}

func (t *HookTask) ToJSON(v any) string {
	p, err := json.Marshal(v)
	if err != nil {
		handleLogger().Error("Failed to marshal JSON", "hookTaskID", t.ID, "error", err)
	}
	return string(p)
}

var _ errx.NotFound = (*ErrHookTaskNotExist)(nil)

type ErrHookTaskNotExist struct {
	args map[string]any
}

func IsHookTaskNotExist(err error) bool {
	_, ok := err.(ErrHookTaskNotExist)
	return ok
}

func (err ErrHookTaskNotExist) Error() string {
	return fmt.Sprintf("hook task does not exist: %v", err.args)
}

func (ErrHookTaskNotExist) NotFound() bool {
	return true
}

func (t *HookTask) deliver(ctx context.Context, logger *logx.Logger, webhooks *WebhooksStore) {
	payloadURL, err := url.Parse(t.URL)
	if err != nil {
		t.ResponseContent = fmt.Sprintf(`{"body": "Cannot parse payload URL: %v"}`, err)
		return
	}
	if netx.IsBlockedLocalHostname(payloadURL.Hostname(), conf.Security.LocalNetworkAllowlist) {
		t.ResponseContent = `{"body": "Payload URL resolved to a local network address that is implicitly blocked."}`
		return
	}

	var body, contentType string
	switch t.ContentType {
	case JSON:
		body, contentType = t.PayloadContent, "application/json"
	case FORM:
		body, contentType = url.Values{"payload": {t.PayloadContent}}.Encode(), "application/x-www-form-urlencoded"
	}
	req, err := http.NewRequest(http.MethodPost, t.URL, strings.NewReader(body))
	if err != nil {
		t.ResponseContent = fmt.Sprintf(`{"body": "Cannot create request: %v"}`, err)
		return
	}
	req.Header.Set("User-Agent", "GogsServer")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("X-Github-Delivery", t.UUID)
	req.Header.Set("X-Github-Event", string(t.EventType))
	req.Header.Set("X-Gogs-Delivery", t.UUID)
	req.Header.Set("X-Gogs-Signature", t.Signature)
	req.Header.Set("X-Gogs-Event", string(t.EventType))

	t.IsDelivered = true

	// Record delivery information.
	t.RequestInfo = &HookRequest{
		Headers: map[string]string{},
	}
	for k, vals := range req.Header {
		t.RequestInfo.Headers[k] = strings.Join(vals, ",")
	}

	t.ResponseInfo = &HookResponse{
		Headers: map[string]string{},
	}

	defer func() {
		t.Delivered = time.Now().UnixNano()
		if t.IsSucceed {
			logger.Debug("Hook delivered", "uuid", t.UUID)
		} else {
			logger.Debug("Hook delivery failed", "uuid", t.UUID)
		}

		status := HookStatus(HookStatusFailed)
		if t.IsSucceed {
			status = HookStatusSucceed
		}
		if err := webhooks.setLastStatus(ctx, t.HookID, status); err != nil {
			logger.Error("Failed to update last status of webhook", "webhookID", t.HookID, "error", err)
		}
	}()

	client := &http.Client{
		Timeout: time.Duration(conf.Webhook.DeliverTimeout) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: conf.Webhook.SkipTLSVerify},
			DisableKeepAlives: true,
		},
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			// The webhook target is explicitly configured by the user, so any
			// redirect would silently retarget the signed payload. Refuse all
			// redirects rather than chase them.
			return errors.Newf("refusing to follow webhook redirect to %q", req.URL.Redacted())
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.ResponseInfo.Body = fmt.Sprintf("Delivery: %v", err)
		return
	}
	defer resp.Body.Close()

	// Status code is 20x can be seen as succeed.
	t.IsSucceed = resp.StatusCode/100 == 2
	t.ResponseInfo.Status = resp.StatusCode
	for k, vals := range resp.Header {
		t.ResponseInfo.Headers[k] = strings.Join(vals, ",")
	}

	p, err := io.ReadAll(resp.Body)
	if err != nil {
		t.ResponseInfo.Body = fmt.Sprintf("read body: %s", err)
		return
	}
	t.ResponseInfo.Body = string(p)
}

// DeliverHooks delivers the deliveries that have not been made, and then
// those of each repository that is queued.
// TODO: shoot more hooks at same time.
func DeliverHooks(logger *logx.Logger) {
	ctx := context.Background()
	webhooks, hookTasks := Handle.Webhooks(), Handle.HookTasks()
	deliver := func(repoID int64) {
		tasks, err := hookTasks.listUndelivered(ctx, repoID)
		if err != nil {
			logger.Error("Failed to list undelivered hook tasks", "repoID", repoID, "error", err)
			return
		}
		for _, t := range tasks {
			t.deliver(ctx, logger, webhooks)
			if err = hookTasks.Update(ctx, t); err != nil {
				logger.Error("Failed to update hook task", "hookTaskID", t.ID, "error", err)
			}
		}
	}

	deliver(0)

	// Start listening on new hook requests.
	for repoID := range HookQueue.Queue() {
		logger.Debug("Delivering hooks", "repoID", repoID)
		HookQueue.Remove(repoID)
		id, err := strconv.ParseInt(repoID, 10, 64)
		if err != nil {
			logger.Error("Failed to parse queued repository ID", "repoID", repoID, "error", err)
			continue
		}
		deliver(id)
	}
}

func InitDeliverHooks(logger *logx.Logger) {
	go DeliverHooks(logger)
}
