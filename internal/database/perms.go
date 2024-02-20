// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"

	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
)

// PermsStore is the persistent interface for permissions.
type PermsStore interface {
	// AccessMode returns the access mode of given user has to the repository.
	AccessMode(ctx context.Context, userID, repoID int64, opts AccessModeOptions) AccessMode
	// Authorize returns true if the user has as good as desired access mode to the
	// repository.
	Authorize(ctx context.Context, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool
	// SetRepoPerms does a full update to which users have which level of access to
	// given repository. Keys of the "accessMap" are user IDs.
	SetRepoPerms(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error
}

var Perms PermsStore

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

var _ PermsStore = (*permsStore)(nil)

type permsStore struct {
	*gorm.DB
}

// NewPermsStore returns a persistent interface for permissions with given
// database connection.
func NewPermsStore(db *gorm.DB) PermsStore {
	return &permsStore{DB: db}
}

type AccessModeOptions struct {
	OwnerID int64 // The ID of the repository owner.
	Private bool  // Whether the repository is private.
}

func (s *permsStore) AccessMode(ctx context.Context, userID, repoID int64, opts AccessModeOptions) (mode AccessMode) {
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
	err := s.WithContext(ctx).Where("user_id = ? AND repo_id = ?", userID, repoID).First(access).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			log.Error("Failed to get access [user_id: %d, repo_id: %d]: %v", userID, repoID, err)
		}
		return mode
	}
	return access.Mode
}

func (s *permsStore) Authorize(ctx context.Context, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool {
	return desired <= s.AccessMode(ctx, userID, repoID, opts)
}

func (s *permsStore) SetRepoPerms(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error {
	records := make([]*Access, 0, len(accessMap))
	for userID, mode := range accessMap {
		records = append(records, &Access{
			UserID: userID,
			RepoID: repoID,
			Mode:   mode,
		})
	}

	return s.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("repo_id = ?", repoID).Delete(new(Access)).Error
		if err != nil {
			return err
		}

		return tx.Create(&records).Error
	})
}
