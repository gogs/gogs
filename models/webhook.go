// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"errors"

	"github.com/gogits/gogs/modules/log"
)

var (
	ErrWebhookNotExist = errors.New("Webhook does not exist")
)

// Content types.
const (
	CT_JSON = iota + 1
	CT_FORM
)

type HookEvent struct {
	PushOnly bool `json:"push_only"`
}

type Webhook struct {
	Id          int64
	RepoId      int64
	Url         string `xorm:"TEXT"`
	ContentType int
	Secret      string `xorm:"TEXT"`
	Events      string `xorm:"TEXT"`
	*HookEvent  `xorm:"-"`
	IsSsl       bool
	IsActive    bool
}

func (w *Webhook) GetEvent() {
	w.HookEvent = &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), w.HookEvent); err != nil {
		log.Error("webhook.GetEvent(%d): %v", w.Id, err)
	}
}

func (w *Webhook) SaveEvent() error {
	data, err := json.Marshal(w.HookEvent)
	w.Events = string(data)
	return err
}

func (w *Webhook) HasPushEvent() bool {
	if w.PushOnly {
		return true
	}
	return false
}

// CreateWebhook creates new webhook.
func CreateWebhook(w *Webhook) error {
	_, err := orm.Insert(w)
	return err
}

// UpdateWebhook updates information of webhook.
func UpdateWebhook(w *Webhook) error {
	_, err := orm.AllCols().Update(w)
	return err
}

// GetWebhookById returns webhook by given ID.
func GetWebhookById(hookId int64) (*Webhook, error) {
	w := &Webhook{Id: hookId}
	has, err := orm.Get(w)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrWebhookNotExist
	}
	return w, nil
}

// GetActiveWebhooksByRepoId returns all active webhooks of repository.
func GetActiveWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = orm.Find(&ws, &Webhook{RepoId: repoId, IsActive: true})
	return ws, err
}

// GetWebhooksByRepoId returns all webhooks of repository.
func GetWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = orm.Find(&ws, &Webhook{RepoId: repoId})
	return ws, err
}

// DeleteWebhook deletes webhook of repository.
func DeleteWebhook(hookId int64) error {
	_, err := orm.Delete(&Webhook{Id: hookId})
	return err
}
