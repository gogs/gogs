// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/jinzhu/gorm"
	log "unknwon.dev/clog/v2"
)

// TwoFactorsStore is the persistent interface for 2FA.
//
// NOTE: All methods are sorted in alphabetical order.
type TwoFactorsStore interface {
	// IsUserEnabled returns true if the user has enabled 2FA.
	IsUserEnabled(userID int64) bool
}

var TwoFactors TwoFactorsStore

type twoFactors struct {
	*gorm.DB
}

func (db *twoFactors) IsUserEnabled(userID int64) bool {
	var count int64
	err := db.Model(new(TwoFactor)).Where("user_id = ?", userID).Count(&count).Error
	if err != nil {
		log.Error("Failed to count two factors [user_id: %d]: %v", userID, err)
	}
	return count > 0
}
