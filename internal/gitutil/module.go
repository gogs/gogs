// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"
)

// ModuleStore is the interface for Git operations.
//
// NOTE: All methods are sorted in alphabetical order.
type ModuleStore interface {
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

	// GetPullRequestMeta gathers pull request metadata based on given head and base information.
	PullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	// ListTagsAfter returns a list of tags "after" (exclusive) given tag.
	ListTagsAfter(repoPath, after string, limit int) (*TagsPage, error)
}

// module holds the real implementation.
type module struct{}

func (module) RepoAddRemote(repoPath, name, url string, opts ...git.AddRemoteOptions) error {
	return git.RepoAddRemote(repoPath, name, url, opts...)
}

func (module) RepoDiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	return git.RepoDiffNameOnly(repoPath, base, head, opts...)
}

func (module) RepoLog(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	return git.RepoLog(repoPath, rev, opts...)
}

func (module) RepoMergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	return git.RepoMergeBase(repoPath, base, head, opts...)
}

func (module) RepoRemoveRemote(repoPath, name string, opts ...git.RemoveRemoteOptions) error {
	return git.RepoRemoveRemote(repoPath, name, opts...)
}

func (module) RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error) {
	return git.RepoTags(repoPath, opts...)
}

// Module is a mockable interface for Git operations.
var Module ModuleStore = module{}
