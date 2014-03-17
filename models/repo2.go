// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
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

var (
	ErrRepoFileNotLoaded = fmt.Errorf("repo file not loaded")
)

type RepoFile struct {
	*git.TreeEntry
	Path       string
	Message    string
	Created    time.Time
	Size       int64
	Repo       *git.Repository
	LastCommit string
}

func findTree(repo *git.Repository, tree *git.Tree, rpath string) *git.Tree {
	if rpath == "" {
		return tree
	}
	paths := strings.Split(rpath, "/")
	var g = tree
	for _, p := range paths {
		s := g.EntryByName(p)
		if s == nil {
			return nil
		}
		g, err := repo.LookupTree(s.Id)
		if err != nil {
			return nil
		}
		if g == nil {
			return nil
		}
	}
	return g
}

func (file *RepoFile) LookupBlob() (*git.Blob, error) {
	if file.Repo == nil {
		return nil, ErrRepoFileNotLoaded
	}

	return file.Repo.LookupBlob(file.Id)
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

			var cm = lastCommit

			for {
				if cm.ParentCount() == 0 {
					break
				} else if cm.ParentCount() == 1 {
					pt := findTree(repo, cm.Parent(0).Tree, dirname)
					if pt == nil {
						break
					}
					pEntry := pt.EntryByName(entry.Name)
					if pEntry == nil || !pEntry.Id.Equal(entry.Id) {
						break
					} else {
						cm = cm.Parent(0)
					}
				} else {
					var emptyCnt = 0
					var sameIdcnt = 0
					for i := 0; i < cm.ParentCount(); i++ {
						p := cm.Parent(i)
						pt := findTree(repo, p.Tree, dirname)
						var pEntry *git.TreeEntry
						if pt != nil {
							pEntry = pt.EntryByName(entry.Name)
						}

						if pEntry == nil {
							if emptyCnt == cm.ParentCount()-1 {
								goto loop
							} else {
								emptyCnt = emptyCnt + 1
								continue
							}
						} else {
							if !pEntry.Id.Equal(entry.Id) {
								goto loop
							} else {
								if sameIdcnt == cm.ParentCount()-1 {
									// TODO: now follow the first parent commit?
									cm = cm.Parent(0)
									break
								}
								sameIdcnt = sameIdcnt + 1
							}
						}
					}
				}
			}

		loop:

			rp := &RepoFile{
				entry,
				path.Join(dirname, entry.Name),
				cm.Message(),
				cm.Committer.When,
				size,
				repo,
				cm.Id().String(),
			}

			if entry.IsFile() {
				repofiles = append(repofiles, rp)
			} else if entry.IsDir() {
				repodirs = append(repodirs, rp)
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
