package gitx

import (
	"context"
	"testing"

	"gogs.io/gogs/internal/git"
)

var _ ModuleStore = (*MockModuleStore)(nil)

type MockModuleStore struct {
	remoteAdd    func(ctx context.Context, repoPath, name, url string, opts ...git.RemoteAddOptions) error
	diffNameOnly func(ctx context.Context, repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	log          func(ctx context.Context, repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	mergeBase    func(ctx context.Context, repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	remoteRemove func(ctx context.Context, repoPath, name string, opts ...git.RemoteRemoveOptions) error
	repoTags     func(ctx context.Context, repoPath string, opts ...git.TagsOptions) ([]string, error)

	pullRequestMeta func(ctx context.Context, headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	listTagsAfter   func(ctx context.Context, repoPath, after string, limit int) (*TagsPage, error)
}

func (m *MockModuleStore) RemoteAdd(ctx context.Context, repoPath, name, url string, opts ...git.RemoteAddOptions) error {
	return m.remoteAdd(ctx, repoPath, name, url, opts...)
}

func (m *MockModuleStore) DiffNameOnly(ctx context.Context, repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	return m.diffNameOnly(ctx, repoPath, base, head, opts...)
}

func (m *MockModuleStore) Log(ctx context.Context, repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	return m.log(ctx, repoPath, rev, opts...)
}

func (m *MockModuleStore) MergeBase(ctx context.Context, repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	return m.mergeBase(ctx, repoPath, base, head, opts...)
}

func (m *MockModuleStore) RemoteRemove(ctx context.Context, repoPath, name string, opts ...git.RemoteRemoveOptions) error {
	return m.remoteRemove(ctx, repoPath, name, opts...)
}

func (m *MockModuleStore) RepoTags(ctx context.Context, repoPath string, opts ...git.TagsOptions) ([]string, error) {
	return m.repoTags(ctx, repoPath, opts...)
}

func (m *MockModuleStore) PullRequestMeta(ctx context.Context, headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error) {
	return m.pullRequestMeta(ctx, headPath, basePath, headBranch, baseBranch)
}

func (m *MockModuleStore) ListTagsAfter(ctx context.Context, repoPath, after string, limit int) (*TagsPage, error) {
	return m.listTagsAfter(ctx, repoPath, after, limit)
}

func SetMockModuleStore(t *testing.T, mock ModuleStore) {
	before := Module
	Module = mock
	t.Cleanup(func() {
		Module = before
	})
}
