// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
	"github.com/gogits/gogs/pkg/setting"
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
	if setting.UseMySQL {
		_, err = x.Exec("ALTER TABLE `repository` ADD `use_custom_avatar` TINYINT(1) DEFAULT 0")
	}
	else if setting.UsePostgreSQL {
		_, err = x.Exec("ALTER TABLE `repository` ADD `use_custom_avatar` BOOLEAN DEFAULT FALSE")
	}
	else if setting.UseMSSQL {
		_, err = x.Exec("ALTER TABLE `repository` ADD `use_custom_avatar` TINYINT DEFAULT 0")
	}
	else if setting.UseSQLite3 {
		_, err = x.Exec("ALTER TABLE `repository` ADD `use_custom_avatar` INT DEFAULT 0")
	}
	if err != nil {
		return fmt.Errorf("Add use_custom_avatar field: %v", err)
	}
	return err
}
