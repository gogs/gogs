// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "fmt"

type WebhookNotExist struct {
	ID int64
}

func IsWebhookNotExist(err error) bool {
	_, ok := err.(WebhookNotExist)
	return ok
}

func (err WebhookNotExist) Error() string {
	return fmt.Sprintf("webhook does not exist [id: %d]", err.ID)
}

type HookTaskNotExist struct {
	HookID int64
	UUID   string
}

func IsHookTaskNotExist(err error) bool {
	_, ok := err.(HookTaskNotExist)
	return ok
}

func (err HookTaskNotExist) Error() string {
	return fmt.Sprintf("hook task does not exist [hook_id: %d, uuid: %s]", err.HookID, err.UUID)
}
