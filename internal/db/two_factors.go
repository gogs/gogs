// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"encoding/base64"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/cryptoutil"
)

// TwoFactorsStore is the persistent interface for 2FA.
//
// NOTE: All methods are sorted in alphabetical order.
type TwoFactorsStore interface {
	// Create creates a new 2FA token and recovery codes for given user.
	// The "key" is used to encrypt and later decrypt given "secret",
	// which should be configured in site-level and change of the "key"
	// will break all existing 2FA tokens.
	Create(userID int64, key, secret string) error
	// IsUserEnabled returns true if the user has enabled 2FA.
	IsUserEnabled(userID int64) bool
}

var TwoFactors TwoFactorsStore

// NOTE: This is a GORM create hook.
func (t *TwoFactor) BeforeCreate() {
	t.CreatedUnix = time.Now().Unix()
}

var _ TwoFactorsStore = (*twoFactors)(nil)

type twoFactors struct {
	*gorm.DB
}

func (db *twoFactors) Create(userID int64, key, secret string) error {
	encrypted, err := cryptoutil.AESGCMEncrypt(cryptoutil.MD5Bytes(key), []byte(secret))
	if err != nil {
		return errors.Wrap(err, "encrypt secret")
	}
	tf := &TwoFactor{
		UserID: userID,
		Secret: base64.StdEncoding.EncodeToString(encrypted),
	}

	recoveryCodes, err := generateRecoveryCodes(userID)
	if err != nil {
		return errors.Wrap(err, "generate recovery codes")
	}

	vals := make([]string, 0, len(recoveryCodes))
	items := make([]interface{}, 0, len(recoveryCodes)*2)
	for _, code := range recoveryCodes {
		vals = append(vals, "(?, ?)")
		items = append(items, code.UserID, code.Code)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Create(tf).Error
		if err != nil {
			return err
		}

		sql := "INSERT INTO two_factor_recovery_code (user_id, code) VALUES " + strings.Join(vals, ", ")
		return tx.Exec(sql, items...).Error
	})
}

func (db *twoFactors) IsUserEnabled(userID int64) bool {
	var count int64
	err := db.Model(new(TwoFactor)).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		log.Error("Failed to count two factors [user_id: %d]: %v", userID, err)
	}
	return count > 0
}
