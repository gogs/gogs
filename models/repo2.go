// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"path"
	"strings"
	"time"

	git "github.com/gogits/git"
)

type RepoFile struct {
	*git.TreeEntry
	Path       string
	Message    string
	Created    time.Time
	Size       int64
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
