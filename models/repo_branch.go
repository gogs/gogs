// Copyright 2016 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"

	"github.com/gogits/git-module"
)

type Branch struct {
	Path string
	Name string
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

	branches := make([]*Branch, len(brs))
	for i := range brs {
		branches[i] = &Branch{
			Path: path,
			Name: brs[i],
		}
	}
	return branches, nil
}

func (repo *Repository) GetBranch(br string) (*Branch, error) {
	if !git.IsBranchExist(repo.RepoPath(), br) {
		return nil, ErrBranchNotExist{br}
	}
	return &Branch{
		Path: repo.RepoPath(),
		Name: br,
	}, nil
}

func (repo *Repository) GetBranches() ([]*Branch, error) {
	return GetBranchesByPath(repo.RepoPath())
}

func (br *Branch) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.OpenRepository(br.Path)
	if err != nil {
		return nil, err
	}
	return gitRepo.GetBranchCommit(br.Name)
}

// ProtectBranch contains options of a protected branch.
type ProtectBranch struct {
	ID                 int64
	RepoID             int64  `xorm:"UNIQUE(protect_branch)"`
	Name               string `xorm:"UNIQUE(protect_branch)"`
	Protected          bool
	RequirePullRequest bool
}

// GetProtectBranchOfRepoByName returns *ProtectBranch by branch name in given repostiory.
func GetProtectBranchOfRepoByName(repoID int64, name string) (*ProtectBranch, error) {
	protectBranch := &ProtectBranch{
		RepoID: repoID,
		Name:   name,
	}
	has, err := x.Get(protectBranch)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrBranchNotExist{name}
	}
	return protectBranch, nil
}

// IsBranchOfRepoRequirePullRequest returns true if branch requires pull request in given repository.
func IsBranchOfRepoRequirePullRequest(repoID int64, name string) bool {
	protectBranch, err := GetProtectBranchOfRepoByName(repoID, name)
	if err != nil {
		return false
	}
	return protectBranch.Protected && protectBranch.RequirePullRequest
}

// UpdateProtectBranch saves branch protection options.
// If ID is 0, it creates a new record. Otherwise, updates existing record.
func UpdateProtectBranch(protectBranch *ProtectBranch) (err error) {
	if protectBranch.ID == 0 {
		if _, err = x.Insert(protectBranch); err != nil {
			return fmt.Errorf("Insert: %v", err)
		}
		return
	}

	_, err = x.Id(protectBranch.ID).AllCols().Update(protectBranch)
	return err
}

// GetProtectBranchesByRepoID returns a list of *ProtectBranch in given repostiory.
func GetProtectBranchesByRepoID(repoID int64) ([]*ProtectBranch, error) {
	protectBranches := make([]*ProtectBranch, 0, 2)
	return protectBranches, x.Where("repo_id = ?", repoID).Asc("name").Find(&protectBranches)
}
