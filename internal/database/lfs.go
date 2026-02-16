package database

import (
	"context"
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
	"gogs.io/gogs/internal/lfsx"
)

// LFSObject is the relation between an LFS object and a repository.
type LFSObject struct {
	RepoID    int64        `gorm:"primaryKey;auto_increment:false"`
	OID       lfsx.OID     `gorm:"primaryKey;column:oid"`
	Size      int64        `gorm:"not null"`
	Storage   lfsx.Storage `gorm:"not null"`
	CreatedAt time.Time    `gorm:"not null"`
}

// LFSStore is the storage layer for LFS objects.
type LFSStore struct {
	db *gorm.DB
}

func newLFSStore(db *gorm.DB) *LFSStore {
	return &LFSStore{db: db}
}

// CreateObject creates an LFS object record in database.
func (s *LFSStore) CreateObject(ctx context.Context, repoID int64, oid lfsx.OID, size int64, storage lfsx.Storage) error {
	object := &LFSObject{
		RepoID:  repoID,
		OID:     oid,
		Size:    size,
		Storage: storage,
	}
	return s.db.WithContext(ctx).Create(object).Error
}

type ErrLFSObjectNotExist struct {
	args errx.Args
}

func IsErrLFSObjectNotExist(err error) bool {
	return errors.As(err, &ErrLFSObjectNotExist{})
}

func (err ErrLFSObjectNotExist) Error() string {
	return fmt.Sprintf("LFS object does not exist: %v", err.args)
}

func (ErrLFSObjectNotExist) NotFound() bool {
	return true
}

// GetObjectByOID returns the LFS object with given OID. It returns
// ErrLFSObjectNotExist when not found.
func (s *LFSStore) GetObjectByOID(ctx context.Context, repoID int64, oid lfsx.OID) (*LFSObject, error) {
	object := new(LFSObject)
	err := s.db.WithContext(ctx).Where("repo_id = ? AND oid = ?", repoID, oid).First(object).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrLFSObjectNotExist{args: errx.Args{"repoID": repoID, "oid": oid}}
		}
		return nil, err
	}
	return object, err
}

// GetObjectsByOIDs returns LFS objects found within "oids". The returned list
// could have fewer elements if some oids were not found.
func (s *LFSStore) GetObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsx.OID) ([]*LFSObject, error) {
	if len(oids) == 0 {
		return []*LFSObject{}, nil
	}

	objects := make([]*LFSObject, 0, len(oids))
	err := s.db.WithContext(ctx).Where("repo_id = ? AND oid IN (?)", repoID, oids).Find(&objects).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return objects, nil
}
