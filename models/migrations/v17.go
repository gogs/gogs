// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"fmt"

	"github.com/go-xorm/xorm"
)

func removeInvalidProtectBranchWhitelist(x *xorm.Engine) error {
	exist, err := x.IsTableExist("protect_branch_whitelist")
	if err != nil {
		return fmt.Errorf("IsTableExist: %v", err)
	} else if !exist {
		return nil
	}
	_, err = x.Exec("DELETE FROM protect_branch_whitelist WHERE protect_branch_id = 0")
	return err
}
