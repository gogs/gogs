// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"errors"
	"strings"

	"github.com/Unknwon/com"
)

func IsBranchExist(repoPath, branchName string) bool {
	_, _, err := com.ExecCmdDir(repoPath, "git", "show-ref", "--verify", "refs/heads/"+branchName)
	return err == nil
}

func (repo *Repository) IsBranchExist(branchName string) bool {
	return IsBranchExist(repo.Path, branchName)
}

func (repo *Repository) GetBranches() ([]string, error) {
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "show-ref", "--heads")
	if err != nil {
		return nil, errors.New(stderr)
	}
	infos := strings.Split(stdout, "\n")
	branches := make([]string, len(infos)-1)
	for i, info := range infos[:len(infos)-1] {
		parts := strings.Split(info, " ")
		if len(parts) != 2 {
			continue // NOTE: I should believe git will not give me wrong string.
		}
		branches[i] = strings.TrimPrefix(parts[1], "refs/heads/")
	}
	return branches, nil
}
