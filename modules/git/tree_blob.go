// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"
	"path"
	"strings"
)

func (t *Tree) GetTreeEntryByPath(relpath string) (*TreeEntry, error) {
	if len(relpath) == 0 {
		return &TreeEntry{
			Id:   t.Id,
			Type: TREE,
			mode: ModeTree,
		}, nil
		// return nil, fmt.Errorf("GetTreeEntryByPath(empty relpath): %v", ErrNotExist)
	}

	relpath = path.Clean(relpath)
	parts := strings.Split(relpath, "/")
	var err error
	tree := t
	for i, name := range parts {
		if i == len(parts)-1 {
			entries, err := tree.ListEntries(path.Dir(relpath))
			if err != nil {
				return nil, err
			}
			for _, v := range entries {
				if v.name == name {
					return v, nil
				}
			}
		} else {
			tree, err = tree.SubTree(name)
			if err != nil {
				return nil, err
			}
		}
	}
	return nil, fmt.Errorf("GetTreeEntryByPath: %v", ErrNotExist)
}

func (t *Tree) GetBlobByPath(rpath string) (*Blob, error) {
	entry, err := t.GetTreeEntryByPath(rpath)
	if err != nil {
		return nil, err
	}

	if !entry.IsDir() {
		return entry.Blob(), nil
	}

	return nil, ErrNotExist
}
