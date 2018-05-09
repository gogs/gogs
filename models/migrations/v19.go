// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
)

func updateRepositoryAddAvatarFields(x *xorm.Engine) error {
	exist, err := x.IsTableExist("repository")
	if err != nil {
		return fmt.Errorf("IsTableExist: %v", err)
	} else if !exist {
		return nil
	}
	_, err = x.Exec("ALTER TABLE `repository` ADD `avatar` VARCHAR(2048) NOT NULL")
	if err != nil {
		return fmt.Errorf("Add avatar field: %v", err)
	}
	_, err = x.Exec("ALTER TABLE `repository` ADD `avatar_email` VARCHAR(256) NOT NULL")
	if err != nil {
		return fmt.Errorf("Add avatar_email field: %v", err)
	}
	_, err = x.Exec("ALTER TABLE `repository` ADD `use_custom_avatar` INT(1) DEFAULT NULL")
	if err != nil {
		return fmt.Errorf("Add use_custom_avatar field: %v", err)
	}
	return err
}
