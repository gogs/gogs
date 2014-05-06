// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"

	"github.com/gogits/gogs/modules/log"
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
	Payload     string `xorm:"TEXT"`
	ContentType int
	Secret      string `xorm:"TEXT"`
	Events      string `xorm:"TEXT"`
	IsSsl       bool
	IsActive    bool
}

func (w *Webhook) GetEvent() *HookEvent {
	h := &HookEvent{}
	if err := json.Unmarshal([]byte(w.Events), h); err != nil {
		log.Error("webhook.GetEvent(%d): %v", w.Id, err)
	}
	return h
}

func (w *Webhook) SaveEvent(h *HookEvent) error {
	data, err := json.Marshal(h)
	w.Events = string(data)
	return err
}

// CreateWebhook creates new webhook.
func CreateWebhook(w *Webhook) error {
	_, err := orm.Insert(w)
	return err
}

// GetWebhooksByRepoId returns all webhooks of repository.
func GetWebhooksByRepoId(repoId int64) (ws []*Webhook, err error) {
	err = orm.Find(&ws, &Webhook{RepoId: repoId})
	return ws, err
}
