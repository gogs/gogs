package database

import (
	"context"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

// Access represents the highest access level of a user has to a repository. The
// only access type that is not in this table is the real owner of a repository.
// In case of an organization repository, the members of the owners team are in
// this table.
type Access struct {
	ID     int64      `gorm:"primaryKey"`
	UserID int64      `gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64      `gorm:"uniqueIndex:access_user_repo_unique;not null"`
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
func (s *PermissionsStore) AccessMode(ctx context.Context, logger *logx.Logger, userID, repoID int64, opts AccessModeOptions) (mode AccessMode) {
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
			logger.Error("Failed to get access", "userID", userID, "repoID", repoID, "error", err)
		}
		return mode
	}
	return access.Mode
}

// Authorize returns true if the user has as good as desired access mode to the
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
		} else if len(records) == 0 {
			return nil
		}

		return tx.Create(&records).Error
	})
}

// recalculateAccesses sets the access modes of the repository from its
// collaborators and, when an organization owns it, from the teams of the
// organization.
func (s *PermissionsStore) recalculateAccesses(ctx context.Context, repo *Repository) error {
	if err := repo.getOwner(s.db.WithContext(ctx)); err != nil {
		return errors.Wrap(err, "get owner")
	}
	if repo.Owner.IsOrganization() {
		return s.recalculateTeamAccesses(ctx, repo, 0)
	}

	accessMap, err := s.collaboratorAccesses(ctx, repo.ID)
	if err != nil {
		return err
	}
	return s.SetRepoPerms(ctx, repo.ID, accessMap)
}

// collaboratorAccesses returns the access modes of the collaborators of the
// repository with given ID, keyed by their user IDs.
func (s *PermissionsStore) collaboratorAccesses(ctx context.Context, repoID int64) (map[int64]AccessMode, error) {
	collaborations, err := newCollaborationsStore(s.db).listByRepoID(ctx, repoID)
	if err != nil {
		return nil, errors.Wrap(err, "list collaborations")
	}

	accessMap := make(map[int64]AccessMode, len(collaborations))
	for _, c := range collaborations {
		accessMap[c.UserID] = c.Mode
	}
	return accessMap, nil
}

// recalculateTeamAccesses sets the access modes of the repository, which an
// organization owns, from its collaborators and the teams of the organization
// other than the team with given ID. The owner team has owner access to every
// repository of the organization, and a member of several teams gets the
// highest access mode among them.
func (s *PermissionsStore) recalculateTeamAccesses(ctx context.Context, repo *Repository, ignoredTeamID int64) error {
	db := s.db.WithContext(ctx)
	if err := repo.getOwner(db); err != nil {
		return errors.Wrap(err, "get owner")
	} else if !repo.Owner.IsOrganization() {
		return errors.Newf("owner is not an organization: %d", repo.OwnerID)
	}

	accessMap, err := s.collaboratorAccesses(ctx, repo.ID)
	if err != nil {
		return err
	}

	if err = repo.Owner.getTeams(db); err != nil {
		return errors.Wrap(err, "list teams")
	}
	for _, t := range repo.Owner.Teams {
		if t.ID == ignoredTeamID {
			continue
		}

		if t.IsOwnerTeam() {
			t.Authorize = AccessModeOwner
		} else if !t.hasRepository(db, repo.ID) {
			continue
		}

		if err = t.getMembers(db); err != nil {
			return errors.Wrapf(err, "list members of team %d", t.ID)
		}
		for _, m := range t.Members {
			accessMap[m.ID] = max(accessMap[m.ID], t.Authorize)
		}
	}
	return s.SetRepoPerms(ctx, repo.ID, accessMap)
}
