package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

// Access represents the highest access level that a user has to a repository. The
// only access type that is not in this table is the real owner of a repository.
// In case of an organization repository, the members of the owners team are in
// this table.
type Access struct {
	ID     int64      `gorm:"primaryKey"`
	UserID int64      `xorm:"UNIQUE(s)" gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64      `xorm:"UNIQUE(s)" gorm:"uniqueIndex:access_user_repo_unique;not null"`
	Mode   AccessMode `gorm:"not null"`
}

// AccessMode is the access mode that a user has to a repository.
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

// implicitAccessMode returns the access mode that given user has to the
// repository without looking up the access table, and whether that mode is final.
func implicitAccessMode(userID int64, opts AccessModeOptions) (mode AccessMode, final bool) {
	// Everyone has read access to public repository.
	if !opts.Private {
		mode = AccessModeRead
	}

	// Anonymous user gets the default access.
	if userID <= 0 {
		return mode, true
	}

	if userID == opts.OwnerID {
		return AccessModeOwner, true
	}
	return mode, false
}

// AccessMode returns the access mode that given user has to the repository.
func (s *PermissionsStore) AccessMode(ctx context.Context, logger *logx.Logger, userID, repoID int64, opts AccessModeOptions) AccessMode {
	if repoID <= 0 {
		return AccessModeNone
	}

	mode, final := implicitAccessMode(userID, opts)
	if final {
		return mode
	}

	access := new(Access)
	err := s.db.WithContext(ctx).Where("user_id = ? AND repo_id = ?", userID, repoID).First(access).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Error("Failed to get access", "userID", userID, "repoID", repoID, "error", err)
		}
		return mode
	}
	return access.Mode
}

// AccessModes returns the access mode that given user has to each of the
// repositories, keyed by repository ID, with a single lookup of the access table.
func (s *PermissionsStore) AccessModes(ctx context.Context, userID int64, repos []*Repository) (map[int64]AccessMode, error) {
	modes := make(map[int64]AccessMode, len(repos))
	var lookupIDs []int64
	for _, repo := range repos {
		mode, final := implicitAccessMode(userID, AccessModeOptions{OwnerID: repo.OwnerID, Private: repo.IsPrivate})
		modes[repo.ID] = mode
		if !final {
			lookupIDs = append(lookupIDs, repo.ID)
		}
	}
	if len(lookupIDs) == 0 {
		return modes, nil
	}

	var accesses []*Access
	err := s.db.WithContext(ctx).Where("user_id = ? AND repo_id IN ?", userID, lookupIDs).Find(&accesses).Error
	if err != nil {
		return nil, errors.Wrap(err, "list accesses")
	}
	for _, access := range accesses {
		modes[access.RepoID] = access.Mode
	}
	return modes, nil
}

// Authorize returns true if the user has at least the desired access mode to the
// repository.
func (s *PermissionsStore) Authorize(ctx context.Context, logger *logx.Logger, userID, repoID int64, desired AccessMode, opts AccessModeOptions) bool {
	return desired <= s.AccessMode(ctx, logger, userID, repoID, opts)
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
