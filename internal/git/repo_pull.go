package git

import (
	"context"
	"strings"

	"github.com/cockroachdb/errors"
)

// MergeBase returns merge base between base and head revisions of the
// repository. It returns ErrNoMergeBase when there is none, including when a
// revision does not name a commit, e.g., a branch that only exists in another
// repository.
func (r *Repository) MergeBase(ctx context.Context, base, head string) (string, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.MergeBase")
	defer span.End()

	return r.mergeBase(ctx, base, head, nil)
}

// mergeBase is like MergeBase but runs Git with given environment variables.
func (r *Repository) mergeBase(ctx context.Context, base, head string, envs []string) (string, error) {
	args := []string{"merge-base", "--end-of-options", base, head}
	stdout, err := exec(ctx, r.path, args, envs)
	if err != nil {
		if isExitStatus(err, 1) || isExitStatus(err, 128) {
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

// PullRequestMetaOptions contains optional arguments for gathering metadata
// of a pull request.
type PullRequestMetaOptions struct {
	// The commit to compare the head branch against instead of the merge base
	// with the base branch, e.g., a commit that merges the branches the pull
	// request depends on. It must be in the repository.
	Base string
}

// PullRequestMeta gathers metadata of a pull request that merges the head
// branch of the repository into the base branch of the repository at the
// basePath, which may be the same repository.
func (r *Repository) PullRequestMeta(ctx context.Context, basePath, headBranch, baseBranch string, opts ...PullRequestMetaOptions) (*PullRequestMeta, error) {
	ctx, span := tracer.Start(ctx, "git.Repository.PullRequestMeta")
	defer span.End()

	var opt PullRequestMetaOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Base != "" {
		return r.pullRequestMeta(ctx, opt.Base, headBranch)
	}

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
		// Git splits the value on the path list separator unless the entry is
		// double-quoted, so quote it to support paths containing ":" (or ";" on
		// Windows).
		objectsDir := strings.TrimSpace(string(stdout))
		envs = []string{"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + quoteAlternatePath(objectsDir)}
	}

	mergeBase, err := r.mergeBase(ctx, base, headBranch, envs)
	if err != nil {
		return nil, errors.Wrap(err, "get merge base")
	}
	return r.pullRequestMeta(ctx, mergeBase, headBranch)
}

// pullRequestMeta gathers metadata of a pull request that compares the head
// branch against the merge base.
func (r *Repository) pullRequestMeta(ctx context.Context, mergeBase, headBranch string) (*PullRequestMeta, error) {
	// A given base may not be an ancestor of the head branch, so only list the
	// commits of the head branch.
	commits, err := r.Log(ctx, mergeBase+".."+headBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get commits")
	}

	names, err := r.DiffNameOnly(ctx, mergeBase, headBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get changed files")
	}

	return &PullRequestMeta{
		MergeBase: mergeBase,
		Commits:   commits,
		NumFiles:  len(names),
	}, nil
}

// quoteAlternatePath quotes the path in the C style that Git accepts for an
// entry of GIT_ALTERNATE_OBJECT_DIRECTORIES.
func quoteAlternatePath(path string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(path) + `"`
}
