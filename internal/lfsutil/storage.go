package lfsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/osutil"
)

var (
	ErrObjectNotExist = errors.New("object does not exist")
	ErrOIDMismatch    = errors.New("content hash does not match OID")
)

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
	// The path for storing temporary files during upload verification.
	TempDir string
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

	fpath := s.storagePath(oid)
	dir := filepath.Dir(fpath)

	defer rc.Close()

	if err := os.MkdirAll(dir, os.ModePerm); err != nil {
		return 0, errors.Wrap(err, "create directories")
	}

	// If the object file already exists, skip the upload and return the
	// existing file's size.
	if fi, err := os.Stat(fpath); err == nil {
		_, _ = io.Copy(io.Discard, rc)
		return fi.Size(), nil
	}

	// Write to a temp file and verify the content hash before publishing.
	// This ensures the final path always contains a complete, hash-verified
	// file, even when concurrent uploads of the same OID race.
	if err := os.MkdirAll(s.TempDir, os.ModePerm); err != nil {
		return 0, errors.Wrap(err, "create temp directory")
	}
	tmp, err := os.CreateTemp(s.TempDir, "upload-*")
	if err != nil {
		return 0, errors.Wrap(err, "create temp file")
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	hash := sha256.New()
	written, err := io.Copy(tmp, io.TeeReader(rc, hash))
	if closeErr := tmp.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return 0, errors.Wrap(err, "write object file")
	}

	if computed := hex.EncodeToString(hash.Sum(nil)); computed != string(oid) {
		return 0, ErrOIDMismatch
	}

	if err := os.Rename(tmpPath, fpath); err != nil && !os.IsExist(err) {
		return 0, errors.Wrap(err, "publish object file")
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
