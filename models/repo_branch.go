// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"github.com/gogits/git-module"
)

type Branch struct {
	Path		string
	Name      	string
}

func GetBranchesByPath(path string) ([]*Branch, error) {
	gitRepo, err := git.OpenRepository(path)
	if err != nil {
		return nil, err
	}

	brs, err := gitRepo.GetBranches()
	if err != nil {
		return nil, err
	}

	Branches := make([]*Branch, len(brs))
	for i := range brs {
		Branches[i] = &Branch{
			Path: path,
			Name: brs[i],
		}
	}
	return Branches, nil
}

func (br *Branch) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.OpenRepository(br.Path)
	if err != nil {
		return nil, err
	}
	return gitRepo.GetBranchCommit(br.Name)
}
