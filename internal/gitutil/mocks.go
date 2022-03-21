// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"testing"

	"github.com/gogs/git-module"
)

var _ ModuleStore = (*MockModuleStore)(nil)

type MockModuleStore struct {
	remoteAdd    func(repoPath, name, url string, opts ...git.RemoteAddOptions) error
	diffNameOnly func(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	log          func(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	mergeBase    func(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	remoteRemove func(repoPath, name string, opts ...git.RemoteRemoveOptions) error
	repoTags     func(repoPath string, opts ...git.TagsOptions) ([]string, error)

	pullRequestMeta func(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	listTagsAfter   func(repoPath, after string, limit int) (*TagsPage, error)
}

func (m *MockModuleStore) RemoteAdd(repoPath, name, url string, opts ...git.RemoteAddOptions) error {
	return m.remoteAdd(repoPath, name, url, opts...)
}

func (m *MockModuleStore) DiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	return m.diffNameOnly(repoPath, base, head, opts...)
}

func (m *MockModuleStore) Log(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	return m.log(repoPath, rev, opts...)
}

func (m *MockModuleStore) MergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	return m.mergeBase(repoPath, base, head, opts...)
}

func (m *MockModuleStore) RemoteRemove(repoPath, name string, opts ...git.RemoteRemoveOptions) error {
	return m.remoteRemove(repoPath, name, opts...)
}

func (m *MockModuleStore) RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error) {
	return m.repoTags(repoPath, opts...)
}

func (m *MockModuleStore) PullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error) {
	return m.pullRequestMeta(headPath, basePath, headBranch, baseBranch)
}

func (m *MockModuleStore) ListTagsAfter(repoPath, after string, limit int) (*TagsPage, error) {
	return m.listTagsAfter(repoPath, after, limit)
}

func SetMockModuleStore(t *testing.T, mock ModuleStore) {
	before := Module
	Module = mock
	t.Cleanup(func() {
		Module = before
	})
}
