// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gogs/git-module"
	"github.com/pkg/errors"
	log "unknwon.dev/clog/v2"
)

// PullRequestMeta contains metadata for a pull request.
type PullRequestMeta struct {
	// The merge base of the pull request.
	MergeBase string
	// The commits that are requested to be merged.
	Commits []*git.Commit
	// The number of files changed.
	NumFiles int
}

func (moduler) PullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error) {
	tmpRemoteBranch := baseBranch

	// We need to create a temporary remote when the pull request is sent from a forked repository.
	if headPath != basePath {
		tmpRemote := strconv.FormatInt(time.Now().UnixNano(), 10)
		err := Module.RepoAddRemote(headPath, tmpRemote, basePath, git.AddRemoteOptions{Fetch: true})
		if err != nil {
			return nil, fmt.Errorf("add remote: %v", err)
		}
		defer func() {
			err := Module.RepoRemoveRemote(headPath, tmpRemote)
			if err != nil {
				log.Error("Failed to remove remote %q [path: %s]: %v", tmpRemote, headPath, err)
				return
			}
		}()

		tmpRemoteBranch = "remotes/" + tmpRemote + "/" + baseBranch
	}

	mergeBase, err := Module.RepoMergeBase(headPath, tmpRemoteBranch, headBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get merge base")
	}

	commits, err := Module.RepoLog(headPath, mergeBase+"..."+headBranch)
	if err != nil {
		return nil, errors.Wrap(err, "get commits")
	}

	// Count number of changed files
	names, err := Module.RepoDiffNameOnly(headPath, tmpRemoteBranch, headBranch, git.DiffNameOnlyOptions{NeedsMergeBase: true})
	if err != nil {
		return nil, errors.Wrap(err, "get changed files")
	}

	return &PullRequestMeta{
		MergeBase: mergeBase,
		Commits:   commits,
		NumFiles:  len(names),
	}, nil
}
