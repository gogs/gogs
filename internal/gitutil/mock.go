// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"
)

var _ ModuleStore = (*MockModuleStore)(nil)

// MockModuleStore is a mock implementation of ModuleStore interface.
type MockModuleStore struct {
	repoAddRemote    func(repoPath, name, url string, opts ...git.AddRemoteOptions) error
	repoDiffNameOnly func(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	repoLog          func(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	repoMergeBase    func(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	repoRemoveRemote func(repoPath, name string, opts ...git.RemoveRemoteOptions) error
	repoTags         func(repoPath string, opts ...git.TagsOptions) ([]string, error)

	pullRequestMeta func(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	listTagsAfter   func(repoPath, after string, limit int) (*TagsPage, error)
}

func (m *MockModuleStore) RepoAddRemote(repoPath, name, url string, opts ...git.AddRemoteOptions) error {
	return m.repoAddRemote(repoPath, name, url, opts...)
}

func (m *MockModuleStore) RepoDiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	return m.repoDiffNameOnly(repoPath, base, head, opts...)
}

func (m *MockModuleStore) RepoLog(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	return m.repoLog(repoPath, rev, opts...)
}

func (m *MockModuleStore) RepoMergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	return m.repoMergeBase(repoPath, base, head, opts...)
}

func (m *MockModuleStore) RepoRemoveRemote(repoPath, name string, opts ...git.RemoveRemoteOptions) error {
	return m.repoRemoveRemote(repoPath, name, opts...)
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
