// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
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

	parents []sha1 // sha1 strings
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

func (c *Commit) GetCommitOfRelPath(relPath string) (*Commit, error) {
	return c.repo.getCommitOfRelPath(c.Id, relPath)
}
