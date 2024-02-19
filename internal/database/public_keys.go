// Copyright 2023 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"os"
	"path/filepath"

	"github.com/pkg/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/osutil"
)

// PublicKeysStore is the persistent interface for public keys.
type PublicKeysStore interface {
	// RewriteAuthorizedKeys rewrites the "authorized_keys" file under the SSH root
	// path with all public keys stored in the database.
	RewriteAuthorizedKeys() error
}

var PublicKeys PublicKeysStore

var _ PublicKeysStore = (*publicKeys)(nil)

type publicKeys struct {
	*gorm.DB
}

// NewPublicKeysStore returns a persistent interface for public keys with given
// database connection.
func NewPublicKeysStore(db *gorm.DB) PublicKeysStore {
	return &publicKeys{DB: db}
}

func authorizedKeysPath() string {
	return filepath.Join(conf.SSH.RootPath, "authorized_keys")
}

func (db *publicKeys) RewriteAuthorizedKeys() error {
	sshOpLocker.Lock()
	defer sshOpLocker.Unlock()

	err := os.MkdirAll(conf.SSH.RootPath, os.ModePerm)
	if err != nil {
		return errors.Wrap(err, "create SSH root path")
	}
	fpath := authorizedKeysPath()
	tempPath := fpath + ".tmp"
	f, err := os.OpenFile(tempPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Wrap(err, "create temporary file")
	}
	defer func() {
		_ = f.Close()
		_ = os.Remove(tempPath)
	}()

	// NOTE: More recently updated keys are more likely to be used more frequently,
	// putting them in the earlier lines could speed up the key lookup by SSHD.
	rows, err := db.Model(&PublicKey{}).Order("updated_unix DESC").Rows()
	if err != nil {
		return errors.Wrap(err, "iterate public keys")
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var key PublicKey
		err = db.ScanRows(rows, &key)
		if err != nil {
			return errors.Wrap(err, "scan rows")
		}

		_, err = f.WriteString(key.AuthorizedString())
		if err != nil {
			return errors.Wrapf(err, "write key %d", key.ID)
		}
	}
	if err = rows.Err(); err != nil {
		return errors.Wrap(err, "check rows.Err")
	}

	err = f.Close()
	if err != nil {
		return errors.Wrap(err, "close temporary file")
	}
	if osutil.IsExist(fpath) {
		err = os.Remove(fpath)
		if err != nil {
			return errors.Wrap(err, "remove")
		}
	}
	err = os.Rename(tempPath, fpath)
	if err != nil {
		return errors.Wrap(err, "rename")
	}
	return nil
}
