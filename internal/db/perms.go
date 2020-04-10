// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"strings"

	"github.com/jinzhu/gorm"
	log "unknwon.dev/clog/v2"
)

// PermsStore is the persistent interface for permissions.
//
// NOTE: All methods are sorted in alphabetical order.
type PermsStore interface {
	// AccessMode returns the access mode of given user has to the repository.
	AccessMode(userID int64, repo *Repository) AccessMode
	// Authorize returns true if the user has as good as desired access mode to the repository.
	Authorize(userID int64, repo *Repository, desired AccessMode) bool
	// SetRepoPerms does a full update to which users have which level of access to given repository.
	// Keys of the "accessMap" are user IDs.
	SetRepoPerms(repoID int64, accessMap map[int64]AccessMode) error
}

var Perms PermsStore

var _ PermsStore = (*perms)(nil)

type perms struct {
	*gorm.DB
}

func (db *perms) AccessMode(userID int64, repo *Repository) (mode AccessMode) {
	if repo == nil {
		return AccessModeNone
	}

	// Everyone has read access to public repository.
	if !repo.IsPrivate {
		mode = AccessModeRead
	}

	// Anonymous user gets the default access.
	if userID <= 0 {
		return mode
	}

	if userID == repo.OwnerID {
		return AccessModeOwner
	}

	access := new(Access)
	err := db.Where("user_id = ? AND repo_id = ?", userID, repo.ID).First(access).Error
	if err != nil {
		if !gorm.IsRecordNotFoundError(err){
			log.Error("Failed to get access [user_id: %d, repo_id: %d]: %v", userID, repo.ID, err)
		}
		return mode
	}
	return access.Mode
}

func (db *perms) Authorize(userID int64, repo *Repository, desired AccessMode) bool {
	return desired <= db.AccessMode(userID, repo)
}

func (db *perms) SetRepoPerms(repoID int64, accessMap map[int64]AccessMode) error {
	vals := make([]string, 0, len(accessMap))
	items := make([]interface{}, 0, len(accessMap)*3)
	for userID, mode := range accessMap {
		vals = append(vals, "(?, ?, ?)")
		items = append(items, userID, repoID, mode)
	}

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("repo_id = ?", repoID).Delete(new(Access)).Error
		if err != nil {
			return err
		}

		sql := "INSERT INTO access (user_id, repo_id, mode) VALUES " + strings.Join(vals, ", ")
		return tx.Exec(sql, items...).Error
	})
}
