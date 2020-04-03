// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

// Storage is the storage type of an LFS object.
type Storage string

const (
	StorageLocal Storage = "local"
)
