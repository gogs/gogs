// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/lfsutil"
)

// LFSStore is the persistent interface for LFS objects.
//
// NOTE: All methods are sorted in alphabetical order.
type LFSStore interface {
	// CreateObject streams io.ReadCloser to target storage and creates a record in database.
	CreateObject(repoID int64, oid lfsutil.OID, rc io.ReadCloser, storage lfsutil.Storage) error
	// GetObjectByOID returns the LFS object with given OID. It returns ErrLFSObjectNotExist
	// when not found.
	GetObjectByOID(repoID int64, oid lfsutil.OID) (*LFSObject, error)
	// GetObjectsByOIDs returns LFS objects found within "oids". The returned list could have
	// less elements if some oids were not found.
	GetObjectsByOIDs(repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error)
}

var LFS LFSStore

type lfs struct {
	*gorm.DB
}

// LFSObject is the relation between an LFS object and a repository.
type LFSObject struct {
	RepoID    int64           `gorm:"PRIMARY_KEY;AUTO_INCREMENT:false"`
	OID       lfsutil.OID     `gorm:"PRIMARY_KEY;column:oid"`
	Size      int64           `gorm:"NOT NULL"`
	Storage   lfsutil.Storage `gorm:"NOT NULL"`
	CreatedAt time.Time       `gorm:"NOT NULL"`
}

func (db *lfs) CreateObject(repoID int64, oid lfsutil.OID, rc io.ReadCloser, storage lfsutil.Storage) error {
	if storage != lfsutil.StorageLocal {
		return errors.New("only local storage is supported")
	}

	var ioerr error
	fpath := lfsutil.StorageLocalPath(conf.LFS.ObjectsPath, oid)
	defer func() {
		rc.Close()

		// NOTE: Only remove the file if there is an IO error, it is OK to
		// leave the file when the whole operation failed with a DB error,
		// a retry on client side can safely overwrite the same file as OID
		// is seen as unique to every file.
		if ioerr != nil {
			_ = os.Remove(fpath)
		}
	}()

	ioerr = os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	if ioerr != nil {
		return errors.Wrap(ioerr, "create directories")
	}
	w, ioerr := os.Create(fpath)
	if ioerr != nil {
		return errors.Wrap(ioerr, "create file")
	}
	defer w.Close()

	written, ioerr := io.Copy(w, rc)
	if ioerr != nil {
		return errors.Wrap(ioerr, "copy file")
	}

	object := &LFSObject{
		RepoID:  repoID,
		OID:     oid,
		Size:    written,
		Storage: storage,
	}
	return db.DB.Create(object).Error
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

func (db *lfs) GetObjectByOID(repoID int64, oid lfsutil.OID) (*LFSObject, error) {
	object := new(LFSObject)
	err := db.Where("repo_id = ? AND oid = ?", repoID, oid).First(object).Error
	if err != nil {
		if gorm.IsRecordNotFoundError(err) {
			return nil, ErrLFSObjectNotExist{args: errutil.Args{"repoID": repoID, "oid": oid}}
		}
		return nil, err
	}
	return object, err
}

func (db *lfs) GetObjectsByOIDs(repoID int64, oids ...lfsutil.OID) ([]*LFSObject, error) {
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
