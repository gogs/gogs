package lfs

import (
	"context"

	"gogs.io/gogs/internal/database"
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
