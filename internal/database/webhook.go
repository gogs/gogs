package database

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	jsoniter "github.com/json-iterator/go"
	gouuid "github.com/satori/go.uuid"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	api "github.com/gogs/go-gogs-client"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/httplib"
	"gogs.io/gogs/internal/netutil"
	"gogs.io/gogs/internal/sync"
	"gogs.io/gogs/internal/testutil"
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
	URL          string `gorm:"type:text;column:url"`
	ContentType  HookContentType
	Secret       string     `gorm:"type:text"`
	Events       string     `gorm:"type:text"`
	*HookEvent   `gorm:"-"` // LEGACY [1.0]: Cannot ignore JSON (i.e. json:"-") here, it breaks old backup archive
	IsSSL        bool       `gorm:"column:is_ssl"`
	IsActive     bool
	HookTaskType HookTaskType
	Meta         string     `gorm:"type:text"` // store hook-specific attributes
	LastStatus   HookStatus // Last delivery status

	Created     time.Time `gorm:"-" json:"-"`
	CreatedUnix int64
	Updated     time.Time `gorm:"-" json:"-"`
	UpdatedUnix int64
}

func (w *Webhook) BeforeCreate(tx *gorm.DB) error {
	w.CreatedUnix = tx.NowFunc().Unix()
	w.UpdatedUnix = w.CreatedUnix
	return nil
}

func (w *Webhook) BeforeUpdate(tx *gorm.DB) error {
	w.UpdatedUnix = tx.NowFunc().Unix()
	return nil
}

func (w *Webhook) AfterFind(tx *gorm.DB) error {
	w.HookEvent = &HookEvent{}
	if err := jsoniter.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		log.Error("Unmarshal [%d]: %v", w.ID, err)
	}
	w.Created = time.Unix(w.CreatedUnix, 0).Local()
	w.Updated = time.Unix(w.UpdatedUnix, 0).Local()
	return nil
}

func (w *Webhook) SlackMeta() *SlackMeta {
	s := &SlackMeta{}
	if err := jsoniter.Unmarshal([]byte(w.Meta), s); err != nil {
		log.Error("Failed to get Slack meta [webhook_id: %d]: %v", w.ID, err)
	}
	return s
}

// History returns history of webhook by given conditions.
func (w *Webhook) History(page int) ([]*HookTask, error) {
	return HookTasks(w.ID, page)
}

// UpdateEvent handles conversion from HookEvent to Events.
func (w *Webhook) UpdateEvent() error {
	data, err := jsoniter.Marshal(w.HookEvent)
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

// CreateWebhook creates a new web hook.
func CreateWebhook(w *Webhook) error {
	return db.Create(w).Error
}

var _ errutil.NotFound = (*ErrWebhookNotExist)(nil)

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

// getWebhook uses argument bean as query condition,
// ID must be specified and do not assign unnecessary fields.
func getWebhook(bean *Webhook) (*Webhook, error) {
	err := db.Where(bean).First(bean).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrWebhookNotExist{args: map[string]any{"webhookID": bean.ID}}
		}
		return nil, err
	}
	return bean, nil
}

// GetWebhookByID returns webhook by given ID.
// Use this function with caution of accessing unauthorized webhook,
// which means should only be used in non-user interactive functions.
func GetWebhookByID(id int64) (*Webhook, error) {
	return getWebhook(&Webhook{
		ID: id,
	})
}

// GetWebhookOfRepoByID returns webhook of repository by given ID.
func GetWebhookOfRepoByID(repoID, id int64) (*Webhook, error) {
	return getWebhook(&Webhook{
		ID:     id,
		RepoID: repoID,
	})
}

// GetWebhookByOrgID returns webhook of organization by given ID.
func GetWebhookByOrgID(orgID, id int64) (*Webhook, error) {
	return getWebhook(&Webhook{
		ID:    id,
		OrgID: orgID,
	})
}

// getActiveWebhooksByRepoID returns all active webhooks of repository.
func getActiveWebhooksByRepoID(tx *gorm.DB, repoID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, tx.Where("repo_id = ? AND is_active = ?", repoID, true).Find(&webhooks).Error
}

// GetWebhooksByRepoID returns all webhooks of a repository.
func GetWebhooksByRepoID(repoID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, db.Where("repo_id = ?", repoID).Find(&webhooks).Error
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	return db.Model(w).Where("id = ?", w.ID).Updates(w).Error
}

// deleteWebhook uses argument bean as query condition,
// ID must be specified and do not assign unnecessary fields.
func deleteWebhook(bean *Webhook) error {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(bean).Error; err != nil {
			return err
		}
		if err := tx.Where("hook_id = ?", bean.ID).Delete(&HookTask{}).Error; err != nil {
			return err
		}
		return nil
	})
}

// DeleteWebhookOfRepoByID deletes webhook of repository by given ID.
func DeleteWebhookOfRepoByID(repoID, id int64) error {
	return deleteWebhook(&Webhook{
		ID:     id,
		RepoID: repoID,
	})
}

// DeleteWebhookOfOrgByID deletes webhook of organization by given ID.
func DeleteWebhookOfOrgByID(orgID, id int64) error {
	return deleteWebhook(&Webhook{
		ID:    id,
		OrgID: orgID,
	})
}

// GetWebhooksByOrgID returns all webhooks for an organization.
func GetWebhooksByOrgID(orgID int64) (ws []*Webhook, err error) {
	err = db.Where("org_id = ?", orgID).Find(&ws).Error
	return ws, err
}

// getActiveWebhooksByOrgID returns all active webhooks for an organization.
func getActiveWebhooksByOrgID(tx *gorm.DB, orgID int64) ([]*Webhook, error) {
	ws := make([]*Webhook, 0, 3)
	return ws, tx.Where("org_id = ? AND is_active = ?", orgID, true).Find(&ws).Error
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
	ID              int64
	RepoID          int64 `gorm:"index"`
	HookID          int64
	UUID            string
	Type            HookTaskType
	URL             string `gorm:"type:text"`
	Signature       string `gorm:"type:text"`
	api.Payloader   `gorm:"-" json:"-"`
	PayloadContent  string `gorm:"type:text"`
	ContentType     HookContentType
	EventType       HookEventType
	IsSSL           bool
	IsDelivered     bool
	Delivered       int64
	DeliveredString string `gorm:"-" json:"-"`

	// History info.
	IsSucceed       bool
	RequestContent  string        `gorm:"type:text"`
	RequestInfo     *HookRequest  `gorm:"-" json:"-"`
	ResponseContent string        `gorm:"type:text"`
	ResponseInfo    *HookResponse `gorm:"-" json:"-"`
}

func (t *HookTask) BeforeUpdate(tx *gorm.DB) error {
	if t.RequestInfo != nil {
		t.RequestContent = t.ToJSON(t.RequestInfo)
	}
	if t.ResponseInfo != nil {
		t.ResponseContent = t.ToJSON(t.ResponseInfo)
	}
	return nil
}

func (t *HookTask) AfterFind(tx *gorm.DB) error {
	t.DeliveredString = time.Unix(0, t.Delivered).Format("2006-01-02 15:04:05 MST")

	if t.RequestContent != "" {
		t.RequestInfo = &HookRequest{}
		if err := jsoniter.Unmarshal([]byte(t.RequestContent), t.RequestInfo); err != nil {
			log.Error("Unmarshal[%d]: %v", t.ID, err)
		}
	}

	if t.ResponseContent != "" {
		t.ResponseInfo = &HookResponse{}
		if err := jsoniter.Unmarshal([]byte(t.ResponseContent), t.ResponseInfo); err != nil {
			log.Error("Unmarshal [%d]: %v", t.ID, err)
		}
	}
	return nil
}

func (t *HookTask) ToJSON(v any) string {
	p, err := jsoniter.Marshal(v)
	if err != nil {
		log.Error("Marshal [%d]: %v", t.ID, err)
	}
	return string(p)
}

// HookTasks returns a list of hook tasks by given conditions.
func HookTasks(hookID int64, page int) ([]*HookTask, error) {
	tasks := make([]*HookTask, 0, conf.Webhook.PagingNum)
	return tasks, db.Where("hook_id = ?", hookID).Order("id DESC").Limit(conf.Webhook.PagingNum).Offset((page - 1) * conf.Webhook.PagingNum).Find(&tasks).Error
}

// createHookTask creates a new hook task,
// it handles conversion from Payload to PayloadContent.
func createHookTask(tx *gorm.DB, t *HookTask) error {
	data, err := t.JSONPayload()
	if err != nil {
		return err
	}
	t.UUID = gouuid.NewV4().String()
	t.PayloadContent = string(data)
	return tx.Create(t).Error
}

var _ errutil.NotFound = (*ErrHookTaskNotExist)(nil)

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

// GetHookTaskOfWebhookByUUID returns hook task of given webhook by UUID.
func GetHookTaskOfWebhookByUUID(webhookID int64, uuid string) (*HookTask, error) {
	hookTask := &HookTask{
		HookID: webhookID,
		UUID:   uuid,
	}
	err := db.Where("hook_id = ? AND uuid = ?", webhookID, uuid).First(hookTask).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrHookTaskNotExist{args: map[string]any{"webhookID": webhookID, "uuid": uuid}}
		}
		return nil, err
	}
	return hookTask, nil
}

// UpdateHookTask updates information of hook task.
func UpdateHookTask(t *HookTask) error {
	return db.Model(t).Where("id = ?", t.ID).Updates(t).Error
}

// prepareHookTasks adds list of webhooks to task queue.
func prepareHookTasks(tx *gorm.DB, repo *Repository, event HookEventType, p api.Payloader, webhooks []*Webhook) (err error) {
	if len(webhooks) == 0 {
		return nil
	}

	var payloader api.Payloader
	for _, w := range webhooks {
		switch event {
		case HookEventTypeCreate:
			if !w.HasCreateEvent() {
				continue
			}
		case HookEventTypeDelete:
			if !w.HasDeleteEvent() {
				continue
			}
		case HookEventTypeFork:
			if !w.HasForkEvent() {
				continue
			}
		case HookEventTypePush:
			if !w.HasPushEvent() {
				continue
			}
		case HookEventTypeIssues:
			if !w.HasIssuesEvent() {
				continue
			}
		case HookEventTypePullRequest:
			if !w.HasPullRequestEvent() {
				continue
			}
		case HookEventTypeIssueComment:
			if !w.HasIssueCommentEvent() {
				continue
			}
		case HookEventTypeRelease:
			if !w.HasReleaseEvent() {
				continue
			}
		}

		// Use separate objects so modifications won't be made on payload on non-Gogs type hooks.
		switch w.HookTaskType {
		case SLACK:
			payloader, err = GetSlackPayload(p, event, w.Meta)
			if err != nil {
				return errors.Newf("GetSlackPayload: %v", err)
			}
		case DISCORD:
			payloader, err = GetDiscordPayload(p, event, w.Meta)
			if err != nil {
				return errors.Newf("GetDiscordPayload: %v", err)
			}
		case DINGTALK:
			payloader, err = GetDingtalkPayload(p, event)
			if err != nil {
				return errors.Newf("GetDingtalkPayload: %v", err)
			}
		default:
			payloader = p
		}

		var signature string
		if len(w.Secret) > 0 {
			data, err := payloader.JSONPayload()
			if err != nil {
				log.Error("prepareWebhooks.JSONPayload: %v", err)
			}
			sig := hmac.New(sha256.New, []byte(w.Secret))
			_, _ = sig.Write(data)
			signature = hex.EncodeToString(sig.Sum(nil))
		}

		if err = createHookTask(tx, &HookTask{
			RepoID:      repo.ID,
			HookID:      w.ID,
			Type:        w.HookTaskType,
			URL:         w.URL,
			Signature:   signature,
			Payloader:   payloader,
			ContentType: w.ContentType,
			EventType:   event,
			IsSSL:       w.IsSSL,
		}); err != nil {
			return errors.Newf("createHookTask: %v", err)
		}
	}

	// It's safe to fail when the whole function is called during hook execution
	// because resource released after exit. Also, there is no process started to
	// consume this input during hook execution.
	go HookQueue.Add(repo.ID)
	return nil
}

func prepareWebhooks(tx *gorm.DB, repo *Repository, event HookEventType, p api.Payloader) error {
	webhooks, err := getActiveWebhooksByRepoID(tx, repo.ID)
	if err != nil {
		return errors.Newf("getActiveWebhooksByRepoID [%d]: %v", repo.ID, err)
	}

	// check if repo belongs to org and append additional webhooks
	if repo.mustOwner(tx).IsOrganization() {
		// get hooks for org
		orgws, err := getActiveWebhooksByOrgID(tx, repo.OwnerID)
		if err != nil {
			return errors.Newf("getActiveWebhooksByOrgID [%d]: %v", repo.OwnerID, err)
		}
		webhooks = append(webhooks, orgws...)
	}
	return prepareHookTasks(tx, repo, event, p, webhooks)
}

// PrepareWebhooks adds all active webhooks to task queue.
func PrepareWebhooks(repo *Repository, event HookEventType, p api.Payloader) error {
	// NOTE: To prevent too many cascading changes in a single refactoring PR, we
	// choose to ignore this function in tests.
	if db == nil && testutil.InTest {
		return nil
	}
	return prepareWebhooks(db, repo, event, p)
}

// TestWebhook adds the test webhook matches the ID to task queue.
func TestWebhook(repo *Repository, event HookEventType, p api.Payloader, webhookID int64) error {
	webhook, err := GetWebhookOfRepoByID(repo.ID, webhookID)
	if err != nil {
		return errors.Newf("GetWebhookOfRepoByID [repo_id: %d, id: %d]: %v", repo.ID, webhookID, err)
	}
	return prepareHookTasks(db, repo, event, p, []*Webhook{webhook})
}

func (t *HookTask) deliver() {
	payloadURL, err := url.Parse(t.URL)
	if err != nil {
		t.ResponseContent = fmt.Sprintf(`{"body": "Cannot parse payload URL: %v"}`, err)
		return
	}
	if netutil.IsBlockedLocalHostname(payloadURL.Hostname(), conf.Security.LocalNetworkAllowlist) {
		t.ResponseContent = `{"body": "Payload URL resolved to a local network address that is implicitly blocked."}`
		return
	}

	t.IsDelivered = true

	timeout := time.Duration(conf.Webhook.DeliverTimeout) * time.Second
	req := httplib.Post(t.URL).SetTimeout(timeout, timeout).
		Header("X-Github-Delivery", t.UUID).
		Header("X-Github-Event", string(t.EventType)).
		Header("X-Gogs-Delivery", t.UUID).
		Header("X-Gogs-Signature", t.Signature).
		Header("X-Gogs-Event", string(t.EventType)).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: conf.Webhook.SkipTLSVerify})

	switch t.ContentType {
	case JSON:
		req = req.Header("Content-Type", "application/json").Body(t.PayloadContent)
	case FORM:
		req.Param("payload", t.PayloadContent)
	}

	// Record delivery information.
	t.RequestInfo = &HookRequest{
		Headers: map[string]string{},
	}
	for k, vals := range req.Headers() {
		t.RequestInfo.Headers[k] = strings.Join(vals, ",")
	}

	t.ResponseInfo = &HookResponse{
		Headers: map[string]string{},
	}

	defer func() {
		t.Delivered = time.Now().UnixNano()
		if t.IsSucceed {
			log.Trace("Hook delivered: %s", t.UUID)
		} else {
			log.Trace("Hook delivery failed: %s", t.UUID)
		}

		// Update webhook last delivery status.
		w, err := GetWebhookByID(t.HookID)
		if err != nil {
			log.Error("GetWebhookByID: %v", err)
			return
		}
		if t.IsSucceed {
			w.LastStatus = HookStatusSucceed
		} else {
			w.LastStatus = HookStatusFailed
		}
		if err = UpdateWebhook(w); err != nil {
			log.Error("UpdateWebhook: %v", err)
			return
		}
	}()

	resp, err := req.Response()
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

// DeliverHooks checks and delivers undelivered hooks.
// TODO: shoot more hooks at same time.
func DeliverHooks() {
	tasks := make([]*HookTask, 0, 10)
	err := db.Where("is_delivered = ?", false).Find(&tasks).Error
	if err != nil {
		log.Error("Find undelivered hook tasks: %v", err)
	} else {
		for _, t := range tasks {
			t.deliver()
			if err := UpdateHookTask(t); err != nil {
				log.Error("UpdateHookTask [%d]: %v", t.ID, err)
			}
		}
	}

	// Start listening on new hook requests.
	for repoID := range HookQueue.Queue() {
		log.Trace("DeliverHooks [repo_id: %v]", repoID)
		HookQueue.Remove(repoID)

		tasks = make([]*HookTask, 0, 5)
		if err := db.Where("repo_id = ? AND is_delivered = ?", repoID, false).Find(&tasks).Error; err != nil {
			log.Error("Get repository [%s] hook tasks: %v", repoID, err)
			continue
		}
		for _, t := range tasks {
			t.deliver()
			if err := UpdateHookTask(t); err != nil {
				log.Error("UpdateHookTask [%d]: %v", t.ID, err)
				continue
			}
		}
	}
}

func InitDeliverHooks() {
	go DeliverHooks()
}
