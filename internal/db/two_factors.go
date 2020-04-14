// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"encoding/base64"
	"fmt"

	"github.com/jinzhu/gorm"
	"github.com/unknwon/com"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/tool"
)

// TwoFactorsStore is the persistent interface for 2FA.
//
// NOTE: All methods are sorted in alphabetical order.
type TwoFactorsStore interface {
	// Create creates a new 2FA token and recovery codes for given user.
	Create(userID int64, secret string) error
	// IsUserEnabled returns true if the user has enabled 2FA.
	IsUserEnabled(userID int64) bool
}

var TwoFactors TwoFactorsStore

var _ TwoFactorsStore = (*twoFactors)(nil)

type twoFactors struct {
	*gorm.DB
}

func (db *twoFactors) Create(userID int64, secret string) error {
	t := &TwoFactor{
		UserID: userID,
	}

	// Encrypt secret
	encryptSecret, err := com.AESGCMEncrypt(tool.MD5Bytes(conf.Security.SecretKey), []byte(secret))
	if err != nil {
		return fmt.Errorf("AESGCMEncrypt: %v", err)
	}
	t.Secret = base64.StdEncoding.EncodeToString(encryptSecret)

	recoveryCodes, err := generateRecoveryCodes(userID)
	if err != nil {
		return fmt.Errorf("generateRecoveryCodes: %v", err)
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(t); err != nil {
		return fmt.Errorf("insert two-factor: %v", err)
	} else if _, err = sess.Insert(recoveryCodes); err != nil {
		return fmt.Errorf("insert recovery codes: %v", err)
	}

	return sess.Commit()
}

func (db *twoFactors) IsUserEnabled(userID int64) bool {
	var count int64
	err := db.Model(new(TwoFactor)).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		log.Error("Failed to count two factors [user_id: %d]: %v", userID, err)
	}
	return count > 0
}
