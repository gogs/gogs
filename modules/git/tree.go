// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"errors"
	"strings"

	"github.com/Unknwon/com"
)

var (
	ErrNotExist = errors.New("error not exist")
)

// A tree is a flat directory listing.
type Tree struct {
	Id   sha1
	repo *Repository

	// parent tree
	ptree *Tree

	entries       Entries
	entriesParsed bool
}

// Parse tree information from the (uncompressed) raw
// data from the tree object.
func parseTreeData(tree *Tree, data []byte) ([]*TreeEntry, error) {
	entries := make([]*TreeEntry, 0, 10)
	l := len(data)
	pos := 0
	for pos < l {
		entry := new(TreeEntry)
		entry.ptree = tree
		step := 6
		switch string(data[pos : pos+step]) {
		case "100644":
			entry.mode = ModeBlob
			entry.Type = BLOB
		case "100755":
			entry.mode = ModeExec
			entry.Type = BLOB
		case "120000":
			entry.mode = ModeSymlink
			entry.Type = BLOB
		case "160000":
			entry.mode = ModeCommit
			entry.Type = COMMIT

			step = 8
		case "040000":
			entry.mode = ModeTree
			entry.Type = TREE
		default:
			return nil, errors.New("unknown type: " + string(data[pos:pos+step]))
		}
		pos += step + 6 // Skip string type of entry type.

		step = 40
		id, err := NewIdFromString(string(data[pos : pos+step]))
		if err != nil {
			return nil, err
		}
		entry.Id = id
		pos += step + 1 // Skip half of sha1.

		step = bytes.IndexByte(data[pos:], '\n')
		entry.name = string(data[pos : pos+step])
		pos += step + 1
		entries = append(entries, entry)
	}
	return entries, nil
}

func (t *Tree) SubTree(rpath string) (*Tree, error) {
	if len(rpath) == 0 {
		return t, nil
	}

	paths := strings.Split(rpath, "/")
	var err error
	var g = t
	var p = t
	var te *TreeEntry
	for _, name := range paths {
		te, err = p.GetTreeEntryByPath(name)
		if err != nil {
			return nil, err
		}

		g, err = t.repo.getTree(te.Id)
		if err != nil {
			return nil, err
		}
		g.ptree = p
		p = g
	}
	return g, nil
}

func (t *Tree) ListEntries(relpath string) (Entries, error) {
	if t.entriesParsed {
		return t.entries, nil
	}
	t.entriesParsed = true

	stdout, stderr, err := com.ExecCmdDirBytes(t.repo.Path,
		"git", "ls-tree", t.Id.String())
	if err != nil {
		if strings.Contains(err.Error(), "exit status 128") {
			return nil, errors.New(strings.TrimSpace(string(stderr)))
		}
		return nil, err
	}
	t.entries, err = parseTreeData(t, stdout)
	return t.entries, err
}

func NewTree(repo *Repository, id sha1) *Tree {
	tree := new(Tree)
	tree.Id = id
	tree.repo = repo
	return tree
}
