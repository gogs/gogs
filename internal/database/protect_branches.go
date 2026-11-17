package database

import (
	"context"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/tool"
)

// ProtectBranchesStore is the storage layer for the protection of branches.
type ProtectBranchesStore struct {
	db *gorm.DB
}

func newProtectBranchesStore(db *gorm.DB) *ProtectBranchesStore {
	return &ProtectBranchesStore{db: db}
}

// GetByName returns the protection options of the branch with given name in
// the repository. It returns ErrBranchNotExist when the branch has none.
func (s *ProtectBranchesStore) GetByName(ctx context.Context, repoID int64, name string) (*ProtectBranch, error) {
	protectBranch := new(ProtectBranch)
	err := s.db.WithContext(ctx).Where("repo_id = ? AND name = ?", repoID, name).First(protectBranch).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBranchNotExist{args: map[string]any{"name": name}}
		}
		return nil, err
	}
	return protectBranch, nil
}

// ListProtected returns the protected branches of given repository, ordered by
// name.
func (s *ProtectBranchesStore) ListProtected(ctx context.Context, repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, s.db.WithContext(ctx).
		Where("repo_id = ? AND protected = ?", repoID, true).
		Order("name ASC").
		Find(&protectBranches).
		Error
}

// RequiresPullRequest returns true if the branch with given name in the
// repository is protected and takes changes only through pull requests.
func (s *ProtectBranchesStore) RequiresPullRequest(ctx context.Context, logger *logx.Logger, repoID int64, name string) bool {
	protectBranch, err := s.GetByName(ctx, repoID, name)
	if err != nil {
		if !IsErrBranchNotExist(err) {
			logger.Error("Failed to get protected branch", "repoID", repoID, "branch", name, "error", err)
		}
		return false
	}
	return protectBranch.Protected && protectBranch.RequirePullRequest
}

// IsUserInAllowlist returns whether the user is in the allowlist of the branch,
// by name or as a current member of an allowlisted team.
func (s *ProtectBranchesStore) IsUserInAllowlist(ctx context.Context, protectBranch *ProtectBranch, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if slices.Contains(tool.StringsToInt64s(strings.Split(protectBranch.AllowlistUserIDs, ",")), userID) {
		return true, nil
	}
	teamIDs := tool.StringsToInt64s(strings.Split(protectBranch.AllowlistTeamIDs, ","))
	if len(teamIDs) == 0 {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).Model(new(TeamUser)).Where("team_id IN ? AND uid = ?", teamIDs, userID).Count(&count).Error
	return count > 0, err
}

// getRepository returns the repository with given ID and its owner.
func (s *ProtectBranchesStore) getRepository(ctx context.Context, repoID int64) (*Repository, error) {
	repo, err := newReposStore(s.db).GetByID(ctx, repoID)
	if err != nil {
		return nil, errors.Wrap(err, "get repository")
	}
	repo.Owner, err = newUsersStore(s.db).GetByID(ctx, repo.OwnerID)
	if err != nil {
		return nil, errors.Wrap(err, "get owner")
	}
	return repo, nil
}

// Save creates the protection options of the branch when its ID is 0, and
// updates them otherwise. The pre-receive hook that enforces the protection is
// installed before they are saved, so that a failure leaves the branch
// unprotected rather than protected but not enforced, and the delegate hooks
// of the repository are synced after.
func (s *ProtectBranchesStore) Save(ctx context.Context, protectBranch *ProtectBranch) error {
	repo, err := s.getRepository(ctx, protectBranch.RepoID)
	if err != nil {
		return err
	}
	if protectBranch.Protected {
		err = writeDelegateHook(repo.RepoPath(), git.HookPreReceive)
		if err != nil {
			return err
		}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if protectBranch.ID == 0 {
			err := tx.Create(protectBranch).Error
			if err != nil {
				return errors.Wrap(err, "create")
			}
		}
		return tx.Model(protectBranch).Select("*").Updates(protectBranch).Error
	})
	if err != nil {
		return err
	}
	return syncRepoDelegateHooks(s.db.WithContext(ctx), repo)
}

// SaveForOrg saves the protection options of the branch of an organization's
// repository like Save, with given allowlist. Users and teams of the allowlist
// who cannot write to the repository are dropped.
func (s *ProtectBranchesStore) SaveForOrg(ctx context.Context, logger *logx.Logger, protectBranch *ProtectBranch, allowlistUserIDs, allowlistTeamIDs []int64) error {
	repo, err := s.getRepository(ctx, protectBranch.RepoID)
	if err != nil {
		return err
	} else if !repo.Owner.IsOrganization() {
		return errors.New("expect repository owner to be an organization")
	}

	permissions := newPermissionsStore(s.db)
	validUserIDs := make([]int64, 0, len(allowlistUserIDs))
	for _, userID := range allowlistUserIDs {
		if userID > 0 && permissions.Authorize(ctx, logger, userID, repo.ID, AccessModeWrite,
			AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			validUserIDs = append(validUserIDs, userID)
		}
	}
	protectBranch.AllowlistUserIDs = strings.Join(tool.Int64sToStrings(validUserIDs), ",")

	var writerTeamIDs []int64
	err = s.db.WithContext(ctx).
		Table("team").
		Joins("INNER JOIN team_repo ON team_repo.team_id = team.id").
		Where("team_repo.org_id = ? AND team_repo.repo_id = ? AND team.authorize >= ?", repo.OwnerID, repo.ID, AccessModeWrite).
		Pluck("team.id", &writerTeamIDs).
		Error
	if err != nil {
		return errors.Wrap(err, "list teams that can write")
	}
	validTeamIDs := make([]int64, 0, len(allowlistTeamIDs))
	for _, teamID := range writerTeamIDs {
		if slices.Contains(allowlistTeamIDs, teamID) {
			validTeamIDs = append(validTeamIDs, teamID)
		}
	}
	protectBranch.AllowlistTeamIDs = strings.Join(tool.Int64sToStrings(validTeamIDs), ",")

	return s.Save(ctx, protectBranch)
}
