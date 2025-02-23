// Copyright 2025 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"gorm.io/gorm"
)

func renameRepoPullsAllowRebaseToPullsAllowAlt(db *gorm.DB) error {
	type Repository struct {
		PullsAllowRebase bool `gorm:"not null;default:FALSE"`
		PullsAllowAlt bool `gorm:"not null;default:FALSE"`
	}
	if db.Migrator().HasColumn(&Repository{}, "PullsAllowAlt") {
		return errMigrationSkipped
	}
	return db.Migrator().RenameColumn(&Repository{}, "PullsAllowRebase", "PullsAllowAlt")
}
