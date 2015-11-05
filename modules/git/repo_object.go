// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"fmt"
	"io"
)

// ObjectType represents type of the Git object.
type ObjectType int

const (
	OBJECT_COMMIT ObjectType = 0x10
	OBJECT_TREE   ObjectType = 0x20
	OBJECT_BLOB   ObjectType = 0x30
	OBJECT_TAG    ObjectType = 0x40
)

func (t ObjectType) String() string {
	switch t {
	case OBJECT_COMMIT:
		return string(OBJETC_TYPE_NAME_COMMIT)
	case OBJECT_TREE:
		return string(OBJETC_TYPE_NAME_TREE)
	case OBJECT_BLOB:
		return string(OBJETC_TYPE_NAME_BLOB)
	case OBJECT_TAG:
		return string(OBJETC_TYPE_NAME_TAG)
	default:
		return "undefined"
	}
}

// ObjectTypeName represents name of the Git object's type.
type ObjectTypeName string

const (
	OBJETC_TYPE_NAME_COMMIT ObjectTypeName = "commit"
	OBJETC_TYPE_NAME_TREE   ObjectTypeName = "tree"
	OBJETC_TYPE_NAME_BLOB   ObjectTypeName = "blob"
	OBJETC_TYPE_NAME_TAG    ObjectTypeName = "tag"
)

// findObjectPack tries to find packed object by given a SHA1,
// and returns which pack it is in and the offset,
// or return nil if not found.
func (repo *Repository) findObjectPack(id sha1) (*idxFile, uint64, error) {
	if err := repo.buildIndexFiles(); err != nil {
		return nil, 0, fmt.Errorf("buildIndexFiles: %v", err)
	}

	for _, indexfile := range repo.indexfiles {
		if offset, ok := indexfile.offsetValues[id]; ok {
			return indexfile, offset, nil
		}
	}
	return nil, 0, nil
}

// hasObject checks if given object exists and if it's packed.
func (repo *Repository) hasObject(id sha1) (bool, bool, error) {
	sha1 := id.String()
	if isFile(filepathFromSHA1(repo.Path, sha1)) {
		return true, false, nil
	}

	pack, _, err := repo.findObjectPack(id)
	if err != nil {
		return false, false, fmt.Errorf("findObjectPack: %v", err)
	} else if pack == nil {
		return false, false, nil
	}
	return true, true, nil
}

// getRawObject returns object type, size and data reader.
func (repo *Repository) getRawObject(id sha1, metaOnly bool) (ObjectType, int64, io.ReadCloser, error) {
	sha1 := id.String()
	found, packed, err := repo.hasObject(id)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("hasObject: %v", err)
	} else if !found {
		return 0, 0, nil, fmt.Errorf("object not found: %s", sha1)
	}

	if packed {
		pack, offset, err := repo.findObjectPack(id)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("findObjectPack: %v", err)
		}
		return readObjectBytes(pack.packpath, repo.indexfiles, offset, metaOnly)
	}

	return readObjectFile(filepathFromSHA1(repo.Path, sha1), metaOnly)
}
