package database

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/tool"
)

type Branch struct {
	RepoPath string
	Name     string

	IsProtected bool
	Commit      *git.Commit
}

func GetBranchesByPath(ctx context.Context, path string) ([]*Branch, error) {
	names, err := git.RepoBranches(ctx, path)
	if err != nil {
		return nil, errors.Newf("list branches")
	}

	branches := make([]*Branch, len(names))
	for i := range names {
		branches[i] = &Branch{
			RepoPath: path,
			Name:     names[i],
		}
	}
	return branches, nil
}

var _ errx.NotFound = (*ErrBranchNotExist)(nil)

type ErrBranchNotExist struct {
	args map[string]any
}

func IsErrBranchNotExist(err error) bool {
	_, ok := err.(ErrBranchNotExist)
	return ok
}

func (err ErrBranchNotExist) Error() string {
	return fmt.Sprintf("branch does not exist: %v", err.args)
}

func (ErrBranchNotExist) NotFound() bool {
	return true
}

func (r *Repository) GetBranch(ctx context.Context, name string) (*Branch, error) {
	if !git.RepoHasBranch(ctx, r.RepoPath(), name) {
		return nil, ErrBranchNotExist{args: map[string]any{"name": name}}
	}
	return &Branch{
		RepoPath: r.RepoPath(),
		Name:     name,
	}, nil
}

func (r *Repository) GetBranches(ctx context.Context) ([]*Branch, error) {
	return GetBranchesByPath(ctx, r.RepoPath())
}

func (br *Branch) GetCommit(ctx context.Context) (*git.Commit, error) {
	return git.RepoBranchCommit(ctx, br.RepoPath, br.Name)
}

// ProtectBranch contains options of a protected branch.
type ProtectBranch struct {
	ID                 int64
	RepoID             int64  `xorm:"UNIQUE(protect_branch)"`
	Name               string `xorm:"UNIQUE(protect_branch)"`
	Protected          bool
	RequirePullRequest bool
	// The allowlist columns keep their legacy names.
	EnableAllowlist  bool   `xorm:"'enable_whitelist'"`
	AllowlistUserIDs string `xorm:"'whitelist_user_i_ds' TEXT"`
	AllowlistTeamIDs string `xorm:"'whitelist_team_i_ds' TEXT"`
}

// GetProtectBranchOfRepoByName returns *ProtectBranch by branch name in given repository.
func GetProtectBranchOfRepoByName(repoID int64, name string) (*ProtectBranch, error) {
	protectBranch := &ProtectBranch{
		RepoID: repoID,
		Name:   name,
	}
	has, err := x.Get(protectBranch)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrBranchNotExist{args: map[string]any{"name": name}}
	}
	return protectBranch, nil
}

// IsUserInAllowlist returns whether the user is in the allowlist of the branch,
// by name or as a current member of an allowlisted team.
func (pb *ProtectBranch) IsUserInAllowlist(userID int64) (bool, error) {
	if userID <= 0 {
		return false, nil
	}
	if slices.Contains(tool.StringsToInt64s(strings.Split(pb.AllowlistUserIDs, ",")), userID) {
		return true, nil
	}
	teamIDs := tool.StringsToInt64s(strings.Split(pb.AllowlistTeamIDs, ","))
	return x.In("team_id", teamIDs).And("uid = ?", userID).Exist(new(TeamUser))
}

// IsBranchOfRepoRequirePullRequest returns true if branch requires pull request in given repository.
func IsBranchOfRepoRequirePullRequest(repoID int64, name string) bool {
	protectBranch, err := GetProtectBranchOfRepoByName(repoID, name)
	if err != nil {
		return false
	}
	return protectBranch.Protected && protectBranch.RequirePullRequest
}

// UpdateProtectBranch saves branch protection options.
// If ID is 0, it creates a new record. Otherwise, updates existing record.
func UpdateProtectBranch(protectBranch *ProtectBranch) (err error) {
	repo, err := GetRepositoryByID(protectBranch.RepoID)
	if err != nil {
		return errors.Wrap(err, "get repository")
	}
	delegateHooksPool.CheckIn(strconv.FormatInt(repo.ID, 10))
	defer delegateHooksPool.CheckOut(strconv.FormatInt(repo.ID, 10))
	if err = enforceProtectBranch(repo, protectBranch); err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if protectBranch.ID == 0 {
		if _, err = sess.Insert(protectBranch); err != nil {
			return errors.Newf("insert: %v", err)
		}
	}

	if _, err = sess.ID(protectBranch.ID).AllCols().Update(protectBranch); err != nil {
		return errors.Newf("update: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return err
	}
	return repo.syncDelegateHooks()
}

// enforceProtectBranch installs the pre-receive hook that enforces the
// protection of the branch before it is saved, so that a failure leaves the
// branch unprotected rather than protected but not enforced.
func enforceProtectBranch(repo *Repository, protectBranch *ProtectBranch) error {
	if !protectBranch.Protected {
		return nil
	}
	return writeDelegateHook(repo.RepoPath(), git.HookPreReceive)
}

// UpdateOrgProtectBranch saves branch protection options of organizational
// repository, dropping users and teams of the allowlist who cannot write to the
// repository. If ID is 0, it creates a new record. Otherwise, updates existing
// record.
func UpdateOrgProtectBranch(logger *logx.Logger, repo *Repository, protectBranch *ProtectBranch, allowlistUserIDs, allowlistTeamIDs string) (err error) {
	if err = repo.GetOwner(); err != nil {
		return errors.Newf("GetOwner: %v", err)
	} else if !repo.Owner.IsOrganization() {
		return errors.Newf("expect repository owner to be an organization")
	}

	userIDs := tool.StringsToInt64s(strings.Split(allowlistUserIDs, ","))
	validUserIDs := make([]int64, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID > 0 && Handle.Permissions().Authorize(context.TODO(), logger, userID, repo.ID, AccessModeWrite,
			AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			},
		) {
			validUserIDs = append(validUserIDs, userID)
		}
	}
	protectBranch.AllowlistUserIDs = strings.Join(tool.Int64sToStrings(validUserIDs), ",")

	teamIDs := tool.StringsToInt64s(strings.Split(allowlistTeamIDs, ","))
	teams, err := GetTeamsHaveAccessToRepo(repo.OwnerID, repo.ID, AccessModeWrite)
	if err != nil {
		return errors.Newf("GetTeamsHaveAccessToRepo [org_id: %d, repo_id: %d]: %v", repo.OwnerID, repo.ID, err)
	}
	validTeamIDs := make([]int64, 0, len(teams))
	for i := range teams {
		if teams[i].HasWriteAccess() && slices.Contains(teamIDs, teams[i].ID) {
			validTeamIDs = append(validTeamIDs, teams[i].ID)
		}
	}
	protectBranch.AllowlistTeamIDs = strings.Join(tool.Int64sToStrings(validTeamIDs), ",")

	return UpdateProtectBranch(protectBranch)
}

// GetProtectBranchesByRepoID returns a list of *ProtectBranch in given repository.
func GetProtectBranchesByRepoID(repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, x.Where("repo_id = ? and protected = ?", repoID, true).Asc("name").Find(&protectBranches)
}
