// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
)

// Access represents the highest access level of a user has to a repository. The
// only access type that is not in this table is the real owner of a repository.
// In case of an organization repository, the members of the owners team are in
// this table.
type Access struct {
	ID     int64      `gorm:"primaryKey"`
	UserID int64      `xorm:"UNIQUE(s)" gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64      `xorm:"UNIQUE(s)" gorm:"uniqueIndex:access_user_repo_unique;not null"`
	Mode   AccessMode `gorm:"not null"`
}

// AccessMode is the access mode of a user has to a repository.
type AccessMode int

const (
	AccessModeNone  AccessMode = iota // 0
	AccessModeRead                    // 1
	AccessModeWrite                   // 2
	AccessModeAdmin                   // 3
	AccessModeOwner                   // 4
)

func (mode AccessMode) String() string {
	switch mode {
	case AccessModeRead:
		return "read"
	case AccessModeWrite:
		return "write"
	case AccessModeAdmin:
		return "admin"
	case AccessModeOwner:
		return "owner"
	default:
		return "none"
	}
}

// ParseAccessMode returns corresponding access mode to given permission string.
func ParseAccessMode(permission string) AccessMode {
	switch permission {
	case "write":
		return AccessModeWrite
	case "admin":
		return AccessModeAdmin
	default:
		return AccessModeRead
	}
}

// PermissionsStore is the storage layer for repository permissions.
type PermissionsStore struct {
	db *gorm.DB
}

func newPermissionsStore(db *gorm.DB) *PermissionsStore {
	return &PermissionsStore{db: db}
}

type AccessModeOptions struct {
	OwnerID int64 // The ID of the repository owner.
	Private bool  // Whether the repository is private.
}

// AccessMode returns the access mode of given user has to the repository.
func (s *PermissionsStore) AccessMode(ctx context.Context, userID, repoID int64, opts AccessModeOptions) (mode AccessMode) {
	if repoID <= 0 {
		return AccessModeNone
	}

	// Everyone has read access to public repository.
	if !opts.Private {
		mode = AccessModeRead
	}

	// Anonymous user gets the default access.
	if userID <= 0 {
		return mode
	}

	if userID == opts.OwnerID {
		return AccessModeOwner
	}

	access := new(Access)
	err := s.db.WithContext(ctx).Where("user_id = ? AND repo_id = ?", userID, repoID).First(access).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Error("Failed to get access [user_id: %d, repo_id: %d]: %v", userID, repoID, err)
		}
		return mode
	}
	return access.Mode
}

// Authorize returns true if the user has as good as desired access mode to the
// repository.
func (s *PermissionsStore) Authorize(ctx context.Context, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool {
	return desired <= s.AccessMode(ctx, userID, repoID, opts)
}

// SetRepoPerms does a full update to which users have which level of access to
// given repository. Keys of the "accessMap" are user IDs.
func (s *PermissionsStore) SetRepoPerms(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error {
	records := make([]*Access, 0, len(accessMap))
	for userID, mode := range accessMap {
		records = append(records, &Access{
			UserID: userID,
			RepoID: repoID,
			Mode:   mode,
		})
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("repo_id = ?", repoID).Delete(new(Access)).Error
		if err != nil {
			return err
		}

		return tx.Create(&records).Error
	})
}
