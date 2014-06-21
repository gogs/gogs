// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gogits/gogs/modules/httplib"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

var (
	ErrWebhookNotExist = errors.New("Webhook does not exist")
)

type HookContentType int

const (
	JSON HookContentType = iota + 1
	FORM
)

// HookEvent represents events that will delivery hook.
type HookEvent struct {
	PushOnly bool `json:"push_only"`
}

// Webhook represents a web hook object.
type Webhook struct {
	Id          int64
	RepoId      int64
	Url         string `xorm:"TEXT"`
	ContentType HookContentType
	Secret      string `xorm:"TEXT"`
	Events      string `xorm:"TEXT"`
	*HookEvent  `xorm:"-"`
	IsSsl       bool
	IsActive    bool
}

// GetEvent handles conversion from Events to HookEvent.
func (w *Webhook) GetEvent() {
	w.HookEvent = &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		log.Error("webhook.GetEvent(%d): %v", w.Id, err)
	}
}

// UpdateEvent handles conversion from HookEvent to Events.
func (w *Webhook) UpdateEvent() error {
	data, err := json.Marshal(w.HookEvent)
	w.Events = string(data)
	return err
}

// HasPushEvent returns true if hook enbaled push event.
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
	err = x.Find(&ws, &Webhook{RepoId: repoId, IsActive: true})
	return ws, err
}

// GetWebhooksByRepoId returns all webhooks of repository.
func GetWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = x.Find(&ws, &Webhook{RepoId: repoId})
	return ws, err
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	_, err := x.AllCols().Update(w)
	return err
}

// DeleteWebhook deletes webhook of repository.
func DeleteWebhook(hookId int64) error {
	_, err := x.Delete(&Webhook{Id: hookId})
	return err
}

//   ___ ___                __   ___________              __
//  /   |   \  ____   ____ |  | _\__    ___/____    _____|  | __
// /    ~    \/  _ \ /  _ \|  |/ / |    |  \__  \  /  ___/  |/ /
// \    Y    (  <_> |  <_> )    <  |    |   / __ \_\___ \|    <
//  \___|_  / \____/ \____/|__|_ \ |____|  (____  /____  >__|_ \
//        \/                    \/              \/     \/     \/

type HookTaskType int

const (
	WEBHOOK HookTaskType = iota + 1
	SERVICE
)

type PayloadAuthor struct {
	Name  string `json:"name"`
	Email string `json:"email"`
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
	Owner       *PayloadAuthor `json:"author"`
	Private     bool           `json:"private"`
}

// Payload represents a payload information of hook.
type Payload struct {
	Secret  string           `json:"secret"`
	Ref     string           `json:"ref"`
	Commits []*PayloadCommit `json:"commits"`
	Repo    *PayloadRepo     `json:"repository"`
	Pusher  *PayloadAuthor   `json:"pusher"`
}

// HookTask represents a hook task.
type HookTask struct {
	Id             int64
	Type           HookTaskType
	Url            string
	*Payload       `xorm:"-"`
	PayloadContent string `xorm:"TEXT"`
	ContentType    HookContentType
	IsSsl          bool
	IsDeliveried   bool
}

// CreateHookTask creates a new hook task,
// it handles conversion from Payload to PayloadContent.
func CreateHookTask(t *HookTask) error {
	data, err := json.Marshal(t.Payload)
	if err != nil {
		return err
	}
	t.PayloadContent = string(data)
	_, err = x.Insert(t)
	return err
}

// UpdateHookTask updates information of hook task.
func UpdateHookTask(t *HookTask) error {
	_, err := x.AllCols().Update(t)
	return err
}

// DeliverHooks checks and delivers undelivered hooks.
func DeliverHooks() {
	timeout := time.Duration(setting.WebhookDeliverTimeout) * time.Second
	x.Where("is_deliveried=?", false).Iterate(new(HookTask),
		func(idx int, bean interface{}) error {
			t := bean.(*HookTask)
			// Only support JSON now.
			if _, err := httplib.Post(t.Url).SetTimeout(timeout, timeout).
				Body([]byte(t.PayloadContent)).Response(); err != nil {
				log.Error("webhook.DeliverHooks(Delivery): %v", err)
				return nil
			}

			t.IsDeliveried = true
			if err := UpdateHookTask(t); err != nil {
				log.Error("webhook.DeliverHooks(UpdateHookTask): %v", err)
				return nil
			}

			log.Trace("Hook delivered: %s", t.PayloadContent)
			return nil
		})
}
