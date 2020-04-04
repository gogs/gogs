// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"time"

	"github.com/jinzhu/gorm"

	"gogs.io/gogs/internal/lfsutil"
)

// LFSStore is the database interface for LFS objects.
//
// NOTE: All methods are sorted in alphabetical order.
type LFSStore interface {
	// GetObjectsByOIDs returns LFS objects found within "oids".
	// The returned list could have less elements if some oids were not found.
	GetObjectsByOIDs(repoID int64, oids ...string) ([]*LFSObject, error)
}

var LFS LFSStore

type lfs struct {
	*gorm.DB
}

// LFSObject is the relation between an LFS object and a repository.
type LFSObject struct {
	RepoID    int64           `gorm:"PRIMARY_KEY;AUTO_INCREMENT:false"`
	OID       string          `gorm:"PRIMARY_KEY;column:oid"`
	Size      int64           `gorm:"NOT NULL"`
	Storage   lfsutil.Storage `gorm:"NOT NULL"`
	CreatedAt time.Time       `gorm:"NOT NULL"`
}

func (db *lfs) GetObjectsByOIDs(repoID int64, oids ...string) ([]*LFSObject, error) {
	if len(oids) == 0 {
		return []*LFSObject{}, nil
	}

	objects := make([]*LFSObject, 0, len(oids))
	err := db.Where("repo_id = ? AND oid IN (?)", repoID, oids).Find(&objects).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return objects, nil
}
