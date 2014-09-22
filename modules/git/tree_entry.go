// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"sort"
	"strings"

	"github.com/Unknwon/com"
)

type EntryMode int

// There are only a few file modes in Git. They look like unix file modes, but they can only be
// one of these.
const (
	ModeBlob    EntryMode = 0100644
	ModeExec    EntryMode = 0100755
	ModeSymlink EntryMode = 0120000
	ModeCommit  EntryMode = 0160000
	ModeTree    EntryMode = 0040000
)

type TreeEntry struct {
	Id   sha1
	Type ObjectType

	mode EntryMode
	name string

	ptree *Tree

	commited bool

	size  int64
	sized bool
}

func (te *TreeEntry) Name() string {
	return te.name
}

func (te *TreeEntry) Size() int64 {
	if te.IsDir() {
		return 0
	}

	if te.sized {
		return te.size
	}

	stdout, _, err := com.ExecCmdDir(te.ptree.repo.Path, "git", "cat-file", "-s", te.Id.String())
	if err != nil {
		return 0
	}

	te.sized = true
	te.size = com.StrTo(strings.TrimSpace(stdout)).MustInt64()
	return te.size
}

func (te *TreeEntry) IsSubModule() bool {
	return te.mode == ModeCommit
}

func (te *TreeEntry) IsDir() bool {
	return te.mode == ModeTree
}

func (te *TreeEntry) EntryMode() EntryMode {
	return te.mode
}

func (te *TreeEntry) Blob() *Blob {
	return &Blob{
		repo:      te.ptree.repo,
		TreeEntry: te,
	}
}

type Entries []*TreeEntry

var sorter = []func(t1, t2 *TreeEntry) bool{
	func(t1, t2 *TreeEntry) bool {
		return (t1.IsDir() || t1.IsSubModule()) && !t2.IsDir() && !t2.IsSubModule()
	},
	func(t1, t2 *TreeEntry) bool {
		return t1.name < t2.name
	},
}

func (bs Entries) Len() int      { return len(bs) }
func (bs Entries) Swap(i, j int) { bs[i], bs[j] = bs[j], bs[i] }
func (bs Entries) Less(i, j int) bool {
	t1, t2 := bs[i], bs[j]
	var k int
	for k = 0; k < len(sorter)-1; k++ {
		sort := sorter[k]
		switch {
		case sort(t1, t2):
			return true
		case sort(t2, t1):
			return false
		}
	}
	return sorter[k](t1, t2)
}

func (bs Entries) Sort() {
	sort.Sort(bs)
}
