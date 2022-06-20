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
	// RemoteAdd adds a new remote to the repository in given path.
	RemoteAdd(repoPath, name, url string, opts ...git.RemoteAddOptions) error
	// DiffNameOnly returns a list of changed files between base and head revisions
	// of the repository in given path.
	DiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	// Log returns a list of commits in the state of given revision of the
	// repository in given path. The returned list is in reverse chronological
	// order.
	Log(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	// MergeBase returns merge base between base and head revisions of the
	// repository in given path.
	MergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	// RemoteRemove removes a remote from the repository in given path.
	RemoteRemove(repoPath, name string, opts ...git.RemoteRemoveOptions) error
	// RepoTags returns a list of tags of the repository in given path.
	RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error)

	// PullRequestMeta gathers pull request metadata based on given head and base
	// information.
	PullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error)
	// ListTagsAfter returns a list of tags "after" (exclusive) given tag.
	ListTagsAfter(repoPath, after string, limit int) (*TagsPage, error)
}

// module holds the real implementation.
type module struct{}

func (module) RemoteAdd(repoPath, name, url string, opts ...git.RemoteAddOptions) error {
	return git.RemoteAdd(repoPath, name, url, opts...)
}

func (module) DiffNameOnly(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
	return git.DiffNameOnly(repoPath, base, head, opts...)
}

func (module) Log(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
	return git.Log(repoPath, rev, opts...)
}

func (module) MergeBase(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
	return git.MergeBase(repoPath, base, head, opts...)
}

func (module) RemoteRemove(repoPath, name string, opts ...git.RemoteRemoveOptions) error {
	return git.RemoteRemove(repoPath, name, opts...)
}

func (module) RepoTags(repoPath string, opts ...git.TagsOptions) ([]string, error) {
	return git.RepoTags(repoPath, opts...)
}

// Module is a mockable interface for Git operations.
var Module ModuleStore = module{}
