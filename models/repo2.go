// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"path"
	"strings"
	"time"

	"github.com/Unknwon/com"

	"github.com/gogits/git"
)

type Commit struct {
	Author  string
	Email   string
	Date    time.Time
	SHA     string
	Message string
}

type RepoFile struct {
	*git.TreeEntry
	Path    string
	Message string
	Created time.Time
	Size    int64
}

func GetBranches(userName, reposName string) ([]string, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	refs, err := repo.AllReferences()
	if err != nil {
		return nil, err
	}

	brs := make([]string, len(refs))
	for i, ref := range refs {
		brs[i] = ref.Name
	}
	return brs, nil
}

func GetReposFiles(userName, reposName, branchName, rpath string) ([]*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
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

func GetLastestCommit(userName, repoName string) (*Commit, error) {
	stdout, _, err := com.ExecCmd("git", "--git-dir="+RepoPath(userName, repoName), "log", "-1")
	if err != nil {
		return nil, err
	}

	commit := new(Commit)
	for _, line := range strings.Split(stdout, "\n") {
		if len(line) == 0 {
			continue
		}
		switch {
		case line[0] == 'c':
			commit.SHA = line[7:]
		case line[0] == 'A':
			infos := strings.SplitN(line, " ", 3)
			commit.Author = infos[1]
			commit.Email = infos[2][1 : len(infos[2])-1]
		case line[0] == 'D':
			commit.Date, err = time.Parse("Mon Jan 02 15:04:05 2006 -0700", line[8:])
			if err != nil {
				return nil, err
			}
		case line[:4] == "    ":
			commit.Message = line[4:]
		}
	}
	return commit, nil
}
