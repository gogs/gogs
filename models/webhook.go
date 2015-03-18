// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"crypto/tls"
	"encoding/json"
	"errors"
	"io/ioutil"
	"time"

	"github.com/gogits/gogs/modules/httplib"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	"github.com/gogits/gogs/modules/uuid"
)

var (
	ErrWebhookNotExist = errors.New("Webhook does not exist")
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

// HookEvent represents events that will delivery hook.
type HookEvent struct {
	PushOnly bool `json:"push_only"`
}

// Webhook represents a web hook object.
type Webhook struct {
	Id           int64
	RepoId       int64
	Url          string `xorm:"TEXT"`
	ContentType  HookContentType
	Secret       string `xorm:"TEXT"`
	Events       string `xorm:"TEXT"`
	*HookEvent   `xorm:"-"`
	IsSsl        bool
	IsActive     bool
	HookTaskType HookTaskType
	Meta         string `xorm:"TEXT"` // store hook-specific attributes
	OrgId        int64
	Created      time.Time `xorm:"CREATED"`
	Updated      time.Time `xorm:"UPDATED"`
}

// GetEvent handles conversion from Events to HookEvent.
func (w *Webhook) GetEvent() {
	w.HookEvent = &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		log.Error(4, "webhook.GetEvent(%d): %v", w.Id, err)
	}
}

func (w *Webhook) GetSlackHook() *Slack {
	s := &Slack{}
	if err := json.Unmarshal([]byte(w.Meta), s); err != nil {
		log.Error(4, "webhook.GetSlackHook(%d): %v", w.Id, err)
	}
	return s
}

// UpdateEvent handles conversion from HookEvent to Events.
func (w *Webhook) UpdateEvent() error {
	data, err := json.Marshal(w.HookEvent)
	w.Events = string(data)
	return err
}

// HasPushEvent returns true if hook enabled push event.
func (w *Webhook) HasPushEvent() bool {
	if w.PushOnly {
		return true
	}
	return false
}

// CreateWebhook creates a new web hook.
func CreateWebhook(w *Webhook) error {
	_, err := x.Insert(w)
	return err
}

// GetWebhookById returns webhook by given ID.
func GetWebhookById(hookId int64) (*Webhook, error) {
	w := &Webhook{Id: hookId}
	has, err := x.Get(w)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrWebhookNotExist
	}
	return w, nil
}

// GetActiveWebhooksByRepoId returns all active webhooks of repository.
func GetActiveWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = x.Where("repo_id=?", repoId).And("is_active=?", true).Find(&ws)
	return ws, err
}

// GetWebhooksByRepoId returns all webhooks of repository.
func GetWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = x.Find(&ws, &Webhook{RepoId: repoId})
	return ws, err
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	_, err := x.Id(w.Id).AllCols().Update(w)
	return err
}

// DeleteWebhook deletes webhook of repository.
func DeleteWebhook(hookId int64) error {
	_, err := x.Delete(&Webhook{Id: hookId})
	return err
}

// GetWebhooksByOrgId returns all webhooks for an organization.
func GetWebhooksByOrgId(orgId int64) (ws []*Webhook, err error) {
	err = x.Find(&ws, &Webhook{OrgId: orgId})
	return ws, err
}

// GetActiveWebhooksByOrgId returns all active webhooks for an organization.
func GetActiveWebhooksByOrgId(orgId int64) (ws []*Webhook, err error) {
	err = x.Where("org_id=?", orgId).And("is_active=?", true).Find(&ws)
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
	HOOK_EVENT_PUSH HookEventType = "push"
)

// FIXME: just use go-gogs-client structs maybe?
type PayloadAuthor struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	UserName string `json:"username"`
}

type PayloadCommit struct {
	Id      string         `json:"id"`
	Message string         `json:"message"`
	Url     string         `json:"url"`
	Author  *PayloadAuthor `json:"author"`
}

type PayloadRepo struct {
	Id          int64          `json:"id"`
	Name        string         `json:"name"`
	Url         string         `json:"url"`
	Description string         `json:"description"`
	Website     string         `json:"website"`
	Watchers    int            `json:"watchers"`
	Owner       *PayloadAuthor `json:"owner"`
	Private     bool           `json:"private"`
}

type BasePayload interface {
	GetJSONPayload() ([]byte, error)
}

// Payload represents a payload information of hook.
type Payload struct {
	Secret     string           `json:"secret"`
	Ref        string           `json:"ref"`
	Commits    []*PayloadCommit `json:"commits"`
	Repo       *PayloadRepo     `json:"repository"`
	Pusher     *PayloadAuthor   `json:"pusher"`
	Before     string           `json:"before"`
	After      string           `json:"after"`
	CompareUrl string           `json:"compare_url"`
}

func (p Payload) GetJSONPayload() ([]byte, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return []byte{}, err
	}
	return data, nil
}

// HookTask represents a hook task.
type HookTask struct {
	Id             int64
	Uuid           string
	Type           HookTaskType
	Url            string
	BasePayload    `xorm:"-"`
	PayloadContent string `xorm:"TEXT"`
	ContentType    HookContentType
	EventType      HookEventType
	IsSsl          bool
	IsDelivered    bool
	IsSucceed      bool
}

// CreateHookTask creates a new hook task,
// it handles conversion from Payload to PayloadContent.
func CreateHookTask(t *HookTask) error {
	data, err := t.BasePayload.GetJSONPayload()
	if err != nil {
		return err
	}
	t.Uuid = uuid.NewV4().String()
	t.PayloadContent = string(data)
	_, err = x.Insert(t)
	return err
}

// UpdateHookTask updates information of hook task.
func UpdateHookTask(t *HookTask) error {
	_, err := x.Id(t.Id).AllCols().Update(t)
	return err
}

var (
	// Prevent duplicate deliveries.
	// This happens with massive hook tasks cannot finish delivering
	// before next shooting starts.
	isShooting = false
)

// DeliverHooks checks and delivers undelivered hooks.
// FIXME: maybe can use goroutine to shoot a number of them at same time?
func DeliverHooks() {
	if isShooting {
		return
	}
	isShooting = true
	defer func() { isShooting = false }()

	tasks := make([]*HookTask, 0, 10)
	timeout := time.Duration(setting.Webhook.DeliverTimeout) * time.Second
	x.Where("is_delivered=?", false).Iterate(new(HookTask),
		func(idx int, bean interface{}) error {
			t := bean.(*HookTask)
			req := httplib.Post(t.Url).SetTimeout(timeout, timeout).
				Header("X-Gogs-Delivery", t.Uuid).
				Header("X-Gogs-Event", string(t.EventType)).
				SetTLSClientConfig(&tls.Config{InsecureSkipVerify: setting.Webhook.SkipTLSVerify})

			switch t.ContentType {
			case JSON:
				req = req.Header("Content-Type", "application/json").Body(t.PayloadContent)
			case FORM:
				req.Param("payload", t.PayloadContent)
			}

			t.IsDelivered = true

			// FIXME: record response.
			switch t.Type {
			case GOGS:
				{
					if _, err := req.Response(); err != nil {
						log.Error(5, "Delivery: %v", err)
					} else {
						t.IsSucceed = true
					}
				}
			case SLACK:
				{
					if res, err := req.Response(); err != nil {
						log.Error(5, "Delivery: %v", err)
					} else {
						defer res.Body.Close()
						contents, err := ioutil.ReadAll(res.Body)
						if err != nil {
							log.Error(5, "%s", err)
						} else {
							if string(contents) != "ok" {
								log.Error(5, "slack failed with: %s", string(contents))
							} else {
								t.IsSucceed = true
							}
						}
					}
				}
			}

			tasks = append(tasks, t)

			if t.IsSucceed {
				log.Trace("Hook delivered(%s): %s", t.Uuid, t.PayloadContent)
			}
			return nil
		})

	// Update hook task status.
	for _, t := range tasks {
		if err := UpdateHookTask(t); err != nil {
			log.Error(4, "UpdateHookTask(%d): %v", t.Id, err)
		}
	}
}
