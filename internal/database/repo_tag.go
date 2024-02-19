// Copyright 2021 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"fmt"

	"github.com/gogs/git-module"
)

type Tag struct {
	RepoPath string
	Name     string

	IsProtected bool
	Commit      *git.Commit
}

func (ta *Tag) GetCommit() (*git.Commit, error) {
	gitRepo, err := git.Open(ta.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("open repository: %v", err)
	}
	return gitRepo.TagCommit(ta.Name)
}

func GetTagsByPath(path string) ([]*Tag, error) {
	gitRepo, err := git.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open repository: %v", err)
	}

	names, err := gitRepo.Tags()
	if err != nil {
		return nil, fmt.Errorf("list tags")
	}

	tags := make([]*Tag, len(names))
	for i := range names {
		tags[i] = &Tag{
			RepoPath: path,
			Name:     names[i],
		}
	}
	return tags, nil
}

func (repo *Repository) GetTags() ([]*Tag, error) {
	return GetTagsByPath(repo.RepoPath())
}
