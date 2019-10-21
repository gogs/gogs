// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"xorm.io/xorm"
)

func cleanUnlinkedWebhookAndHookTasks(x *xorm.Engine) error {
	_, err := x.Exec(`DELETE FROM webhook WHERE repo_id NOT IN (SELECT id FROM repository);`)
	if err != nil {
		return err
	}
	_, err = x.Exec(`DELETE FROM hook_task WHERE repo_id NOT IN (SELECT id FROM repository);`)
	return err
}
