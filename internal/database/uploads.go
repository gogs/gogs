package database

import (
	"context"
	"io"
	"os"
	"path"
	"slices"

	"github.com/cockroachdb/errors"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/pathx"
)

// UploadsStore is the storage layer for files uploaded to be committed later.
type UploadsStore struct {
	db *gorm.DB
}

func newUploadsStore(db *gorm.DB) *UploadsStore {
	return &UploadsStore{db: db}
}

// New stores the content of given reader as an upload with given file name.
func (s *UploadsStore) New(ctx context.Context, name string, r io.Reader) (*Upload, error) {
	// 🚨 SECURITY: Prevent path traversal.
	name = pathx.Clean(name)
	if name == "" {
		return nil, errors.New("empty file name")
	}

	upload := &Upload{
		UUID: uuid.New().String(),
		Name: name,
	}

	localPath := upload.LocalPath()
	err := os.MkdirAll(path.Dir(localPath), os.ModePerm)
	if err != nil {
		return nil, errors.Wrap(err, "create directory")
	}

	fw, err := os.Create(localPath)
	if err != nil {
		return nil, errors.Wrap(err, "create file")
	}
	defer func() { _ = fw.Close() }()

	_, err = io.Copy(fw, r)
	if err != nil {
		return nil, errors.Wrap(err, "write file")
	}

	err = s.db.WithContext(ctx).Create(upload).Error
	if err != nil {
		return nil, err
	}
	return upload, nil
}

// isValidUUID returns true if given string is a UUID. PostgreSQL rejects
// queries that compare the "uuid" column with anything else.
func isValidUUID(s string) bool {
	_, err := uuid.Parse(s)
	return err == nil
}

// GetByUUID returns the upload with given UUID. It returns ErrUploadNotExist
// when not found.
func (s *UploadsStore) GetByUUID(ctx context.Context, uuid string) (*Upload, error) {
	if !isValidUUID(uuid) {
		return nil, ErrUploadNotExist{0, uuid}
	}

	upload := new(Upload)
	err := s.db.WithContext(ctx).Where("uuid = ?", uuid).First(upload).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUploadNotExist{0, uuid}
		}
		return nil, err
	}
	return upload, nil
}

// ListByUUIDs returns the uploads with given UUIDs. UUIDs that no upload has
// are skipped.
func (s *UploadsStore) ListByUUIDs(ctx context.Context, uuids []string) ([]*Upload, error) {
	uuids = slices.DeleteFunc(slices.Clone(uuids), func(s string) bool { return !isValidUUID(s) })
	if len(uuids) == 0 {
		return []*Upload{}, nil
	}

	uploads := make([]*Upload, 0, len(uuids))
	return uploads, s.db.WithContext(ctx).Where("uuid IN ?", uuids).Find(&uploads).Error
}

// Delete deletes given uploads and their files.
func (s *UploadsStore) Delete(ctx context.Context, uploads ...*Upload) error {
	if len(uploads) == 0 {
		return nil
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		ids := make([]int64, len(uploads))
		for i := range uploads {
			ids[i] = uploads[i].ID
		}
		err := tx.Where("id IN ?", ids).Delete(new(Upload)).Error
		if err != nil {
			return errors.Wrap(err, "delete uploads")
		}

		for _, upload := range uploads {
			localPath := upload.LocalPath()
			if !osx.IsFile(localPath) {
				continue
			}

			err = os.Remove(localPath)
			if err != nil {
				return errors.Wrap(err, "remove file")
			}
		}
		return nil
	})
}

// DeleteByUUID deletes the upload with given UUID and its file. Deleting an
// upload that does not exist is not an error.
func (s *UploadsStore) DeleteByUUID(ctx context.Context, uuid string) error {
	upload, err := s.GetByUUID(ctx, uuid)
	if err != nil {
		if IsErrUploadNotExist(err) {
			return nil
		}
		return errors.Wrap(err, "get upload")
	}
	return s.Delete(ctx, upload)
}
