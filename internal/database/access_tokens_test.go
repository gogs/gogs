package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
)

func TestAccessToken_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		token := &AccessToken{
			CreatedUnix: 1,
		}
		_ = token.BeforeCreate(db)
		assert.Equal(t, int64(1), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix) // Do not set UpdatedUnix until it is used.
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		token := &AccessToken{}
		_ = token.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix) // Do not set UpdatedUnix until it is used.
	})
}

func TestAccessToken_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("UpdatedUnix has been set and within 7 days", func(t *testing.T) {
		token := &AccessToken{
			CreatedUnix: now.Unix(),
			UpdatedUnix: now.Add(time.Second).Unix(),
		}
		_ = token.AfterFind(db)
		assert.Equal(t, token.CreatedUnix, token.Created.Unix())
		assert.Equal(t, token.UpdatedUnix, token.Updated.Unix())
		assert.True(t, token.HasUsed)
		assert.True(t, token.HasRecentActivity)
	})

	t.Run("UpdatedUnix has been set and not within 7 days", func(t *testing.T) {
		token := &AccessToken{
			CreatedUnix: now.Add(-1 * 9 * 24 * time.Hour).Unix(),
			UpdatedUnix: now.Add(-1 * 8 * 24 * time.Hour).Unix(),
		}
		_ = token.AfterFind(db)
		assert.Equal(t, token.CreatedUnix, token.Created.Unix())
		assert.Equal(t, token.UpdatedUnix, token.Updated.Unix())
		assert.True(t, token.HasUsed)
		assert.False(t, token.HasRecentActivity)
	})

	t.Run("UpdatedUnix has not been set", func(t *testing.T) {
		token := &AccessToken{
			CreatedUnix: now.Unix(),
		}
		_ = token.AfterFind(db)
		assert.Equal(t, token.CreatedUnix, token.Created.Unix())
		assert.True(t, token.Updated.IsZero())
		assert.False(t, token.HasUsed)
		assert.False(t, token.HasRecentActivity)
	})
}

func TestAccessTokens(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &AccessTokensStore{
		db: newTestDB(t, "AccessTokensStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *AccessTokensStore)
	}{
		{"Create", accessTokensCreate},
		{"DeleteByID", accessTokensDeleteByID},
		{"GetBySHA1", accessTokensGetBySHA},
		{"List", accessTokensList},
		{"Touch", accessTokensTouch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func accessTokensCreate(t *testing.T, ctx context.Context, s *AccessTokensStore) {
	token, err := s.Create(ctx, 1, "Test")
	require.NoError(t, err)

	assert.Equal(t, int64(1), token.UserID)
	assert.Equal(t, "Test", token.Name)
	assert.Equal(t, 40, len(token.Sha1), "sha1 length")

	token, err = s.GetBySHA1(ctx, token.Sha1)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), token.Created.UTC().Format(time.RFC3339))

	// A second access token with the same name cannot be created.
	_, err = s.Create(ctx, token.UserID, token.Name)
	wantErr := ErrAccessTokenAlreadyExist{
		args: errx.Args{
			"userID": token.UserID,
			"name":   token.Name,
		},
	}
	assert.Equal(t, wantErr, err)
}

func accessTokensDeleteByID(t *testing.T, ctx context.Context, s *AccessTokensStore) {
	token, err := s.Create(ctx, 1, "Test")
	require.NoError(t, err)

	// Deleting a token with a mismatched user ID is a no-op.
	err = s.DeleteByID(ctx, 2, token.ID)
	require.NoError(t, err)

	_, err = s.GetBySHA1(ctx, token.Sha1)
	require.NoError(t, err)

	err = s.DeleteByID(ctx, token.UserID, token.ID)
	require.NoError(t, err)

	_, err = s.GetBySHA1(ctx, token.Sha1)
	wantErr := ErrAccessTokenNotExist{
		args: errx.Args{
			"sha": token.Sha1,
		},
	}
	assert.Equal(t, wantErr, err)
}

func accessTokensGetBySHA(t *testing.T, ctx context.Context, s *AccessTokensStore) {
	token, err := s.Create(ctx, 1, "Test")
	require.NoError(t, err)

	_, err = s.GetBySHA1(ctx, token.Sha1)
	require.NoError(t, err)

	_, err = s.GetBySHA1(ctx, "bad_sha")
	wantErr := ErrAccessTokenNotExist{
		args: errx.Args{
			"sha": "bad_sha",
		},
	}
	assert.Equal(t, wantErr, err)
}

func accessTokensList(t *testing.T, ctx context.Context, s *AccessTokensStore) {
	_, err := s.Create(ctx, 1, "user1_1")
	require.NoError(t, err)
	_, err = s.Create(ctx, 1, "user1_2")
	require.NoError(t, err)

	_, err = s.Create(ctx, 2, "user2_1")
	require.NoError(t, err)

	tokens, err := s.List(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, 2, len(tokens), "number of tokens")

	assert.Equal(t, int64(1), tokens[0].UserID)
	assert.Equal(t, "user1_1", tokens[0].Name)

	assert.Equal(t, int64(1), tokens[1].UserID)
	assert.Equal(t, "user1_2", tokens[1].Name)
}

func accessTokensTouch(t *testing.T, ctx context.Context, s *AccessTokensStore) {
	token, err := s.Create(ctx, 1, "Test")
	require.NoError(t, err)

	assert.True(t, token.Updated.IsZero())

	err = s.Touch(ctx, token.ID)
	require.NoError(t, err)

	token, err = s.GetBySHA1(ctx, token.Sha1)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), token.Updated.UTC().Format(time.RFC3339))
}
