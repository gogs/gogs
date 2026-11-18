package database

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/netx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
	"gogs.io/gogs/internal/sync"
)

var HookQueue = sync.NewUniqueQueue(1000)

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
	// A redelivery succeeds only when this attempt does.
	t.IsSucceed = false

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

// HookTasksStore is the storage layer for deliveries of webhooks.
type HookTasksStore struct {
	db *gorm.DB
}

func newHookTasksStore(db *gorm.DB) *HookTasksStore {
	return &HookTasksStore{db: db}
}

// ListByWebhook returns the deliveries of the webhook with given ID, newest
// first, on given page of given size.
func (s *HookTasksStore) ListByWebhook(ctx context.Context, webhookID int64, page, pageSize int) ([]*HookTask, error) {
	if page < 1 {
		page = 1
	}
	db := s.db.WithContext(ctx).Where("hook_id = ?", webhookID).Order("id DESC")
	// GORM sends "LIMIT 0" for a page size of 0, which would list nothing.
	if pageSize > 0 {
		db = db.Limit(pageSize).Offset((page - 1) * pageSize)
	}
	var tasks []*HookTask
	return tasks, db.Find(&tasks).Error
}

// GetByUUID returns the delivery of the webhook with given UUID. It returns
// ErrHookTaskNotExist when not found.
func (s *HookTasksStore) GetByUUID(ctx context.Context, webhookID int64, uuid string) (*HookTask, error) {
	task := new(HookTask)
	err := s.db.WithContext(ctx).Where("hook_id = ? AND uuid = ?", webhookID, uuid).First(task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHookTaskNotExist{args: map[string]any{"webhookID": webhookID, "uuid": uuid}}
		}
		return nil, err
	}
	return task, nil
}

// Update saves all fields of the delivery.
func (s *HookTasksStore) Update(ctx context.Context, t *HookTask) error {
	return s.db.WithContext(ctx).Model(t).Select("*").Updates(t).Error
}

// listUndelivered returns the deliveries that have not been made, of given
// repository, or of all repositories when repoID is 0.
func (s *HookTasksStore) listUndelivered(ctx context.Context, repoID int64) ([]*HookTask, error) {
	db := s.db.WithContext(ctx).Where("is_delivered = ?", false)
	if repoID > 0 {
		db = db.Where("repo_id = ?", repoID)
	}
	var tasks []*HookTask
	return tasks, db.Order("id ASC").Find(&tasks).Error
}

// Prepare creates a delivery of given event for each active webhook of the
// repository and of its owner, when the owner is an organization, that is
// subscribed to the event, and queues them.
func (s *WebhooksStore) Prepare(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader) error {
	webhooks, err := s.listActive(ctx, repo)
	if err != nil {
		return err
	}
	return s.prepareHookTasks(ctx, logger, repo, event, p, webhooks)
}

// Test creates a delivery of given event for the webhook with given ID of the
// repository when it is subscribed to the event, even if it is not active, and
// queues it. It returns ErrWebhookNotExist when the repository has no such
// webhook.
func (s *WebhooksStore) Test(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader, webhookID int64) error {
	webhook, err := s.GetOfRepo(ctx, repo.ID, webhookID)
	if err != nil {
		return err
	}
	return s.prepareHookTasks(ctx, logger, repo, event, p, []*Webhook{webhook})
}

// hasEvent returns true if the webhook is subscribed to given event.
func (w *Webhook) hasEvent(event HookEventType) bool {
	switch event {
	case HookEventTypeCreate:
		return w.HasCreateEvent()
	case HookEventTypeDelete:
		return w.HasDeleteEvent()
	case HookEventTypeFork:
		return w.HasForkEvent()
	case HookEventTypePush:
		return w.HasPushEvent()
	case HookEventTypeIssues:
		return w.HasIssuesEvent()
	case HookEventTypePullRequest:
		return w.HasPullRequestEvent()
	case HookEventTypeIssueComment:
		return w.HasIssueCommentEvent()
	case HookEventTypeRelease:
		return w.HasReleaseEvent()
	}
	return true
}

// prepareHookTasks creates a delivery of given event for each of given
// webhooks that is subscribed to the event, and queues them.
func (s *WebhooksStore) prepareHookTasks(ctx context.Context, logger *logx.Logger, repo *Repository, event HookEventType, p apiv1types.WebhookPayloader, webhooks []*Webhook) (err error) {
	if len(webhooks) == 0 {
		return nil
	}

	db := s.db.WithContext(ctx)
	for _, w := range webhooks {
		if !w.hasEvent(event) {
			continue
		}

		// Use separate objects so modifications won't be made on payload on
		// non-Gogs type hooks.
		var payloader apiv1types.WebhookPayloader
		switch w.HookTaskType {
		case SLACK:
			payloader, err = GetSlackPayload(p, event, w.Meta)
			if err != nil {
				return errors.Wrap(err, "get Slack payload")
			}
		case DISCORD:
			payloader, err = GetDiscordPayload(p, event, w.Meta)
			if err != nil {
				return errors.Wrap(err, "get Discord payload")
			}
		case DINGTALK:
			payloader, err = GetDingtalkPayload(p, event)
			if err != nil {
				return errors.Wrap(err, "get DingTalk payload")
			}
		default:
			payloader = p
		}

		data, err := payloader.JSONPayload()
		if err != nil {
			return errors.Wrap(err, "encode payload")
		}
		var signature string
		if len(w.Secret) > 0 {
			sig := hmac.New(sha256.New, []byte(w.Secret))
			_, _ = sig.Write(data)
			signature = hex.EncodeToString(sig.Sum(nil))
		}

		err = db.Create(&HookTask{
			RepoID:         repo.ID,
			HookID:         w.ID,
			UUID:           uuid.New().String(),
			Type:           w.HookTaskType,
			URL:            w.URL,
			Signature:      signature,
			PayloadContent: string(data),
			ContentType:    w.ContentType,
			EventType:      event,
			IsSSL:          w.IsSSL,
		}).Error
		if err != nil {
			return errors.Wrap(err, "create hook task")
		}
	}

	// It's safe to fail when the whole function is called during hook execution
	// because resource released after exit. Also, there is no process started to
	// consume this input during hook execution.
	go HookQueue.Add(repo.ID)
	logger.Debug("Prepared webhook deliveries", "repoID", repo.ID, "event", event)
	return nil
}
