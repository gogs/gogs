// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"container/list"
	"fmt"
	"path"
	"strings"

	"github.com/Unknwon/com"

	"github.com/gogits/git"
)

// RepoFile represents a file object in git repository.
type RepoFile struct {
	*git.TreeEntry
	Path   string
	Size   int64
	Repo   *git.Repository
	Commit *git.Commit
}

// LookupBlob returns the content of an object.
func (file *RepoFile) LookupBlob() (*git.Blob, error) {
	if file.Repo == nil {
		return nil, ErrRepoFileNotLoaded
	}

	return file.Repo.LookupBlob(file.Id)
}

// GetBranches returns all branches of given repository.
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

func GetTargetFile(userName, reposName, branchName, commitId, rpath string) (*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(branchName, commitId)
	if err != nil {
		return nil, err
	}

	parts := strings.Split(path.Clean(rpath), "/")

	var entry *git.TreeEntry
	tree := commit.Tree
	for i, part := range parts {
		if i == len(parts)-1 {
			entry = tree.EntryByName(part)
			if entry == nil {
				return nil, ErrRepoFileNotExist
			}
		} else {
			tree, err = repo.SubTree(tree, part)
			if err != nil {
				return nil, err
			}
		}
	}

	size, err := repo.ObjectSize(entry.Id)
	if err != nil {
		return nil, err
	}

	repoFile := &RepoFile{
		entry,
		rpath,
		size,
		repo,
		commit,
	}

	return repoFile, nil
}

// GetReposFiles returns a list of file object in given directory of repository.
func GetReposFiles(userName, reposName, branchName, commitId, rpath string) ([]*RepoFile, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}

	commit, err := repo.GetCommit(branchName, commitId)
	if err != nil {
		return nil, err
	}

	var repodirs []*RepoFile
	var repofiles []*RepoFile
	commit.Tree.Walk(func(dirname string, entry *git.TreeEntry) int {
		if dirname == rpath {
			// TODO: size get method shoule be improved
			size, err := repo.ObjectSize(entry.Id)
			if err != nil {
				return 0
			}

			var cm = commit
			var i int
			for {
				i = i + 1
				//fmt.Println(".....", i, cm.Id(), cm.ParentCount())
				if cm.ParentCount() == 0 {
					break
				} else if cm.ParentCount() == 1 {
					pt, _ := repo.SubTree(cm.Parent(0).Tree, dirname)
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
					var lastSameCm *git.Commit
					//fmt.Println(".....", cm.ParentCount())
					for i := 0; i < cm.ParentCount(); i++ {
						//fmt.Println("parent", i, cm.Parent(i).Id())
						p := cm.Parent(i)
						pt, _ := repo.SubTree(p.Tree, dirname)
						var pEntry *git.TreeEntry
						if pt != nil {
							pEntry = pt.EntryByName(entry.Name)
						}

						//fmt.Println("pEntry", pEntry)

						if pEntry == nil {
							emptyCnt = emptyCnt + 1
							if emptyCnt+sameIdcnt == cm.ParentCount() {
								if lastSameCm == nil {
									goto loop
								} else {
									cm = lastSameCm
									break
								}
							}
						} else {
							//fmt.Println(i, "pEntry", pEntry.Id, "entry", entry.Id)
							if !pEntry.Id.Equal(entry.Id) {
								goto loop
							} else {
								lastSameCm = cm.Parent(i)
								sameIdcnt = sameIdcnt + 1
								if emptyCnt+sameIdcnt == cm.ParentCount() {
									// TODO: now follow the first parent commit?
									cm = lastSameCm
									//fmt.Println("sameId...")
									break
								}
							}
						}
					}
				}
			}

		loop:

			rp := &RepoFile{
				entry,
				path.Join(dirname, entry.Name),
				size,
				repo,
				cm,
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

func GetCommit(userName, repoName, branchname, commitid string) (*git.Commit, error) {
	repo, err := git.OpenRepository(RepoPath(userName, repoName))
	if err != nil {
		return nil, err
	}

	return repo.GetCommit(branchname, commitid)
}

// GetCommits returns all commits of given branch of repository.
func GetCommits(userName, reposName, branchname string) (*list.List, error) {
	repo, err := git.OpenRepository(RepoPath(userName, reposName))
	if err != nil {
		return nil, err
	}
	r, err := repo.LookupReference(fmt.Sprintf("refs/heads/%s", branchname))
	if err != nil {
		return nil, err
	}
	return r.AllCommits()
}

type DiffFile struct {
	Name               string
	Addition, Deletion int
	Type               string
	Content            []string
}

type Diff struct {
	NumFiles                     int // Number of file has been changed.
	TotalAddition, TotalDeletion int
	Files                        []*DiffFile
}

func GetDiff(repoPath, commitid string) (*Diff, error) {
	stdout, _, err := com.ExecCmdDir(repoPath, "git", "show", commitid)
	if err != nil {
		return nil, err
	}

	// Sperate parts by file.
	startIndex := strings.Index(stdout, "diff --git ") + 12

	// First part is commit information.
	// Check if it's a merge.
	mergeIndex := strings.Index(stdout[:startIndex], "merge")
	if mergeIndex > -1 {
		mergeCommit := strings.SplitN(strings.Split(stdout[:startIndex], "\n")[1], "", 3)[2]
		return GetDiff(repoPath, mergeCommit)
	}

	parts := strings.Split(stdout[startIndex:], "diff --git ")
	diff := &Diff{NumFiles: len(parts)}
	diff.Files = make([]*DiffFile, 0, diff.NumFiles)
	for _, part := range parts {
		infos := strings.SplitN(part, "\n", 6)
		maxIndex := len(infos) - 1
		infos[maxIndex] = strings.TrimSuffix(strings.TrimSuffix(infos[maxIndex], "\n"), "\n\\ No newline at end of file")

		file := &DiffFile{
			Name:    strings.TrimPrefix(strings.Split(infos[0], " ")[0], "a/"),
			Content: strings.Split(infos[maxIndex], "\n"),
		}
		diff.Files = append(diff.Files, file)
	}
	return diff, nil
}
