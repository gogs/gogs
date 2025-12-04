// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/cryptoutil"
)

func migrateAccessTokenToSHA256(db *gorm.DB) error {
	type accessToken struct {
		ID     int64
		Sha1   string
		SHA256 string `gorm:"TYPE:VARCHAR(64)"`
	}

	if db.Migrator().HasColumn(&accessToken{}, "SHA256") {
		return errMigrationSkipped
	}
	return db.Transaction(func(tx *gorm.DB) error {
		// 1. Add column without constraints because all rows have NULL values for the
		// "sha256" column.
		err := tx.Migrator().AddColumn(&accessToken{}, "SHA256")
		if err != nil {
			return errors.Wrap(err, "add column")
		}

		// 2. Generate SHA256 for existing rows from their values in the "sha1" column.
		var accessTokens []*accessToken
		err = tx.Where("sha256 IS NULL").Find(&accessTokens).Error
		if err != nil {
			return errors.Wrap(err, "list")
		}

		for _, t := range accessTokens {
			sha256 := cryptoutil.SHA256(t.Sha1)
			err = tx.Model(&accessToken{}).Where("id = ?", t.ID).Update("sha256", sha256).Error
			if err != nil {
				return errors.Wrap(err, "update")
			}
		}

		// 3. We are now safe to apply constraints to the "sha256" column.
		type accessTokenWithConstraint struct {
			SHA256 string `gorm:"type:VARCHAR(64);unique;not null"`
		}
		err = tx.Table("access_token").AutoMigrate(&accessTokenWithConstraint{})
		if err != nil {
			return errors.Wrap(err, "auto migrate")
		}

		return nil
	})
}
