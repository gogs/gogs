// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package hooks

import (
	"encoding/json"
	"time"

	"github.com/gogits/gogs/modules/httplib"
	"github.com/gogits/gogs/modules/log"
)

// Hook task types.
const (
	HTT_WEBHOOK = iota + 1
	HTT_SERVICE
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

// Payload represents payload information of hook.
type Payload struct {
	Secret  string           `json:"secret"`
	Ref     string           `json:"ref"`
	Commits []*PayloadCommit `json:"commits"`
	Repo    *PayloadRepo     `json:"repository"`
	Pusher  *PayloadAuthor   `json:"pusher"`
}

// HookTask represents hook task.
type HookTask struct {
	Type int
	Url  string
	*Payload
	ContentType int
	IsSsl       bool
}

var (
	taskQueue = make(chan *HookTask, 1000)
)

// AddHookTask adds new hook task to task queue.
func AddHookTask(t *HookTask) {
	taskQueue <- t
}

func init() {
	go handleQueue()
}

func handleQueue() {
	for {
		select {
		case t := <-taskQueue:
			// Only support JSON now.
			data, err := json.MarshalIndent(t.Payload, "", "\t")
			if err != nil {
				log.Error("hooks.handleQueue(json): %v", err)
				continue
			}

			_, err = httplib.Post(t.Url).SetTimeout(5*time.Second, 5*time.Second).
				Body(data).Response()
			if err != nil {
				log.Error("hooks.handleQueue: Fail to deliver hook: %v", err)
				continue
			}
			log.Info("Hook delivered: %s", string(data))
		}
	}
}
