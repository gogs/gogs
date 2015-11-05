// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"
	"path/filepath"
)

// idxFile represents a pack index file.
type idxFile struct {
	indexpath    string
	packpath     string
	packversion  uint32
	offsetValues map[sha1]uint64
}

// Repository represents a Git repository with cached information.
type Repository struct {
	Path       string
	indexfiles map[string]*idxFile

	commitCache map[sha1]*Commit
	tagCache    map[sha1]*Tag
}

// OpenRepository opens the repository at the given path.
func OpenRepository(repoPath string) (*Repository, error) {
	repoPath, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("get abs path: %v", err)
	} else if !isDir(repoPath) {
		return nil, fmt.Errorf("path does not exist or is not a directory: %s", repoPath)
	}

	return &Repository{Path: repoPath}, nil
}

// buildIndexFiles finds and builds index file map for the repository.
// It will not rebuild the map if it has been built.
func (repo *Repository) buildIndexFiles() error {
	if repo.indexfiles != nil {
		return nil
	}

	filenames, err := filepath.Glob(filepath.Join(repo.Path, "objects/pack/*idx"))
	if err != nil {
		return fmt.Errorf("search glob: %v", err)
	}

	repo.indexfiles = make(map[string]*idxFile, len(filenames))
	for i := range filenames {
		idx, err := readIdxFile(filenames[i])
		if err != nil {
			return fmt.Errorf("readIdxFile: %v", err)
		}
		repo.indexfiles[filenames[i]] = idx
	}

	return nil
}
