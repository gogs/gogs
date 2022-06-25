// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"gorm.io/gorm"
)

func addIndexToActionUserID(db *gorm.DB) error {
	type action struct {
		UserID string `gorm:"index"`
	}
	if db.Migrator().HasIndex(&action{}, "UserID") {
		return nil
	}
	return db.Migrator().CreateIndex(&action{}, "UserID")
}
