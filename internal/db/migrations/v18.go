// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
)

func updateRepositoryDescriptionField(x *xorm.Engine) error {
	exist, err := x.IsTableExist("repository")
	if err != nil {
		return fmt.Errorf("IsTableExist: %v", err)
	} else if !exist {
		return nil
	}
	switch {
	case conf.UseMySQL:
		_, err = x.Exec("ALTER TABLE `repository` MODIFY `description` VARCHAR(512);")
	case conf.UseMSSQL:
		_, err = x.Exec("ALTER TABLE `repository` ALTER COLUMN `description` VARCHAR(512);")
	case conf.UsePostgreSQL:
		_, err = x.Exec("ALTER TABLE `repository` ALTER COLUMN `description` TYPE VARCHAR(512);")
	case conf.UseSQLite3:
		// Sqlite3 uses TEXT type by default for any string type field.
		// Keep this comment to mention that we don't missed any option.
	}
	return err
}
