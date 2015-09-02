// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"container/list"
	"fmt"
	"strings"
	"time"

	"github.com/Unknwon/com"
)

type PullRequestInfo struct {
	MergeBase string
	Commits   *list.List
	// Diff      *Diff
	NumFiles int
}

// GetPullRequestInfo generates and returns pull request information
// between base and head branches of repositories.
func (repo *Repository) GetPullRequestInfo(basePath, baseBranch, headBranch string) (*PullRequestInfo, error) {
	// Add a temporary remote.
	tmpRemote := com.ToStr(time.Now().UnixNano())
	_, stderr, err := com.ExecCmdDir(repo.Path, "git", "remote", "add", "-f", tmpRemote, basePath)
	if err != nil {
		return nil, fmt.Errorf("add base as remote: %v", concatenateError(err, stderr))
	}
	defer func() {
		com.ExecCmdDir(repo.Path, "git", "remote", "remove", tmpRemote)
	}()

	prInfo := new(PullRequestInfo)

	var stdout string
	remoteBranch := "remotes/" + tmpRemote + "/" + baseBranch
	// Get merge base commit.
	stdout, stderr, err = com.ExecCmdDir(repo.Path, "git", "merge-base", remoteBranch, headBranch)
	if err != nil {
		return nil, fmt.Errorf("get merge base: %v", concatenateError(err, stderr))
	}
	prInfo.MergeBase = strings.TrimSpace(stdout)

	stdout, stderr, err = com.ExecCmdDir(repo.Path, "git", "log", remoteBranch+"..."+headBranch, prettyLogFormat)
	if err != nil {
		return nil, fmt.Errorf("list diff logs: %v", concatenateError(err, stderr))
	}
	prInfo.Commits, err = parsePrettyFormatLog(repo, []byte(stdout))
	if err != nil {
		return nil, fmt.Errorf("parsePrettyFormatLog: %v", err)
	}

	// Count number of changed files.
	stdout, stderr, err = com.ExecCmdDir(repo.Path, "git", "diff", "--name-only", remoteBranch+"..."+headBranch)
	if err != nil {
		return nil, fmt.Errorf("list changed files: %v", concatenateError(err, stderr))
	}
	prInfo.NumFiles = len(strings.Split(stdout, "\n")) - 1

	return prInfo, nil
}

// GetPatch generates and returns patch data between given branches.
func (repo *Repository) GetPatch(basePath, baseBranch, headBranch string) ([]byte, error) {
	// Add a temporary remote.
	tmpRemote := com.ToStr(time.Now().UnixNano())
	_, stderr, err := com.ExecCmdDirBytes(repo.Path, "git", "remote", "add", "-f", tmpRemote, basePath)
	if err != nil {
		return nil, fmt.Errorf("add base as remote: %v", concatenateError(err, string(stderr)))
	}
	defer func() {
		com.ExecCmdDir(repo.Path, "git", "remote", "remove", tmpRemote)
	}()

	var stdout []byte
	remoteBranch := "remotes/" + tmpRemote + "/" + baseBranch
	stdout, stderr, err = com.ExecCmdDirBytes(repo.Path, "git", "diff", "-p", remoteBranch, headBranch)
	if err != nil {
		return nil, concatenateError(err, string(stderr))
	}

	return stdout, nil
}

// Merge merges pull request from head repository and branch.
func (repo *Repository) Merge(headRepoPath string, baseBranch, headBranch string) error {

	return nil
}
