// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gogs/git-module"
	log "unknwon.dev/clog/v2"
)

type PullRequestMeta struct {
	MergeBase string
	Commits   []*git.Commit
	NumFiles  int
}

func GetPullRequestMeta(headPath, basePath, headBranch, baseBranch string) (*PullRequestMeta, error) {
	var remoteBranch string

	// We don't need a temporary remote for same repository.
	if headPath != basePath {
		// Add a temporary remote
		tmpRemote := strconv.FormatInt(time.Now().UnixNano(), 10)
		err := git.RepoAddRemote(headPath, tmpRemote, basePath, git.AddRemoteOptions{Fetch: true})
		if err != nil {
			return nil, fmt.Errorf("AddRemote: %v", err)
		}
		defer func() {
			err := git.RepoRemoveRemote(headPath, tmpRemote)
			if err != nil {
				log.Error("Failed to remove remote %q [path: %s]: %v", tmpRemote, headPath, err)
				return
			}
		}()

		remoteBranch = "remotes/" + tmpRemote + "/" + baseBranch
	} else {
		remoteBranch = baseBranch
	}

	var err error
	prMeta := new(PullRequestMeta)
	prMeta.MergeBase, err = git.RepoMergeBase(headPath, remoteBranch, headBranch)
	if err != nil {
		return nil, err
	}

	prMeta.Commits, err = git.RepoLog(headPath, prMeta.MergeBase+"..."+headBranch)
	if err != nil {
		return nil, err
	}

	// Count number of changed files.
	names, err := git.RepoDiffNameOnly(headPath, remoteBranch, headBranch, git.DiffNameOnlyOptions{NeedsMergeBase: true})
	if err != nil {
		return nil, err
	}
	prMeta.NumFiles = len(names)

	return prMeta, nil
}
