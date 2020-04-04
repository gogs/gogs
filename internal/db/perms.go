// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"github.com/jinzhu/gorm"
	log "unknwon.dev/clog/v2"
)

// PermsStore is the persistent interface for permissions.
//
// NOTE: All methods are sorted in alphabetical order.
type PermsStore interface {
	// AccessMode returns the access mode of given user has to the repository.
	AccessMode(userID int64, repo *Repository) AccessMode
	// Authorize returns true if the user has as good as desired access mode to
	// the repository.
	Authorize(userID int64, repo *Repository, desired AccessMode) bool
}

var Perms PermsStore

type perms struct {
	*gorm.DB
}

func (db *perms) AccessMode(userID int64, repo *Repository) AccessMode {
	var mode AccessMode
	// Everyone has read access to public repository.
	if !repo.IsPrivate {
		mode = AccessModeRead
	}

	// Quick check to avoid a DB query.
	if userID <= 0 {
		return mode
	}

	if userID == repo.OwnerID {
		return AccessModeOwner
	}

	access := new(Access)
	err := db.Where("user_id = ? AND repo_id = ?", userID, repo.ID).First(access).Error
	if err != nil {
		log.Error("Failed to get access [user_id: %d, repo_id: %d]: %v", userID, repo.ID, err)
		return mode
	}
	return access.Mode
}

func (db *perms) Authorize(userID int64, repo *Repository, desired AccessMode) bool {
	return desired <= db.AccessMode(userID, repo)
}
