package database

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/git"
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

// PushDenial is the rule of a protected branch that refuses an update of the
// branch, or empty when none does.
type PushDenial string

const (
	PushDeniedNotInAllowlist      PushDenial = "not_in_allowlist"
	PushDeniedRequiresPullRequest PushDenial = "requires_pull_request"
)

// CheckPushByName is CheckPush for the branch with given name in the
// repository, which may not be protected at all.
func (s *ProtectBranchesStore) CheckPushByName(ctx context.Context, repoID int64, branch string, userID int64, viaPullRequest bool) (PushDenial, error) {
	protectBranch, err := s.GetByName(ctx, repoID, branch)
	if err != nil {
		if IsErrBranchNotExist(err) {
			return "", nil
		}
		return "", errors.Wrap(err, "get protected branch")
	}
	return s.CheckPush(ctx, protectBranch, userID, viaPullRequest)
}

// CheckPush returns the rule of the branch protection that refuses the user,
// who can already write to the repository, to update the branch, which a
// merge of a pull request does when viaPullRequest is true. Deletion and force
// pushes, which a protected branch refuses to everyone, are left to the
// caller. Repository and site admins are bound like everyone else.
func (s *ProtectBranchesStore) CheckPush(ctx context.Context, protectBranch *ProtectBranch, userID int64, viaPullRequest bool) (PushDenial, error) {
	if !protectBranch.Protected {
		return "", nil
	}
	if protectBranch.EnableAllowlist {
		inAllowlist, err := s.isUserInAllowlist(ctx, protectBranch, userID)
		if err != nil {
			return "", errors.Wrap(err, "check allowlist")
		} else if !inAllowlist {
			return PushDeniedNotInAllowlist, nil
		}
		// People in the allowlist can push without a pull request.
		return "", nil
	}
	if protectBranch.RequirePullRequest && !viaPullRequest {
		return PushDeniedRequiresPullRequest, nil
	}
	return "", nil
}

// isUserInAllowlist returns whether the user is in the allowlist of the branch,
// by name or as a current member of an allowlisted team that still has the
// repository. Teams of an organization the repository was transferred from
// no longer count.
func (s *ProtectBranchesStore) isUserInAllowlist(ctx context.Context, protectBranch *ProtectBranch, userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	for field := range strings.SplitSeq(protectBranch.AllowlistUserIDs, ",") {
		if id, _ := strconv.ParseInt(field, 10, 64); id == userID {
			return true, nil
		}
	}
	var teamIDs []int64
	for field := range strings.SplitSeq(protectBranch.AllowlistTeamIDs, ",") {
		if id, _ := strconv.ParseInt(field, 10, 64); id > 0 {
			teamIDs = append(teamIDs, id)
		}
	}
	if len(teamIDs) == 0 {
		return false, nil
	}
	var count int64
	err := s.db.WithContext(ctx).
		Model(new(TeamUser)).
		Joins("INNER JOIN team_repo ON team_repo.team_id = team_user.team_id").
		Where("team_user.team_id IN ? AND team_user.uid = ? AND team_repo.repo_id = ?", teamIDs, userID, protectBranch.RepoID).
		Count(&count).
		Error
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
// of the repository are synced after, all written with the hooks options.
func (s *ProtectBranchesStore) Save(ctx context.Context, hooks DelegateHooksOptions, protectBranch *ProtectBranch) error {
	repo, err := s.getRepository(ctx, protectBranch.RepoID)
	if err != nil {
		return err
	}
	if protectBranch.Protected {
		err = writeDelegateHook(hooks, repo.RepoPath(hooks.RepositoryRoot), git.HookPreReceive)
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
	return syncRepoDelegateHooks(s.db.WithContext(ctx), hooks, repo)
}

// SaveForOrg saves the protection options of the branch of an organization's
// repository like Save, with given allowlist. Users and teams of the allowlist
// who cannot write to the repository are dropped.
func (s *ProtectBranchesStore) SaveForOrg(ctx context.Context, logger *logx.Logger, hooks DelegateHooksOptions, protectBranch *ProtectBranch, allowlistUserIDs, allowlistTeamIDs []int64) error {
	repo, err := s.getRepository(ctx, protectBranch.RepoID)
	if err != nil {
		return err
	} else if !repo.Owner.IsOrganization() {
		return errors.New("expect repository owner to be an organization")
	}

	permissions := newPermissionsStore(s.db)
	validUserIDs := make([]string, 0, len(allowlistUserIDs))
	for _, userID := range allowlistUserIDs {
		if userID > 0 && permissions.Authorize(ctx, logger, userID, repo.ID, AccessModeWrite,
			AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			validUserIDs = append(validUserIDs, strconv.FormatInt(userID, 10))
		}
	}
	protectBranch.AllowlistUserIDs = strings.Join(validUserIDs, ",")

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
	validTeamIDs := make([]string, 0, len(allowlistTeamIDs))
	for _, teamID := range writerTeamIDs {
		if slices.Contains(allowlistTeamIDs, teamID) {
			validTeamIDs = append(validTeamIDs, strconv.FormatInt(teamID, 10))
		}
	}
	protectBranch.AllowlistTeamIDs = strings.Join(validTeamIDs, ",")

	return s.Save(ctx, hooks, protectBranch)
}
