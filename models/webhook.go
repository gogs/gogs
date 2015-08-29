// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"strings"
	"sync"
	"time"

	"github.com/go-xorm/xorm"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/modules/httplib"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/uuid"
)

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
	Create bool `json:"create"`
	Push   bool `json:"push"`
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
	ID           int64 `xorm:"pk autoincr"`
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
	Created      time.Time  `xorm:"CREATED"`
	Updated      time.Time  `xorm:"UPDATED"`
}

func (w *Webhook) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "events":
		w.HookEvent = &HookEvent{}
		if err = json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
			log.Error(3, "Unmarshal[%d]: %v", w.ID, err)
		}
	case "created":
		w.Created = regulateTimeZone(w.Created)
	}
}

func (w *Webhook) GetSlackHook() *SlackMeta {
	s := &SlackMeta{}
	if err := json.Unmarshal([]byte(w.Meta), s); err != nil {
		log.Error(4, "webhook.GetSlackHook(%d): %v", w.ID, err)
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

// HasPushEvent returns true if hook enabled push event.
func (w *Webhook) HasPushEvent() bool {
	return w.PushOnly || w.SendEverything ||
		(w.ChooseEvents && w.HookEvents.Push)
}

func (w *Webhook) EventsArray() []string {
	events := make([]string, 0, 2)
	if w.HasCreateEvent() {
		events = append(events, "create")
	}
	if w.HasPushEvent() {
		events = append(events, "push")
	}
	return events
}

// CreateWebhook creates a new web hook.
func CreateWebhook(w *Webhook) error {
	_, err := x.Insert(w)
	return err
}

// GetWebhookByID returns webhook by given ID.
func GetWebhookByID(id int64) (*Webhook, error) {
	w := new(Webhook)
	has, err := x.Id(id).Get(w)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrWebhookNotExist{id}
	}
	return w, nil
}

// GetActiveWebhooksByRepoID returns all active webhooks of repository.
func GetActiveWebhooksByRepoID(repoID int64) (ws []*Webhook, err error) {
	err = x.Where("repo_id=?", repoID).And("is_active=?", true).Find(&ws)
	return ws, err
}

// GetWebhooksByRepoId returns all webhooks of repository.
func GetWebhooksByRepoId(repoID int64) (ws []*Webhook, err error) {
	err = x.Find(&ws, &Webhook{RepoID: repoID})
	return ws, err
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	_, err := x.Id(w.ID).AllCols().Update(w)
	return err
}

// DeleteWebhook deletes webhook of repository.
func DeleteWebhook(id int64) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Delete(&Webhook{ID: id}); err != nil {
		return err
	} else if _, err = sess.Delete(&HookTask{HookID: id}); err != nil {
		return err
	}

	return sess.Commit()
}

// GetWebhooksByOrgId returns all webhooks for an organization.
func GetWebhooksByOrgId(orgID int64) (ws []*Webhook, err error) {
	err = x.Find(&ws, &Webhook{OrgID: orgID})
	return ws, err
}

// GetActiveWebhooksByOrgID returns all active webhooks for an organization.
func GetActiveWebhooksByOrgID(orgID int64) (ws []*Webhook, err error) {
	err = x.Where("org_id=?", orgID).And("is_active=?", true).Find(&ws)
	return ws, err
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
)

var hookTaskTypes = map[string]HookTaskType{
	"gogs":  GOGS,
	"slack": SLACK,
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
	HOOK_EVENT_CREATE HookEventType = "create"
	HOOK_EVENT_PUSH   HookEventType = "push"
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
	ID              int64 `xorm:"pk autoincr"`
	RepoID          int64 `xorm:"INDEX"`
	HookID          int64
	UUID            string
	Type            HookTaskType
	URL             string
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
			log.Error(3, "Unmarshal[%d]: %v", t.ID, err)
		}
	}
}

func (t *HookTask) MarshalJSON(v interface{}) string {
	p, err := json.Marshal(v)
	if err != nil {
		log.Error(3, "Marshal[%d]: %v", t.ID, err)
	}
	return string(p)
}

// HookTasks returns a list of hook tasks by given conditions.
func HookTasks(hookID int64, page int) ([]*HookTask, error) {
	tasks := make([]*HookTask, 0, setting.Webhook.PagingNum)
	return tasks, x.Limit(setting.Webhook.PagingNum, (page-1)*setting.Webhook.PagingNum).Where("hook_id=?", hookID).Desc("id").Find(&tasks)
}

// CreateHookTask creates a new hook task,
// it handles conversion from Payload to PayloadContent.
func CreateHookTask(t *HookTask) error {
	data, err := t.Payloader.JSONPayload()
	if err != nil {
		return err
	}
	t.UUID = uuid.NewV4().String()
	t.PayloadContent = string(data)
	_, err = x.Insert(t)
	return err
}

// UpdateHookTask updates information of hook task.
func UpdateHookTask(t *HookTask) error {
	_, err := x.Id(t.ID).AllCols().Update(t)
	return err
}

// PrepareWebhooks adds new webhooks to task queue for given payload.
func PrepareWebhooks(repo *Repository, event HookEventType, p api.Payloader) error {
	if err := repo.GetOwner(); err != nil {
		return fmt.Errorf("GetOwner: %v", err)
	}

	ws, err := GetActiveWebhooksByRepoID(repo.ID)
	if err != nil {
		return fmt.Errorf("GetActiveWebhooksByRepoID: %v", err)
	}

	// check if repo belongs to org and append additional webhooks
	if repo.Owner.IsOrganization() {
		// get hooks for org
		orgws, err := GetActiveWebhooksByOrgID(repo.OwnerID)
		if err != nil {
			return fmt.Errorf("GetActiveWebhooksByOrgID: %v", err)
		}
		ws = append(ws, orgws...)
	}

	if len(ws) == 0 {
		return nil
	}

	for _, w := range ws {
		switch event {
		case HOOK_EVENT_CREATE:
			if !w.HasCreateEvent() {
				continue
			}
		case HOOK_EVENT_PUSH:
			if !w.HasPushEvent() {
				continue
			}
		}

		switch w.HookTaskType {
		case SLACK:
			p, err = GetSlackPayload(p, event, w.Meta)
			if err != nil {
				return fmt.Errorf("GetSlackPayload: %v", err)
			}
		default:
			p.SetSecret(w.Secret)
		}

		if err = CreateHookTask(&HookTask{
			RepoID:      repo.ID,
			HookID:      w.ID,
			Type:        w.HookTaskType,
			URL:         w.URL,
			Payloader:   p,
			ContentType: w.ContentType,
			EventType:   HOOK_EVENT_PUSH,
			IsSSL:       w.IsSSL,
		}); err != nil {
			return fmt.Errorf("CreateHookTask: %v", err)
		}
	}
	return nil
}

type hookQueue struct {
	// Make sure one repository only occur once in the queue.
	lock    sync.Mutex
	repoIDs map[int64]bool

	queue chan int64
}

func (q *hookQueue) removeRepoID(id int64) {
	q.lock.Lock()
	defer q.lock.Unlock()
	delete(q.repoIDs, id)
}

func (q *hookQueue) addRepoID(id int64) {
	q.lock.Lock()
	if q.repoIDs[id] {
		q.lock.Unlock()
		return
	}
	q.repoIDs[id] = true
	q.lock.Unlock()
	q.queue <- id
}

// AddRepoID adds repository ID to hook delivery queue.
func (q *hookQueue) AddRepoID(id int64) {
	go q.addRepoID(id)
}

var HookQueue *hookQueue

func deliverHook(t *HookTask) {
	t.IsDelivered = true

	timeout := time.Duration(setting.Webhook.DeliverTimeout) * time.Second
	req := httplib.Post(t.URL).SetTimeout(timeout, timeout).
		Header("X-Gogs-Delivery", t.UUID).
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
		t.Delivered = time.Now().UTC().UnixNano()
		if t.IsSucceed {
			log.Trace("Hook delivered: %s", t.UUID)
		}

		// Update webhook last delivery status.
		w, err := GetWebhookByID(t.HookID)
		if err != nil {
			log.Error(5, "GetWebhookByID: %v", err)
			return
		}
		if t.IsSucceed {
			w.LastStatus = HOOK_STATUS_SUCCEED
		} else {
			w.LastStatus = HOOK_STATUS_FAILED
		}
		if err = UpdateWebhook(w); err != nil {
			log.Error(5, "UpdateWebhook: %v", err)
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

	switch t.Type {
	case SLACK:
		if t.ResponseInfo.Body != "ok" {
			log.Error(5, "slack failed with: %s", t.ResponseInfo.Body)
			t.IsSucceed = false
		}
	}
}

// DeliverHooks checks and delivers undelivered hooks.
func DeliverHooks() {
	tasks := make([]*HookTask, 0, 10)
	x.Where("is_delivered=?", false).Iterate(new(HookTask),
		func(idx int, bean interface{}) error {
			t := bean.(*HookTask)
			deliverHook(t)
			tasks = append(tasks, t)
			return nil
		})

	// Update hook task status.
	for _, t := range tasks {
		if err := UpdateHookTask(t); err != nil {
			log.Error(4, "UpdateHookTask(%d): %v", t.ID, err)
		}
	}

	HookQueue = &hookQueue{
		lock:    sync.Mutex{},
		repoIDs: make(map[int64]bool),
		queue:   make(chan int64, setting.Webhook.QueueLength),
	}

	// Start listening on new hook requests.
	for repoID := range HookQueue.queue {
		HookQueue.removeRepoID(repoID)

		tasks = make([]*HookTask, 0, 5)
		if err := x.Where("repo_id=? AND is_delivered=?", repoID, false).Find(&tasks); err != nil {
			log.Error(4, "Get repository(%d) hook tasks: %v", repoID, err)
			continue
		}
		for _, t := range tasks {
			deliverHook(t)
			if err := UpdateHookTask(t); err != nil {
				log.Error(4, "UpdateHookTask(%d): %v", t.ID, err)
			}
		}
	}
}

func InitDeliverHooks() {
	go DeliverHooks()
}
