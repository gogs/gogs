// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"
	"testing"

	"github.com/gogs/git-module"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestModuler_PullRequestMeta(t *testing.T) {
	headPath := "/head/path"
	basePath := "/base/path"
	headBranch := "head_branch"
	baseBranch := "base_branch"
	mergeBase := "MERGE-BASE"
	changedFiles := []string{"a.go", "b.txt"}
	commits := []*git.Commit{
		{ID: git.MustIDFromString("adfd6da3c0a3fb038393144becbf37f14f780087")},
	}

	mockModule := &MockModuleStore{
		repoAddRemote: func(repoPath, name, url string, opts ...git.AddRemoteOptions) error {
			if repoPath != headPath {
				return fmt.Errorf("repoPath: want %q but got %q", headPath, repoPath)
			} else if name == "" {
				return errors.New("empty name")
			} else if url != basePath {
				return fmt.Errorf("url: want %q but got %q", basePath, url)
			}

			if len(opts) == 0 {
				return errors.New("no options")
			} else if !opts[0].Fetch {
				return fmt.Errorf("opts.Fetch: want %v but got %v", true, opts[0].Fetch)
			}

			return nil
		},
		repoMergeBase: func(repoPath, base, head string, opts ...git.MergeBaseOptions) (string, error) {
			if repoPath != headPath {
				return "", fmt.Errorf("repoPath: want %q but got %q", headPath, repoPath)
			} else if base == "" {
				return "", errors.New("empty base")
			} else if head != headBranch {
				return "", fmt.Errorf("head: want %q but got %q", headBranch, head)
			}

			return mergeBase, nil
		},
		repoLog: func(repoPath, rev string, opts ...git.LogOptions) ([]*git.Commit, error) {
			if repoPath != headPath {
				return nil, fmt.Errorf("repoPath: want %q but got %q", headPath, repoPath)
			}

			expRev := mergeBase + "..." + headBranch
			if rev != expRev {
				return nil, fmt.Errorf("rev: want %q but got %q", expRev, rev)
			}

			return commits, nil
		},
		repoDiffNameOnly: func(repoPath, base, head string, opts ...git.DiffNameOnlyOptions) ([]string, error) {
			if repoPath != headPath {
				return nil, fmt.Errorf("repoPath: want %q but got %q", headPath, repoPath)
			} else if base == "" {
				return nil, errors.New("empty base")
			} else if head != headBranch {
				return nil, fmt.Errorf("head: want %q but got %q", headBranch, head)
			}

			if len(opts) == 0 {
				return nil, errors.New("no options")
			} else if !opts[0].NeedsMergeBase {
				return nil, fmt.Errorf("opts.NeedsMergeBase: want %v but got %v", true, opts[0].NeedsMergeBase)
			}

			return changedFiles, nil
		},
		repoRemoveRemote: func(repoPath, name string, opts ...git.RemoveRemoteOptions) error {
			if repoPath != headPath {
				return fmt.Errorf("repoPath: want %q but got %q", headPath, repoPath)
			} else if name == "" {
				return errors.New("empty name")
			}

			return nil
		},

		pullRequestMeta: Module.PullRequestMeta,
	}
	beforeModule := Module
	Module = mockModule
	t.Cleanup(func() {
		Module = beforeModule
	})

	meta, err := Module.PullRequestMeta(headPath, basePath, headBranch, baseBranch)
	if err != nil {
		t.Fatal(err)
	}

	expMeta := &PullRequestMeta{
		MergeBase: mergeBase,
		Commits:   commits,
		NumFiles:  2,
	}
	assert.Equal(t, expMeta, meta)
}
