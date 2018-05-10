// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
	"github.com/gogits/gogs/pkg/setting"
)

func updateRepositoryDescriptionField(x *xorm.Engine) error {
	exist, err := x.IsTableExist("repository")
	if err != nil {
		return fmt.Errorf("IsTableExist: %v", err)
	} else if !exist {
		return nil
	}
	if (setting.UseMySQL) {
		_, err = x.Exec("ALTER TABLE `repository` MODIFY `description` TEXT")
	}
	if (setting.UseMSSQL) {
		_, err = x.Exec("ALTER TABLE `repository` ALTER COLUMN `description` TEXT")
	}
	if (setting.UsePostgreSQL) {
		_, err = x.Exec("ALTER TABLE `repository` ALTER COLUMN `description` TYPE TEXT")
	}
	if (setting.UseSQLite3) {
		// Wait, no! Sqlite3 uses TEXT field by default for any string type field.
	}
	return err
}
