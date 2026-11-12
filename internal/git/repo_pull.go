package git

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
)

// MergeBaseOptions contains optional arguments for getting merge base.
//
// Docs: https://git-scm.com/docs/git-merge-base
type MergeBaseOptions struct {
	CommandOptions
}

// MergeBase returns merge base between base and head revisions of the
// repository.
func (r *Repository) MergeBase(ctx context.Context, base, head string, opts ...MergeBaseOptions) (string, error) {
	var opt MergeBaseOptions
	if len(opts) > 0 {
		opt = opts[0]
	}

	args := []string{"merge-base", "--end-of-options", base, head}
	stdout, err := exec(ctx, r.path, args, opt.Envs)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", ErrNoMergeBase
		}
		return "", err
	}
	return strings.TrimSpace(string(stdout)), nil
}

// PullRequestMeta contains metadata for a pull request.
type PullRequestMeta struct {
	// The merge base of the pull request.
	MergeBase string
	// The commits that are requested to be merged.
	Commits []*Commit
	// The number of files changed.
	NumFiles int
}

// PullRequestMeta gathers metadata of a pull request that merges the head
// branch of the repository into the base branch of the repository at the
// basePath, which may be the same repository.
func (r *Repository) PullRequestMeta(ctx context.Context, basePath, headBranch, baseBranch string) (*PullRequestMeta, error) {
	base := baseBranch
	var envs []string

	// When the pull request is sent from a forked repository, read objects of the
	// base repository through an alternate object directory instead of fetching
	// them, so nothing is ever written to the head repository.
	if r.path != basePath {
		baseRepo, err := Open(basePath)
		if err != nil {
			return nil, errors.Wrap(err, "open base repository")
		}
		base, err = baseRepo.BranchCommitID(ctx, baseBranch)
		if err != nil {
			return nil, errors.Wrap(err, "get base branch commit ID")
		}
		stdout, err := exec(ctx, baseRepo.path, []string{"rev-parse", "--path-format=absolute", "--git-path", "objects"}, nil)
		if err != nil {
			return nil, errors.Wrap(err, "get base objects directory")
		}
		envs = []string{"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + strings.TrimSpace(string(stdout))}
	}

	mergeBase, err := r.MergeBase(ctx, base, headBranch, MergeBaseOptions{CommandOptions: CommandOptions{Envs: envs}})
	if err != nil {
		return nil, errors.Wrap(err, "get merge base")
	}

	commits, err := r.Log(ctx, mergeBase+"..."+headBranch, LogOptions{CommandOptions: CommandOptions{Envs: envs}})
	if err != nil {
		return nil, errors.Wrap(err, "get commits")
	}

	names, err := r.DiffNameOnly(ctx, base, headBranch, DiffNameOnlyOptions{NeedsMergeBase: true, CommandOptions: CommandOptions{Envs: envs}})
	if err != nil {
		return nil, errors.Wrap(err, "get changed files")
	}

	return &PullRequestMeta{
		MergeBase: mergeBase,
		Commits:   commits,
		NumFiles:  len(names),
	}, nil
}
