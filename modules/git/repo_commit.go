// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"container/list"
	"errors"
	"strings"
	"sync"

	"github.com/Unknwon/com"
)

func (repo *Repository) getCommitIdOfRef(refpath string) (string, error) {
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "show-ref", "--verify", refpath)
	if err != nil {
		return "", errors.New(stderr)
	}
	return strings.Split(stdout, " ")[0], nil
}

func (repo *Repository) GetCommitIdOfBranch(branchName string) (string, error) {
	return repo.getCommitIdOfRef("refs/heads/" + branchName)
}

// get branch's last commit or a special commit by id string
func (repo *Repository) GetCommitOfBranch(branchName string) (*Commit, error) {
	commitId, err := repo.GetCommitIdOfBranch(branchName)
	if err != nil {
		return nil, err
	}
	return repo.GetCommit(commitId)
}

func (repo *Repository) GetCommitIdOfTag(tagName string) (string, error) {
	return repo.getCommitIdOfRef("refs/tags/" + tagName)
}

func (repo *Repository) GetCommitOfTag(tagName string) (*Commit, error) {
	tag, err := repo.GetTag(tagName)
	if err != nil {
		return nil, err
	}
	return tag.Commit()
}

// Parse commit information from the (uncompressed) raw
// data from the commit object.
// \n\n separate headers from message
func parseCommitData(data []byte) (*Commit, error) {
	commit := new(Commit)
	commit.parents = make([]sha1, 0, 1)
	// we now have the contents of the commit object. Let's investigate...
	nextline := 0
l:
	for {
		eol := bytes.IndexByte(data[nextline:], '\n')
		switch {
		case eol > 0:
			line := data[nextline : nextline+eol]
			spacepos := bytes.IndexByte(line, ' ')
			reftype := line[:spacepos]
			switch string(reftype) {
			case "tree":
				id, err := NewIdFromString(string(line[spacepos+1:]))
				if err != nil {
					return nil, err
				}
				commit.Tree.Id = id
			case "parent":
				// A commit can have one or more parents
				oid, err := NewIdFromString(string(line[spacepos+1:]))
				if err != nil {
					return nil, err
				}
				commit.parents = append(commit.parents, oid)
			case "author":
				sig, err := newSignatureFromCommitline(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Author = sig
			case "committer":
				sig, err := newSignatureFromCommitline(line[spacepos+1:])
				if err != nil {
					return nil, err
				}
				commit.Committer = sig
			}
			nextline += eol + 1
		case eol == 0:
			commit.CommitMessage = string(data[nextline+1:])
			break l
		default:
			break l
		}
	}
	return commit, nil
}

func (repo *Repository) getCommit(id sha1) (*Commit, error) {
	if repo.commitCache != nil {
		if c, ok := repo.commitCache[id]; ok {
			return c, nil
		}
	} else {
		repo.commitCache = make(map[sha1]*Commit, 10)
	}

	data, bytErr, err := com.ExecCmdDirBytes(repo.Path, "git", "cat-file", "-p", id.String())
	if err != nil {
		return nil, errors.New(err.Error() + ": " + string(bytErr))
	}

	commit, err := parseCommitData(data)
	if err != nil {
		return nil, err
	}
	commit.repo = repo
	commit.Id = id

	repo.commitCache[id] = commit
	return commit, nil
}

// Find the commit object in the repository.
func (repo *Repository) GetCommit(commitId string) (*Commit, error) {
	id, err := NewIdFromString(commitId)
	if err != nil {
		return nil, err
	}

	return repo.getCommit(id)
}

func (repo *Repository) commitsCount(id sha1) (int, error) {
	if gitVer.LessThan(MustParseVersion("1.8.0")) {
		stdout, stderr, err := com.ExecCmdDirBytes(repo.Path, "git", "log", "--pretty=format:''", id.String())
		if err != nil {
			return 0, errors.New(string(stderr))
		}
		return len(bytes.Split(stdout, []byte("\n"))), nil
	}

	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "rev-list", "--count", id.String())
	if err != nil {
		return 0, errors.New(stderr)
	}
	return com.StrTo(strings.TrimSpace(stdout)).Int()
}

// used only for single tree, (]
func (repo *Repository) CommitsBetween(last *Commit, before *Commit) (*list.List, error) {
	l := list.New()
	if last == nil || last.ParentCount() == 0 {
		return l, nil
	}

	var err error
	cur := last
	for {
		if cur.Id.Equal(before.Id) {
			break
		}
		l.PushBack(cur)
		if cur.ParentCount() == 0 {
			break
		}
		cur, err = cur.Parent(0)
		if err != nil {
			return nil, err
		}
	}
	return l, nil
}

func (repo *Repository) commitsBefore(lock *sync.Mutex, l *list.List, parent *list.Element, id sha1, limit int) error {
	commit, err := repo.getCommit(id)
	if err != nil {
		return err
	}

	var e *list.Element
	if parent == nil {
		e = l.PushBack(commit)
	} else {
		var in = parent
		for {
			if in == nil {
				break
			} else if in.Value.(*Commit).Id.Equal(commit.Id) {
				return nil
			} else {
				if in.Next() == nil {
					break
				}
				if in.Value.(*Commit).Committer.When.Equal(commit.Committer.When) {
					break
				}

				if in.Value.(*Commit).Committer.When.After(commit.Committer.When) &&
					in.Next().Value.(*Commit).Committer.When.Before(commit.Committer.When) {
					break
				}
			}
			in = in.Next()
		}

		e = l.InsertAfter(commit, in)
	}

	var pr = parent
	if commit.ParentCount() > 1 {
		pr = e
	}

	for i := 0; i < commit.ParentCount(); i++ {
		id, err := commit.ParentId(i)
		if err != nil {
			return err
		}
		err = repo.commitsBefore(lock, l, pr, id, 0)
		if err != nil {
			return err
		}
	}

	return nil
}

func (repo *Repository) CommitsCount(commitId string) (int, error) {
	id, err := NewIdFromString(commitId)
	if err != nil {
		return 0, err
	}
	return repo.commitsCount(id)
}

func (repo *Repository) FileCommitsCount(branch, file string) (int, error) {
	stdout, stderr, err := com.ExecCmdDir(repo.Path, "git", "rev-list", "--count",
		branch, "--", file)
	if err != nil {
		return 0, errors.New(stderr)
	}
	return com.StrTo(strings.TrimSpace(stdout)).Int()
}

func (repo *Repository) CommitsByFileAndRange(branch, file string, page int) (*list.List, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(repo.Path, "git", "log", branch,
		"--skip="+com.ToStr((page-1)*50), "--max-count=50", prettyLogFormat, "--", file)
	if err != nil {
		return nil, errors.New(string(stderr))
	}
	return parsePrettyFormatLog(repo, stdout)
}

func (repo *Repository) getCommitsBefore(id sha1) (*list.List, error) {
	l := list.New()
	lock := new(sync.Mutex)
	err := repo.commitsBefore(lock, l, nil, id, 0)
	return l, err
}

func (repo *Repository) searchCommits(id sha1, keyword string) (*list.List, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(repo.Path, "git", "log", id.String(), "-100",
		"-i", "--grep="+keyword, prettyLogFormat)
	if err != nil {
		return nil, err
	} else if len(stderr) > 0 {
		return nil, errors.New(string(stderr))
	}
	return parsePrettyFormatLog(repo, stdout)
}

func (repo *Repository) commitsByRange(id sha1, page int) (*list.List, error) {
	stdout, stderr, err := com.ExecCmdDirBytes(repo.Path, "git", "log", id.String(),
		"--skip="+com.ToStr((page-1)*50), "--max-count=50", prettyLogFormat)
	if err != nil {
		return nil, errors.New(string(stderr))
	}
	return parsePrettyFormatLog(repo, stdout)
}

func (repo *Repository) getCommitOfRelPath(id sha1, relPath string) (*Commit, error) {
	stdout, _, err := com.ExecCmdDir(repo.Path, "git", "log", "-1", prettyLogFormat, id.String(), "--", relPath)
	if err != nil {
		return nil, err
	}

	id, err = NewIdFromString(string(stdout))
	if err != nil {
		return nil, err
	}

	return repo.getCommit(id)
}
