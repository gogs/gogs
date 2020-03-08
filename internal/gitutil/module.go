// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"
)

// Moduler is the interface for Git operations.
//
// NOTE: All methods are sorted in alphabetically.
type Moduler interface {
	// AddRemote adds a new remote to the repository in given path.
	RepoAddRemote(repoPath, name, url string, opts ...git.AddRemoteOptions) error
	// RepoDiffNameOnly returns a list of changed files between base and head revisions
	// of the repository in given path.
	RepoDiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	// RepoLog returns a list of commits in the state of given revision of the repository
	// in given path. The returned list is in reverse chronological order.
	RepoLog(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	// RepoMergeBase returns merge base between base and head revisions of the repository
	// in given path.
	RepoMergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	// RepoRemoveRemote removes a remote from the repository in given path.
	RepoRemoveRemote(repoPath, name string, opts ...git.RemoveRemoteOptions) error
	// RepoTags returns a list of tags of the repository in given path.
	RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error)

	Utiler
}

// Utiler is the interface for utility helpers implemented in this package.
//
// NOTE: All methods are sorted in alphabetically.
type Utiler interface {
	// GetPullRequestMeta gathers pull request metadata based on given head and base information.
	PullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	// ListTagsAfter returns a list of tags "after" (exlusive) given tag.
	ListTagsAfter(repoPath, after string, limit int) (*TagsPage, error)
}

// moduler is holds real implementation.
type moduler struct{}

func (moduler) RepoAddRemote(repoPath, name, url string, opts ...git.AddRemoteOptions) error {
	if MockModule.RepoAddRemote != nil {
		return MockModule.RepoAddRemote(repoPath, name, url, opts...)
	}
	return git.RepoAddRemote(repoPath, name, url, opts...)
}

func (moduler) RepoDiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	if MockModule.RepoDiffNameOnly != nil {
		return MockModule.RepoDiffNameOnly(repoPath, base, head, opts...)
	}
	return git.RepoDiffNameOnly(repoPath, base, head, opts...)
}

func (moduler) RepoLog(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	if MockModule.RepoLog != nil {
		return MockModule.RepoLog(repoPath, rev, opts...)
	}
	return git.RepoLog(repoPath, rev, opts...)
}

func (moduler) RepoMergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	if MockModule.RepoMergeBase != nil {
		return MockModule.RepoMergeBase(repoPath, base, head, opts...)
	}
	return git.RepoMergeBase(repoPath, base, head, opts...)
}

func (moduler) RepoRemoveRemote(repoPath, name string, opts ...git.RemoveRemoteOptions) error {
	if MockModule.RepoRemoveRemote != nil {
		return MockModule.RepoRemoveRemote(repoPath, name, opts...)
	}
	return git.RepoRemoveRemote(repoPath, name, opts...)
}

func (moduler) RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error) {
	if MockModule.RepoTags != nil {
		return MockModule.RepoTags(repoPath, opts...)
	}
	return git.RepoTags(repoPath, opts...)
}

// Module is a mockable interface for Git operations.
var Module Moduler = moduler{}
