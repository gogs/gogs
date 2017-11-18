// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"github.com/go-xorm/xorm"
	gouuid "github.com/satori/go.uuid"
	log "gopkg.in/clog.v1"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/models/errors"
	"github.com/gogits/gogs/pkg/httplib"
	"github.com/gogits/gogs/pkg/setting"
	"github.com/gogits/gogs/pkg/sync"
)

var HookQueue = sync.NewUniqueQueue(setting.Webhook.QueueLength)

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
	IssueComment bool `json:"issue_comment"`
	PullRequest  bool `json:"pull_request"`
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
	HOOK_STATUS_NONE = iota
	HOOK_STATUS_SUCCEED
	HOOK_STATUS_FAILED
)

// Webhook represents a web hook object.
type Webhook struct {
	ID           int64
	RepoID       int64
	OrgID        int64
	URL          string `xorm:"url TEXT"`
	ContentType  HookContentType
	Secret       string `xorm:"TEXT"`
	Events       string `xorm:"TEXT"`
	*HookEvent   `xorm:"-"`
	IsSSL        bool `xorm:"is_ssl"`
	IsActive     bool
	HookTaskType HookTaskType
	Meta         string     `xorm:"TEXT"` // store hook-specific attributes
	LastStatus   HookStatus // Last delivery status

	Created     time.Time `xorm:"-"`
	CreatedUnix int64
	Updated     time.Time `xorm:"-"`
	UpdatedUnix int64
}

func (w *Webhook) BeforeInsert() {
	w.CreatedUnix = time.Now().Unix()
	w.UpdatedUnix = w.CreatedUnix
}

func (w *Webhook) BeforeUpdate() {
	w.UpdatedUnix = time.Now().Unix()
}

func (w *Webhook) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "events":
		w.HookEvent = &HookEvent{}
		if err = json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
			log.Error(3, "Unmarshal [%d]: %v", w.ID, err)
		}
	case "created_unix":
		w.Created = time.Unix(w.CreatedUnix, 0).Local()
	case "updated_unix":
		w.Updated = time.Unix(w.UpdatedUnix, 0).Local()
	}
}

func (w *Webhook) GetSlackHook() *SlackMeta {
	s := &SlackMeta{}
	if err := json.Unmarshal([]byte(w.Meta), s); err != nil {
		log.Error(2, "GetSlackHook [%d]: %v", w.ID, err)
	}
	return s
}

// History returns history of webhook by given conditions.
func (w *Webhook) History(page int) ([]*HookTask, error) {
	return HookTasks(w.ID, page)
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
		(w.ChooseEvents && w.HookEvents.Create)
}

// HasDeleteEvent returns true if hook enabled delete event.
func (w *Webhook) HasDeleteEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Delete)
}

// HasForkEvent returns true if hook enabled fork event.
func (w *Webhook) HasForkEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Fork)
}

// HasPushEvent returns true if hook enabled push event.
func (w *Webhook) HasPushEvent() bool {
	return w.PushOnly || w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Push)
}

// HasIssuesEvent returns true if hook enabled issues event.
func (w *Webhook) HasIssuesEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Issues)
}

// HasIssueCommentEvent returns true if hook enabled issue comment event.
func (w *Webhook) HasIssueCommentEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.IssueComment)
}

// HasPullRequestEvent returns true if hook enabled pull request event.
func (w *Webhook) HasPullRequestEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.PullRequest)
}

// HasReleaseEvent returns true if hook enabled release event.
func (w *Webhook) HasReleaseEvent() bool {
	return w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Release)
}

type eventChecker struct {
	checker func() bool
	typ     HookEventType
}

func (w *Webhook) EventsArray() []string {
	events := make([]string, 0, 7)
	eventCheckers := []eventChecker{
		{w.HasCreateEvent, HOOK_EVENT_CREATE},
		{w.HasDeleteEvent, HOOK_EVENT_DELETE},
		{w.HasForkEvent, HOOK_EVENT_FORK},
		{w.HasPushEvent, HOOK_EVENT_PUSH},
		{w.HasIssuesEvent, HOOK_EVENT_ISSUES},
		{w.HasIssueCommentEvent, HOOK_EVENT_ISSUE_COMMENT},
		{w.HasPullRequestEvent, HOOK_EVENT_PULL_REQUEST},
		{w.HasReleaseEvent, HOOK_EVENT_RELEASE},
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
	_, err := x.Insert(w)
	return err
}

// getWebhook uses argument bean as query condition,
// ID must be specified and do not assign unnecessary fields.
func getWebhook(bean *Webhook) (*Webhook, error) {
	has, err := x.Get(bean)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, errors.WebhookNotExist{bean.ID}
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
func getActiveWebhooksByRepoID(e Engine, repoID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, e.Where("repo_id = ?", repoID).And("is_active = ?", true).Find(&webhooks)
}

// GetWebhooksByRepoID returns all webhooks of a repository.
func GetWebhooksByRepoID(repoID int64) ([]*Webhook, error) {
	webhooks := make([]*Webhook, 0, 5)
	return webhooks, x.Find(&webhooks, &Webhook{RepoID: repoID})
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	_, err := x.Id(w.ID).AllCols().Update(w)
	return err
}

// deleteWebhook uses argument bean as query condition,
// ID must be specified and do not assign unnecessary fields.
func deleteWebhook(bean *Webhook) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Delete(bean); err != nil {
		return err
	} else if _, err = sess.Delete(&HookTask{HookID: bean.ID}); err != nil {
		return err
	}

	return sess.Commit()
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
	err = x.Find(&ws, &Webhook{OrgID: orgID})
	return ws, err
}

// getActiveWebhooksByOrgID returns all active webhooks for an organization.
func getActiveWebhooksByOrgID(e Engine, orgID int64) ([]*Webhook, error) {
	ws := make([]*Webhook, 0, 3)
	return ws, e.Where("org_id=?", orgID).And("is_active=?", true).Find(&ws)
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
	HOOK_EVENT_CREATE        HookEventType = "create"
	HOOK_EVENT_DELETE        HookEventType = "delete"
	HOOK_EVENT_FORK          HookEventType = "fork"
	HOOK_EVENT_PUSH          HookEventType = "push"
	HOOK_EVENT_ISSUES        HookEventType = "issues"
	HOOK_EVENT_ISSUE_COMMENT HookEventType = "issue_comment"
	HOOK_EVENT_PULL_REQUEST  HookEventType = "pull_request"
	HOOK_EVENT_RELEASE       HookEventType = "release"
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
	RepoID          int64 `xorm:"INDEX"`
	HookID          int64
	UUID            string
	Type            HookTaskType
	URL             string `xorm:"TEXT"`
	Signature       string `xorm:"TEXT"`
	api.Payloader   `xorm:"-"`
	PayloadContent  string `xorm:"TEXT"`
	ContentType     HookContentType
	EventType       HookEventType
	IsSSL           bool
	IsDelivered     bool
	Delivered       int64
	DeliveredString string `xorm:"-"`

	// History info.
	IsSucceed       bool
	RequestContent  string        `xorm:"TEXT"`
	RequestInfo     *HookRequest  `xorm:"-"`
	ResponseContent string        `xorm:"TEXT"`
	ResponseInfo    *HookResponse `xorm:"-"`
}

func (t *HookTask) BeforeUpdate() {
	if t.RequestInfo != nil {
		t.RequestContent = t.MarshalJSON(t.RequestInfo)
	}
	if t.ResponseInfo != nil {
		t.ResponseContent = t.MarshalJSON(t.ResponseInfo)
	}
}

func (t *HookTask) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "delivered":
		t.DeliveredString = time.Unix(0, t.Delivered).Format("2006-01-02 15:04:05 MST")

	case "request_content":
		if len(t.RequestContent) == 0 {
			return
		}

		t.RequestInfo = &HookRequest{}
		if err = json.Unmarshal([]byte(t.RequestContent), t.RequestInfo); err != nil {
			log.Error(3, "Unmarshal[%d]: %v", t.ID, err)
		}

	case "response_content":
		if len(t.ResponseContent) == 0 {
			return
		}

		t.ResponseInfo = &HookResponse{}
		if err = json.Unmarshal([]byte(t.ResponseContent), t.ResponseInfo); err != nil {
			log.Error(3, "Unmarshal [%d]: %v", t.ID, err)
		}
	}
}

func (t *HookTask) MarshalJSON(v interface{}) string {
	p, err := json.Marshal(v)
	if err != nil {
		log.Error(3, "Marshal [%d]: %v", t.ID, err)
	}
	return string(p)
}

// HookTasks returns a list of hook tasks by given conditions.
func HookTasks(hookID int64, page int) ([]*HookTask, error) {
	tasks := make([]*HookTask, 0, setting.Webhook.PagingNum)
	return tasks, x.Limit(setting.Webhook.PagingNum, (page-1)*setting.Webhook.PagingNum).Where("hook_id=?", hookID).Desc("id").Find(&tasks)
}

// createHookTask creates a new hook task,
// it handles conversion from Payload to PayloadContent.
func createHookTask(e Engine, t *HookTask) error {
	data, err := t.Payloader.JSONPayload()
	if err != nil {
		return err
	}
	t.UUID = gouuid.NewV4().String()
	t.PayloadContent = string(data)
	_, err = e.Insert(t)
	return err
}

// GetHookTaskOfWebhookByUUID returns hook task of given webhook by UUID.
func GetHookTaskOfWebhookByUUID(webhookID int64, uuid string) (*HookTask, error) {
	hookTask := &HookTask{
		HookID: webhookID,
		UUID:   uuid,
	}
	has, err := x.Get(hookTask)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, errors.HookTaskNotExist{webhookID, uuid}
	}
	return hookTask, nil
}

// UpdateHookTask updates information of hook task.
func UpdateHookTask(t *HookTask) error {
	_, err := x.Id(t.ID).AllCols().Update(t)
	return err
}

// prepareHookTasks adds list of webhooks to task queue.
func prepareHookTasks(e Engine, repo *Repository, event HookEventType, p api.Payloader, webhooks []*Webhook) (err error) {
	if len(webhooks) == 0 {
		return nil
	}

	var payloader api.Payloader
	for _, w := range webhooks {
		switch event {
		case HOOK_EVENT_CREATE:
			if !w.HasCreateEvent() {
				continue
			}
		case HOOK_EVENT_DELETE:
			if !w.HasDeleteEvent() {
				continue
			}
		case HOOK_EVENT_FORK:
			if !w.HasForkEvent() {
				continue
			}
		case HOOK_EVENT_PUSH:
			if !w.HasPushEvent() {
				continue
			}
		case HOOK_EVENT_ISSUES:
			if !w.HasIssuesEvent() {
				continue
			}
		case HOOK_EVENT_ISSUE_COMMENT:
			if !w.HasIssueCommentEvent() {
				continue
			}
		case HOOK_EVENT_PULL_REQUEST:
			if !w.HasPullRequestEvent() {
				continue
			}
		case HOOK_EVENT_RELEASE:
			if !w.HasReleaseEvent() {
				continue
			}
		}

		// Use separate objects so modifcations won't be made on payload on non-Gogs type hooks.
		switch w.HookTaskType {
		case SLACK:
			payloader, err = GetSlackPayload(p, event, w.Meta)
			if err != nil {
				return fmt.Errorf("GetSlackPayload: %v", err)
			}
		case DISCORD:
			payloader, err = GetDiscordPayload(p, event, w.Meta)
			if err != nil {
				return fmt.Errorf("GetDiscordPayload: %v", err)
			}
		case DINGTALK:
			payloader, err = GetDingtalkPayload(p, event)
			if err != nil {
				return fmt.Errorf("GetDingtalkPayload: %v", err)
			}
		default:
			payloader = p
		}

		var signature string
		if len(w.Secret) > 0 {
			data, err := payloader.JSONPayload()
			if err != nil {
				log.Error(2, "prepareWebhooks.JSONPayload: %v", err)
			}
			sig := hmac.New(sha256.New, []byte(w.Secret))
			sig.Write(data)
			signature = hex.EncodeToString(sig.Sum(nil))
		}

		if err = createHookTask(e, &HookTask{
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
			return fmt.Errorf("createHookTask: %v", err)
		}
	}

	// It's safe to fail when the whole function is called during hook execution
	// because resource released after exit. Also, there is no process started to
	// consume this input during hook execution.
	go HookQueue.Add(repo.ID)
	return nil
}

func prepareWebhooks(e Engine, repo *Repository, event HookEventType, p api.Payloader) error {
	webhooks, err := getActiveWebhooksByRepoID(e, repo.ID)
	if err != nil {
		return fmt.Errorf("getActiveWebhooksByRepoID [%d]: %v", repo.ID, err)
	}

	// check if repo belongs to org and append additional webhooks
	if repo.mustOwner(e).IsOrganization() {
		// get hooks for org
		orgws, err := getActiveWebhooksByOrgID(e, repo.OwnerID)
		if err != nil {
			return fmt.Errorf("getActiveWebhooksByOrgID [%d]: %v", repo.OwnerID, err)
		}
		webhooks = append(webhooks, orgws...)
	}
	return prepareHookTasks(e, repo, event, p, webhooks)
}

// PrepareWebhooks adds all active webhooks to task queue.
func PrepareWebhooks(repo *Repository, event HookEventType, p api.Payloader) error {
	return prepareWebhooks(x, repo, event, p)
}

// TestWebhook adds the test webhook matches the ID to task queue.
func TestWebhook(repo *Repository, event HookEventType, p api.Payloader, webhookID int64) error {
	webhook, err := GetWebhookOfRepoByID(repo.ID, webhookID)
	if err != nil {
		return fmt.Errorf("GetWebhookOfRepoByID [repo_id: %d, id: %d]: %v", repo.ID, webhookID, err)
	}
	return prepareHookTasks(x, repo, event, p, []*Webhook{webhook})
}

func (t *HookTask) deliver() {
	t.IsDelivered = true

	timeout := time.Duration(setting.Webhook.DeliverTimeout) * time.Second
	req := httplib.Post(t.URL).SetTimeout(timeout, timeout).
		Header("X-Gogs-Delivery", t.UUID).
		Header("X-Gogs-Signature", t.Signature).
		Header("X-Gogs-Event", string(t.EventType)).
		SetTLSClientConfig(&tls.Config{InsecureSkipVerify: setting.Webhook.SkipTLSVerify})

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
			log.Error(3, "GetWebhookByID: %v", err)
			return
		}
		if t.IsSucceed {
			w.LastStatus = HOOK_STATUS_SUCCEED
		} else {
			w.LastStatus = HOOK_STATUS_FAILED
		}
		if err = UpdateWebhook(w); err != nil {
			log.Error(3, "UpdateWebhook: %v", err)
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

	p, err := ioutil.ReadAll(resp.Body)
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
	x.Where("is_delivered = ?", false).Iterate(new(HookTask),
		func(idx int, bean interface{}) error {
			t := bean.(*HookTask)
			t.deliver()
			tasks = append(tasks, t)
			return nil
		})

	// Update hook task status.
	for _, t := range tasks {
		if err := UpdateHookTask(t); err != nil {
			log.Error(4, "UpdateHookTask [%d]: %v", t.ID, err)
		}
	}

	// Start listening on new hook requests.
	for repoID := range HookQueue.Queue() {
		log.Trace("DeliverHooks [repo_id: %v]", repoID)
		HookQueue.Remove(repoID)

		tasks = make([]*HookTask, 0, 5)
		if err := x.Where("repo_id = ?", repoID).And("is_delivered = ?", false).Find(&tasks); err != nil {
			log.Error(4, "Get repository [%s] hook tasks: %v", repoID, err)
			continue
		}
		for _, t := range tasks {
			t.deliver()
			if err := UpdateHookTask(t); err != nil {
				log.Error(4, "UpdateHookTask [%d]: %v", t.ID, err)
				continue
			}
		}
	}
}

func InitDeliverHooks() {
	go DeliverHooks()
}
