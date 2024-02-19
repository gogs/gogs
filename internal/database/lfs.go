// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lfsutil"
)

// LFSStore is the persistent interface for LFS objects.
type LFSStore interface {
	// CreateObject creates a LFS object record in database.
	CreateObject(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error
	// GetObjectByOID returns the LFS object with given OID. It returns
	// ErrLFSObjectNotExist when not found.
	GetObjectByOID(ctx context.Context, repoID int64, oid lfsutil.OID) (*LFSObject, error)
	// GetObjectsByOIDs returns LFS objects found within "oids". The returned list
	// could have less elements if some oids were not found.
	GetObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error)
}

var LFS LFSStore

// LFSObject is the relation between an LFS object and a repository.
type LFSObject struct {
	RepoID    int64           `gorm:"primaryKey;auto_increment:false"`
	OID       lfsutil.OID     `gorm:"primaryKey;column:oid"`
	Size      int64           `gorm:"not null"`
	Storage   lfsutil.Storage `gorm:"not null"`
	CreatedAt time.Time       `gorm:"not null"`
}

var _ LFSStore = (*lfs)(nil)

type lfs struct {
	*gorm.DB
}

func (db *lfs) CreateObject(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error {
	object := &LFSObject{
		RepoID:  repoID,
		OID:     oid,
		Size:    size,
		Storage: storage,
	}
	return db.WithContext(ctx).Create(object).Error
}

type ErrLFSObjectNotExist struct {
	args errutil.Args
}

func IsErrLFSObjectNotExist(err error) bool {
	_, ok := err.(ErrLFSObjectNotExist)
	return ok
}

func (err ErrLFSObjectNotExist) Error() string {
	return fmt.Sprintf("LFS object does not exist: %v", err.args)
}

func (ErrLFSObjectNotExist) NotFound() bool {
	return true
}

func (db *lfs) GetObjectByOID(ctx context.Context, repoID int64, oid lfsutil.OID) (*LFSObject, error) {
	object := new(LFSObject)
	err := db.WithContext(ctx).Where("repo_id = ? AND oid = ?", repoID, oid).First(object).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, ErrLFSObjectNotExist{args: errutil.Args{"repoID": repoID, "oid": oid}}
		}
		return nil, err
	}
	return object, err
}

func (db *lfs) GetObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error) {
	if len(oids) == 0 {
		return []*LFSObject{}, nil
	}

	objects := make([]*LFSObject, 0, len(oids))
	err := db.WithContext(ctx).Where("repo_id = ? AND oid IN (?)", repoID, oids).Find(&objects).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return nil, err
	}
	return objects, nil
}
