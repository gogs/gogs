package git

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	clog "unknwon.dev/clog/v2"
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
	tmpRemoteBranch := baseBranch

	// We need to create a temporary remote when the pull request is sent from a forked repository.
	if r.path != basePath {
		tmpRemote := strconv.FormatInt(time.Now().UnixNano(), 10)
		err := r.RemoteAdd(ctx, tmpRemote, basePath, RemoteAddOptions{Fetch: true})
		if err != nil {
			return nil, fmt.Errorf("add remote: %v", err)
		}
		defer func() {
			err := r.RemoteRemove(ctx, tmpRemote)
			if err != nil {
				clog.Error("Failed to remove remote %q [path: %s]: %v", tmpRemote, r.path, err)
				return
			}
		}()

		tmpRemoteBranch = "remotes/" + tmpRemote + "/" + baseBranch
	}

	mergeBase, err := r.MergeBase(ctx, tmpRemoteBranch, headBranch)
	if err != nil {
		return nil, fmt.Errorf("get merge base: %w", err)
	}

	commits, err := r.Log(ctx, mergeBase+"..."+headBranch)
	if err != nil {
		return nil, fmt.Errorf("get commits: %w", err)
	}

	// Count number of changed files
	names, err := r.DiffNameOnly(ctx, tmpRemoteBranch, headBranch, DiffNameOnlyOptions{NeedsMergeBase: true})
	if err != nil {
		return nil, fmt.Errorf("get changed files: %w", err)
	}

	return &PullRequestMeta{
		MergeBase: mergeBase,
		Commits:   commits,
		NumFiles:  len(names),
	}, nil
}
