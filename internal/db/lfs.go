// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"time"

	"github.com/jinzhu/gorm"

	"gogs.io/gogs/internal/lfsutil"
)

// LFSStore is the database interface for LFS data.
//
// NOTE: All methods are sorted in alphabetically.
type LFSStore interface {
}

var LFS LFSStore

type lfs struct {
	*gorm.DB
}

// LFSObject is the relation between an LFS object and a repository.
type LFSObject struct {
	RepoID    int64           `gorm:"PRIMARY_KEY;AUTO_INCREMENT:false"`
	OID       string          `gorm:"PRIMARY_KEY;COLUMN:oid"`
	Size      int64           `gorm:"NOT NULL"`
	Verified  bool            `gorm:"NOT NULL"`
	Storage   lfsutil.Storage `gorm:"NOT NULL"`
	CreatedAt time.Time       `gorm:"NOT NULL"`
}
