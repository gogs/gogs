// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"
)

type MockModuleStore struct {
	RepoAddRemote    func(repoPath, name, url string, opts ...git.AddRemoteOptions) error
	RepoDiffNameOnly func(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error)
	RepoLog          func(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error)
	RepoMergeBase    func(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error)
	RepoRemoveRemote func(repoPath, name string, opts ...git.RemoveRemoteOptions) error
	RepoTags         func(repoPath string, opts ...git.TagsOptions) ([]string, error)
}

// MockModule holds mock implementation of each method for Modulers interface.
// When the field is non-nil, it is considered mocked. Otherwise, the real
// implementation will be executed.
var MockModule MockModuleStore
