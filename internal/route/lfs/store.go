package lfs

import (
	"context"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/lfsutil"
)

// Store is the data layer carrier for LFS endpoints. This interface is meant to
// abstract away and limit the exposure of the underlying data layer to the
// handler through a thin-wrapper.
type Store interface {
	// GetAccessTokenBySHA1 returns the access token with given SHA1. It returns
	// database.ErrAccessTokenNotExist when not found.
	GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error)
	// TouchAccessTokenByID updates the updated time of the given access token to
	// the current time.
	TouchAccessTokenByID(ctx context.Context, id int64) error

	// CreateLFSObject creates an LFS object record in database.
	CreateLFSObject(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error
	// GetLFSObjectByOID returns the LFS object with given OID. It returns
	// database.ErrLFSObjectNotExist when not found.
	GetLFSObjectByOID(ctx context.Context, repoID int64, oid lfsutil.OID) (*database.LFSObject, error)
	// GetLFSObjectsByOIDs returns LFS objects found within "oids". The returned
	// list could have fewer elements if some oids were not found.
	GetLFSObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*database.LFSObject, error)

	// AuthorizeRepositoryAccess returns true if the user has as good as desired
	// access mode to the repository.
	AuthorizeRepositoryAccess(ctx context.Context, userID, repoID int64, desired database.AccessMode, opts database.AccessModeOptions) bool

	// GetRepositoryByName returns the repository with given owner and name. It
	// returns database.ErrRepoNotExist when not found.
	GetRepositoryByName(ctx context.Context, ownerID int64, name string) (*database.Repository, error)
}

type store struct{}

// NewStore returns a new Store using the global database handle.
func NewStore() Store {
	return &store{}
}

func (*store) GetAccessTokenBySHA1(ctx context.Context, sha1 string) (*database.AccessToken, error) {
	return database.Handle.AccessTokens().GetBySHA1(ctx, sha1)
}

func (*store) TouchAccessTokenByID(ctx context.Context, id int64) error {
	return database.Handle.AccessTokens().Touch(ctx, id)
}

func (*store) CreateLFSObject(ctx context.Context, repoID int64, oid lfsutil.OID, size int64, storage lfsutil.Storage) error {
	return database.Handle.LFS().CreateObject(ctx, repoID, oid, size, storage)
}

func (*store) GetLFSObjectByOID(ctx context.Context, repoID int64, oid lfsutil.OID) (*database.LFSObject, error) {
	return database.Handle.LFS().GetObjectByOID(ctx, repoID, oid)
}

func (*store) GetLFSObjectsByOIDs(ctx context.Context, repoID int64, oids ...lfsutil.OID) ([]*database.LFSObject, error) {
	return database.Handle.LFS().GetObjectsByOIDs(ctx, repoID, oids...)
}

func (*store) AuthorizeRepositoryAccess(ctx context.Context, userID, repoID int64, desired database.AccessMode, opts database.AccessModeOptions) bool {
	return database.Handle.Permissions().Authorize(ctx, userID, repoID, desired, opts)
}

func (*store) GetRepositoryByName(ctx context.Context, ownerID int64, name string) (*database.Repository, error) {
	return database.Handle.Repositories().GetByName(ctx, ownerID, name)
}
