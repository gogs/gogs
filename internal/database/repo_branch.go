package database

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"

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

type ProtectBranchWhitelist struct {
	ID              int64
	ProtectBranchID int64
	RepoID          int64  `xorm:"UNIQUE(protect_branch_whitelist)"`
	Name            string `xorm:"UNIQUE(protect_branch_whitelist)"`
	UserID          int64  `xorm:"UNIQUE(protect_branch_whitelist)"`
}

// IsUserInProtectBranchWhitelist returns true if given user is in the whitelist
// of a branch in a repository, either by name or as a current member of a
// whitelisted team.
func IsUserInProtectBranchWhitelist(repoID, userID int64, branch string) bool {
	// Pushes with a deploy key have no user, and an empty list of IDs reads as
	// a single 0.
	if userID <= 0 {
		return false
	}
	protectBranch, err := GetProtectBranchOfRepoByName(repoID, branch)
	if err != nil {
		return false
	}
	if slices.Contains(tool.StringsToInt64s(strings.Split(protectBranch.WhitelistUserIDs, ",")), userID) {
		return true
	}

	teamIDs := tool.StringsToInt64s(strings.Split(protectBranch.WhitelistTeamIDs, ","))
	if len(teamIDs) == 0 {
		return false
	}
	has, err := x.Where("uid = ?", userID).In("team_id", teamIDs).Get(new(TeamUser))
	return has && err == nil
}

// ProtectBranch contains options of a protected branch.
type ProtectBranch struct {
	ID                 int64
	RepoID             int64  `xorm:"UNIQUE(protect_branch)"`
	Name               string `xorm:"UNIQUE(protect_branch)"`
	Protected          bool
	RequirePullRequest bool
	EnableWhitelist    bool
	WhitelistUserIDs   string `xorm:"TEXT"`
	WhitelistTeamIDs   string `xorm:"TEXT"`
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

	return sess.Commit()
}

// UpdateOrgProtectBranch saves branch protection options of organizational repository.
// If ID is 0, it creates a new record. Otherwise, updates existing record.
// Whitelisted users must have write access and whitelisted teams must have
// write access to the repository when they are added.
func UpdateOrgProtectBranch(repo *Repository, protectBranch *ProtectBranch, whitelistUserIDs, whitelistTeamIDs string) (err error) {
	if err = repo.GetOwner(); err != nil {
		return errors.Newf("GetOwner: %v", err)
	} else if !repo.Owner.IsOrganization() {
		return errors.Newf("expect repository owner to be an organization")
	}

	if protectBranch.WhitelistUserIDs != whitelistUserIDs {
		userIDs := tool.StringsToInt64s(strings.Split(whitelistUserIDs, ","))
		validUserIDs := make([]int64, 0, len(userIDs))
		for _, userID := range userIDs {
			if !Handle.Permissions().Authorize(context.TODO(), userID, repo.ID, AccessModeWrite,
				AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			) {
				continue // Drop invalid user ID
			}

			validUserIDs = append(validUserIDs, userID)
		}

		protectBranch.WhitelistUserIDs = strings.Join(tool.Int64sToStrings(validUserIDs), ",")
	}

	if protectBranch.WhitelistTeamIDs != whitelistTeamIDs {
		teamIDs := tool.StringsToInt64s(strings.Split(whitelistTeamIDs, ","))
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

		protectBranch.WhitelistTeamIDs = strings.Join(tool.Int64sToStrings(validTeamIDs), ",")
	}

	return UpdateProtectBranch(protectBranch)
}

// GetProtectBranchesByRepoID returns a list of *ProtectBranch in given repository.
func GetProtectBranchesByRepoID(repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, x.Where("repo_id = ? and protected = ?", repoID, true).Asc("name").Find(&protectBranches)
}
