// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"path"
	"time"

	git "github.com/gogits/git"
)

type RepoFile struct {
	*git.TreeEntry
	Path    string
	Message string
	Created time.Time
	Size    int64
}

func GetReposFiles(userName, reposName, branchName, rpath string) ([]*RepoFile, error) {
	f := RepoPath(userName, reposName)

	repo, err := git.OpenRepository(f)
	if err != nil {
		return nil, err
	}

	ref, err := repo.LookupReference("refs/heads/" + branchName)
	if err != nil {
		return nil, err
	}

	lastCommit, err := repo.LookupCommit(ref.Oid)
	if err != nil {
		return nil, err
	}

	var repodirs []*RepoFile
	var repofiles []*RepoFile
	lastCommit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			size, err := repo.ObjectSize(entry.Id)
			if err != nil {
				return 0
			}
			switch entry.Filemode {
			case git.FileModeBlob, git.FileModeBlobExec:
				repofiles = append(repofiles, &RepoFile{
					entry,
					path.Join(dirname, entry.Name),
					lastCommit.Message(),
					lastCommit.Committer.When,
					size,
				})
			case git.FileModeTree:
				repodirs = append(repodirs, &RepoFile{
					entry,
					path.Join(dirname, entry.Name),
					lastCommit.Message(),
					lastCommit.Committer.When,
					size,
				})
			}
		}
		return 0
	})

	return append(repodirs, repofiles...), nil
}
