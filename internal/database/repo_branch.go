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
	RepoID             int64  `gorm:"uniqueIndex:UQE_protect_branch_protect_branch"`
	Name               string `gorm:"size:255;uniqueIndex:UQE_protect_branch_protect_branch"`
	Protected          bool
	RequirePullRequest bool
	// The allowlist columns keep their legacy names.
	EnableAllowlist  bool   `gorm:"column:enable_whitelist"`
	AllowlistUserIDs string `gorm:"column:whitelist_user_i_ds;type:TEXT"`
	AllowlistTeamIDs string `gorm:"column:whitelist_team_i_ds;type:TEXT"`
	// RequiredApprovals is how many approving reviews from writers a pull
	// request needs to merge into the branch, or 0 when it needs none. When it
	// needs some, a writer's request for changes also blocks the merge.
	RequiredApprovals int `gorm:"not null;default:0"`
	// DismissStaleApprovals stops counting approvals of older head commits.
	DismissStaleApprovals bool `gorm:"not null;default:false"`
	// RequireResolvedConversations blocks the merge while a review thread is
	// unresolved.
	RequireResolvedConversations bool `gorm:"not null;default:false"`
}

// BranchAlreadyExists represents an error when branch already exists.
type BranchAlreadyExists struct {
	Name string
}

func (err BranchAlreadyExists) Error() string {
	return fmt.Sprintf("branch already exists [name: %s]", err.Name)
}
