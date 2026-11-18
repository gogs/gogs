package database

import (
	"context"
	"fmt"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/git"
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
	EnableAllowlist  bool   `xorm:"'enable_whitelist'" gorm:"column:enable_whitelist"`
	AllowlistUserIDs string `xorm:"'whitelist_user_i_ds' TEXT" gorm:"column:whitelist_user_i_ds"`
	AllowlistTeamIDs string `xorm:"'whitelist_team_i_ds' TEXT" gorm:"column:whitelist_team_i_ds"`
}

// BranchAlreadyExists represents an error when branch already exists.
type BranchAlreadyExists struct {
	Name string
}

// IsBranchAlreadyExists returns true if the error is BranchAlreadyExists.
func IsBranchAlreadyExists(err error) bool {
	_, ok := err.(BranchAlreadyExists)
	return ok
}

func (err BranchAlreadyExists) Error() string {
	return fmt.Sprintf("branch already exists [name: %s]", err.Name)
}
