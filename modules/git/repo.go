// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"path/filepath"
)

// Repository represents a Git repository.
type Repository struct {
	Path string

	commitCache map[sha1]*Commit
	tagCache    map[sha1]*Tag
}

// OpenRepository opens the repository at the given path.
func OpenRepository(repoPath string) (*Repository, error) {
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, err
	}

	return &Repository{Path: repoPath}, nil
}
