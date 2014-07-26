// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"

	"github.com/Unknwon/com"
)

// Find the tree object in the repository.
func (repo *Repository) GetTree(idStr string) (*Tree, error) {
	id, err := NewIdFromString(idStr)
	if err != nil {
		return nil, err
	}
	return repo.getTree(id)
}

func (repo *Repository) getTree(id sha1) (*Tree, error) {
	treePath := filepathFromSHA1(repo.Path, id.String())
	if !com.IsFile(treePath) {
		_, _, err := com.ExecCmdDir(repo.Path, "git", "ls-tree", id.String())
		if err != nil {
			return nil, fmt.Errorf("repo.getTree: %v", ErrNotExist)
		}
	}

	return NewTree(repo, id), nil
}
