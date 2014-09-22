// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bufio"
	"container/list"
	"strings"
)

// Commit represents a git commit.
type Commit struct {
	Tree
	Id            sha1 // The id of this commit object
	Author        *Signature
	Committer     *Signature
	CommitMessage string

	parents    []sha1 // sha1 strings
	submodules map[string]*SubModule
}

// Return the commit message. Same as retrieving CommitMessage directly.
func (c *Commit) Message() string {
	return c.CommitMessage
}

func (c *Commit) Summary() string {
	return strings.Split(c.CommitMessage, "\n")[0]
}

// Return oid of the parent number n (0-based index). Return nil if no such parent exists.
func (c *Commit) ParentId(n int) (id sha1, err error) {
	if n >= len(c.parents) {
		err = IdNotExist
		return
	}
	return c.parents[n], nil
}

// Return parent number n (0-based index)
func (c *Commit) Parent(n int) (*Commit, error) {
	id, err := c.ParentId(n)
	if err != nil {
		return nil, err
	}
	parent, err := c.repo.getCommit(id)
	if err != nil {
		return nil, err
	}
	return parent, nil
}

// Return the number of parents of the commit. 0 if this is the
// root commit, otherwise 1,2,...
func (c *Commit) ParentCount() int {
	return len(c.parents)
}

func (c *Commit) CommitsBefore() (*list.List, error) {
	return c.repo.getCommitsBefore(c.Id)
}

func (c *Commit) CommitsBeforeUntil(commitId string) (*list.List, error) {
	ec, err := c.repo.GetCommit(commitId)
	if err != nil {
		return nil, err
	}
	return c.repo.CommitsBetween(c, ec)
}

func (c *Commit) CommitsCount() (int, error) {
	return c.repo.commitsCount(c.Id)
}

func (c *Commit) SearchCommits(keyword string) (*list.List, error) {
	return c.repo.searchCommits(c.Id, keyword)
}

func (c *Commit) CommitsByRange(page int) (*list.List, error) {
	return c.repo.commitsByRange(c.Id, page)
}

func (c *Commit) GetCommitOfRelPath(relPath string) (*Commit, error) {
	return c.repo.getCommitOfRelPath(c.Id, relPath)
}

func (c *Commit) GetSubModule(entryname string) (*SubModule, error) {
	moduels, err := c.GetSubModules()
	if err != nil {
		return nil, err
	}
	return moduels[entryname], nil
}

func (c *Commit) GetSubModules() (map[string]*SubModule, error) {
	if c.submodules != nil {
		return c.submodules, nil
	}

	entry, err := c.GetTreeEntryByPath(".gitmodules")
	if err != nil {
		return nil, err
	}
	rd, err := entry.Blob().Data()
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(rd)
	c.submodules = make(map[string]*SubModule)
	var ismodule bool
	var path string
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "[submodule") {
			ismodule = true
			continue
		}
		if ismodule {
			fields := strings.Split(scanner.Text(), "=")
			k := strings.TrimSpace(fields[0])
			if k == "path" {
				path = strings.TrimSpace(fields[1])
			} else if k == "url" {
				c.submodules[path] = &SubModule{path, strings.TrimSpace(fields[1])}
				ismodule = false
			}
		}
	}

	return c.submodules, nil
}
