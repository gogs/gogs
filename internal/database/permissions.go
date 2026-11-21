package database

import (
	"cmp"
	"context"
	"slices"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
)

// Access represents the highest access level that a user has to a repository. The
// only access type that is not in this table is the real owner of a repository.
// In case of an organization repository, the members of the owners team are in
// this table.
type Access struct {
	ID     int64      `gorm:"primaryKey"`
	UserID int64      `gorm:"uniqueIndex:access_user_repo_unique;not null"`
	RepoID int64      `gorm:"uniqueIndex:access_user_repo_unique;index:IDX_access_repo_id;not null"`
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

// SetRepoPerms sets which users have which access mode to the repository.
// Keys of the "accessMap" are user IDs, and users not in it lose their access.
// It writes only the rows that differ, so that it keeps concurrent changes to
// the accesses of other users, and does not fail on concurrent rows of the
// same users.
func (s *PermissionsStore) SetRepoPerms(ctx context.Context, repoID int64, accessMap map[int64]AccessMode) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// On SQLite, two transactions that read before they write deadlock each
		// other, so take the write lock first.
		if conf.UseSQLite3 {
			err := tx.Exec("UPDATE access SET mode = mode WHERE repo_id = ?", repoID).Error
			if err != nil {
				return errors.Wrap(err, "lock accesses")
			}
		}

		var existing []*Access
		err := tx.Where("repo_id = ?", repoID).Find(&existing).Error
		if err != nil {
			return errors.Wrap(err, "list accesses")
		}

		var removed []int64
		have := make(map[int64]AccessMode, len(existing))
		for _, a := range existing {
			have[a.UserID] = a.Mode
			if _, ok := accessMap[a.UserID]; !ok {
				removed = append(removed, a.UserID)
			}
		}
		var changed []*Access
		for userID, mode := range accessMap {
			if m, ok := have[userID]; !ok || m != mode {
				changed = append(changed, &Access{UserID: userID, RepoID: repoID, Mode: mode})
			}
		}
		// Sorted, so that concurrent calls lock the rows in the same order.
		slices.Sort(removed)
		slices.SortFunc(changed, func(a, b *Access) int { return cmp.Compare(a.UserID, b.UserID) })

		// In chunks, because a statement can bind only so many parameters.
		for userIDs := range slices.Chunk(removed, 500) {
			err = tx.Where("repo_id = ? AND user_id IN ?", repoID, userIDs).Delete(new(Access)).Error
			if err != nil {
				return errors.Wrap(err, "delete accesses")
			}
		}
		if len(changed) == 0 {
			return nil
		}
		return errors.Wrap(tx.Clauses(upsertAccess).CreateInBatches(changed, 500).Error, "upsert accesses")
	})
}

// upsertAccess makes inserting an access update the mode of an existing one.
var upsertAccess = clause.OnConflict{
	Columns:   []clause.Column{{Name: "user_id"}, {Name: "repo_id"}},
	DoUpdates: clause.AssignmentColumns([]string{"mode"}),
}

// recalculateUserAccess sets the user's access mode to the repository from
// their collaboration and, when an organization owns the repository, from
// their teams. Unlike recalculateAccesses, it writes only the user's row.
func (s *PermissionsStore) recalculateUserAccess(ctx context.Context, repo *Repository, userID int64) error {
	db := s.db.WithContext(ctx)
	mode := AccessModeNone
	collaboration := new(Collaboration)
	err := db.Where("repo_id = ? AND user_id = ?", repo.ID, userID).Take(collaboration).Error
	if err == nil {
		mode = collaboration.Mode
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return errors.Wrap(err, "get collaboration")
	}

	if err = repo.getOwner(db); err != nil {
		return errors.Wrap(err, "get owner")
	}
	if repo.Owner.IsOrganization() {
		var teams []*Team
		err = db.Joins("JOIN team_user ON team_user.team_id = team.id").
			Where("team.org_id = ? AND team_user.uid = ?", repo.OwnerID, userID).
			Find(&teams).Error
		if err != nil {
			return errors.Wrap(err, "list teams")
		}
		for _, t := range teams {
			if t.IsOwnerTeam() {
				mode = AccessModeOwner
			} else if newTeamsStore(db).HasRepo(ctx, repo.OwnerID, t.ID, repo.ID) {
				mode = max(mode, t.Authorize)
			}
		}
	}

	if mode == AccessModeNone {
		err = db.Where("user_id = ? AND repo_id = ?", userID, repo.ID).Delete(new(Access)).Error
		return errors.Wrap(err, "delete access")
	}
	err = db.Clauses(upsertAccess).Create(&Access{UserID: userID, RepoID: repo.ID, Mode: mode}).Error
	return errors.Wrap(err, "upsert access")
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

// recalculateTeamAccesses sets the access modes of an organization's repository
// from its collaborators and the organization's teams, except the team with
// given ID. The owner team always gets owner access, and a member of several
// teams gets their highest mode.
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

	teams := newTeamsStore(s.db)
	orgTeams, err := teams.ListByOrg(ctx, repo.OwnerID)
	if err != nil {
		return errors.Wrap(err, "list teams")
	}
	for _, t := range orgTeams {
		if t.ID == ignoredTeamID {
			continue
		}

		if t.IsOwnerTeam() {
			t.Authorize = AccessModeOwner
		} else if !teams.HasRepo(ctx, repo.OwnerID, t.ID, repo.ID) {
			continue
		}

		members, err := teams.ListMembers(ctx, t.ID)
		if err != nil {
			return errors.Wrapf(err, "list members of team %d", t.ID)
		}
		for _, m := range members {
			accessMap[m.ID] = max(accessMap[m.ID], t.Authorize)
		}
	}
	return s.SetRepoPerms(ctx, repo.ID, accessMap)
}
