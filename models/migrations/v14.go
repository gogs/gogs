// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
)

func setCommentUpdatedWithCreated(x *xorm.Engine) error {
	if _, err := x.Exec("UPDATE comment SET updated_unix = created_unix"); err != nil {
		return fmt.Errorf("set update_unix: %v", err)
	}
	return nil
}
