// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package lfsutil

import (
	"io"
	"os"
	"path/filepath"

	"github.com/pkg/errors"

	"gogs.io/gogs/internal/osutil"
)

var ErrObjectNotExist = errors.New("Object does not exist")

// Storager is an storage backend for uploading and downloading LFS objects.
type Storager interface {
	// Storage returns the name of the storage backend.
	Storage() Storage
	// Upload reads content from the io.ReadCloser and uploads as given oid.
	// The reader is closed once upload is finished. ErrInvalidOID is returned
	// if the given oid is not valid.
	Upload(oid OID, rc io.ReadCloser) (int64, error)
	// Download streams content of given oid to the io.Writer. It is caller's
	// responsibility the close the writer when needed. ErrObjectNotExist is
	// returned if the given oid does not exist.
	Download(oid OID, w io.Writer) error
}

// Storage is the storage type of an LFS object.
type Storage string

const (
	StorageLocal Storage = "local"
)

var _ Storager = (*LocalStorage)(nil)

// LocalStorage is a LFS storage backend on local file system.
type LocalStorage struct {
	// The root path for storing LFS objects.
	Root string
}

func (*LocalStorage) Storage() Storage {
	return StorageLocal
}

func (s *LocalStorage) storagePath(oid OID) string {
	if len(oid) < 2 {
		return ""
	}

	return filepath.Join(s.Root, string(oid[0]), string(oid[1]), string(oid))
}

func (s *LocalStorage) Upload(oid OID, rc io.ReadCloser) (int64, error) {
	if !ValidOID(oid) {
		return 0, ErrInvalidOID
	}

	var err error
	fpath := s.storagePath(oid)
	defer func() {
		rc.Close()

		if err != nil {
			_ = os.Remove(fpath)
		}
	}()

	err = os.MkdirAll(filepath.Dir(fpath), os.ModePerm)
	if err != nil {
		return 0, errors.Wrap(err, "create directories")
	}
	w, err := os.Create(fpath)
	if err != nil {
		return 0, errors.Wrap(err, "create file")
	}
	defer w.Close()

	written, err := io.Copy(w, rc)
	if err != nil {
		return 0, errors.Wrap(err, "copy file")
	}
	return written, nil
}

func (s *LocalStorage) Download(oid OID, w io.Writer) error {
	fpath := s.storagePath(oid)
	if !osutil.IsFile(fpath) {
		return ErrObjectNotExist
	}

	r, err := os.Open(fpath)
	if err != nil {
		return errors.Wrap(err, "open file")
	}
	defer r.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return errors.Wrap(err, "copy file")
	}
	return nil
}
