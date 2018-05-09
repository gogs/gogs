// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
)

func updateRepositoryDescriptionField(x *xorm.Engine) error {
	exist, err := x.IsTableExist("repository")
	if err != nil {
		return fmt.Errorf("IsTableExist: %v", err)
	} else if !exist {
		return nil
	}
	// First try is for Postgress / Oracle
	_, err = x.Exec("ALTER TABLE `repository` ALTER COLUMN `description` TYPE TEXT")
	if err != nil {
		// Second try is for MySQL
		_, err = x.Exec("ALTER TABLE `repository` MODIFY `description` TEXT")
	}
	// Sqlite will fail here
	return err
}
