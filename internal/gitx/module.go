package gitx

import (
	"context"

	"gogs.io/gogs/internal/git"
)

// ModuleStore is the interface for Git operations.
type ModuleStore interface {
	// RemoteAdd adds a new remote to the repository in given path.
	RemoteAdd(ctx context.Context, repoPath, name, url string, opts ...git.RemoteAddOptions) error
	// DiffNameOnly returns a list of changed files between base and head revisions
	// of the repository in given path.
	DiffNameOnly(ctx context.Context, repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	// Log returns a list of commits in the state of given revision of the
	// repository in given path. The returned list is in reverse chronological
	// order.
	Log(ctx context.Context, repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	// MergeBase returns merge base between base and head revisions of the
	// repository in given path.
	MergeBase(ctx context.Context, repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	// RemoteRemove removes a remote from the repository in given path.
	RemoteRemove(ctx context.Context, repoPath, name string, opts ...git.RemoteRemoveOptions) error
	// RepoTags returns a list of tags of the repository in given path.
	RepoTags(ctx context.Context, repoPath string, opts ...git.TagsOptions) ([]string, error)

	// PullRequestMeta gathers pull request metadata based on given head and base
	// information.
	PullRequestMeta(ctx context.Context, headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	// ListTagsAfter returns a list of tags "after" (exclusive) given tag.
	ListTagsAfter(ctx context.Context, repoPath, after string, limit int) (*TagsPage, error)
}

// module holds the real implementation.
type module struct{}

func (module) RemoteAdd(ctx context.Context, repoPath, name, url string, opts ...git.RemoteAddOptions) error {
	r, err := git.Open(repoPath)
	if err != nil {
		return err
	}
	return r.RemoteAdd(ctx, name, url, opts...)
}

func (module) DiffNameOnly(ctx context.Context, repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	r, err := git.Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.DiffNameOnly(ctx, base, head, opts...)
}

func (module) Log(ctx context.Context, repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	r, err := git.Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.Log(ctx, rev, opts...)
}

func (module) MergeBase(ctx context.Context, repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	r, err := git.Open(repoPath)
	if err != nil {
		return "", err
	}
	return r.MergeBase(ctx, base, head, opts...)
}

func (module) RemoteRemove(ctx context.Context, repoPath, name string, opts ...git.RemoteRemoveOptions) error {
	r, err := git.Open(repoPath)
	if err != nil {
		return err
	}
	return r.RemoteRemove(ctx, name, opts...)
}

func (module) RepoTags(ctx context.Context, repoPath string, opts ...git.TagsOptions) ([]string, error) {
	r, err := git.Open(repoPath)
	if err != nil {
		return nil, err
	}
	return r.Tags(ctx, opts...)
}

// Module is a mockable interface for Git operations.
var Module ModuleStore = module{}
