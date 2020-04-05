// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"path/filepath"
)

// Storage is the storage type of an LFS object.
type Storage string

const (
	StorageLocal Storage = "local"
)

// StorageLocalPath returns computed file path for storing object on local file system.
// It returns empty string if given "oid" isn't valid.
func StorageLocalPath(root string, oid OID) string {
	if !ValidOID(oid) {
		return ""
	}

	return filepath.Join(root, string(oid[0]), string(oid[1]), string(oid))
}
