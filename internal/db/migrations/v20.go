// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package migrations

import (
	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/cryptoutil"
)

func migrateAccessTokenSHA1(db *gorm.DB) error {
	type accessToken struct {
		ID     int64
		Sha1   string
		SHA256 string `gorm:"TYPE:VARCHAR(64)"`
	}
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Migrator().AddColumn(&accessToken{}, "SHA256")
		if err != nil {
			return errors.Wrap(err, "add column")
		}

		var accessTokens []*accessToken
		err = tx.Debug().Where("sha256 IS NULL").Find(&accessTokens).Error
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

		type accessTokenWithConstraint struct {
			SHA256 string `gorm:"type:VARCHAR(64);uniqueIndex:access_token_unique;not null"`
		}
		err = tx.Table("access_token").AutoMigrate(&accessTokenWithConstraint{})
		if err != nil {
			return errors.Wrap(err, "auto migrate")
		}

		return nil
	})
}
