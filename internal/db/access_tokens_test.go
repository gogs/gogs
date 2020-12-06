// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errutil"
)

func TestAccessToken_BeforeCreate(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			NowFunc: func() time.Time { return now },
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		token := &AccessToken{CreatedUnix: 1}
		err := token.BeforeCreate(db)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, int64(1), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		token := &AccessToken{}
		err := token.BeforeCreate(db)
		if err != nil {
			t.Fatal(err)
		}

		assert.Equal(t, db.NowFunc().Unix(), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix)
	})
}

func TestAccessTokens(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{&AccessToken{}}
	db, cleanup := newTestDB(t, "accessTokens", tables...)
	store := NewAccessTokensStore(db)

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *accessTokens)
	}{
		{"Create", testAccessTokensCreate},
		{"DeleteByID", testAccessTokensDeleteByID},
		{"GetBySHA", testAccessTokensGetBySHA},
		{"List", testAccessTokensList},
		{"Save", testAccessTokensSave},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := cleanup()
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, context.Background(), store.(*accessTokens))
		})
	}
}

func testAccessTokensCreate(t *testing.T, ctx context.Context, db *accessTokens) {
	// Create first access token with name "Test"
	token, err := db.Create(ctx, 1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, int64(1), token.UserID)
	assert.Equal(t, "Test", token.Name)
	assert.Equal(t, 40, len(token.Sha1), "sha1 length")

	// Get it back and check the Created field
	token, err = db.GetBySHA(ctx, token.Sha1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), token.Created.Format(time.RFC3339))

	// Try create second access token with same name should fail
	_, err = db.Create(ctx, token.UserID, token.Name)
	wantErr := ErrAccessTokenAlreadyExist{
		args: errutil.Args{
			"userID": token.UserID,
			"name":   token.Name,
		},
	}
	assert.Equal(t, wantErr, err)
}

func testAccessTokensDeleteByID(t *testing.T, ctx context.Context, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(ctx, 1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// Delete a token with mismatched user ID is noop
	err = db.DeleteByID(ctx, 2, token.ID)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetBySHA(ctx, token.Sha1)
	if err != nil {
		t.Fatal(err)
	}

	// Now delete this token with correct user ID
	err = db.DeleteByID(ctx, token.UserID, token.ID)
	if err != nil {
		t.Fatal(err)
	}

	// We should get token not found error
	_, err = db.GetBySHA(ctx, token.Sha1)
	want := ErrAccessTokenNotExist{
		args: errutil.Args{
			"sha": token.Sha1,
		},
	}
	assert.Equal(t, want, err)
}

func testAccessTokensGetBySHA(t *testing.T, ctx context.Context, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(ctx, 1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	got, err := db.GetBySHA(ctx, token.Sha1)
	if err != nil {
		t.Fatal(err)
	}
	want := &AccessToken{
		ID:          token.ID,
		UserID:      token.UserID,
		Name:        token.Name,
		Sha1:        token.Sha1,
		Created:     db.NowFunc(),
		CreatedUnix: db.NowFunc().Unix(),
	}
	assert.Equal(t, want, got)

	// Try to get a non-existent token
	_, err = db.GetBySHA(ctx, "bad_sha")
	wantErr := ErrAccessTokenNotExist{
		args: errutil.Args{
			"sha": "bad_sha",
		},
	}
	assert.Equal(t, wantErr, err)
}

func testAccessTokensList(t *testing.T, ctx context.Context, db *accessTokens) {
	// Create two access tokens for user 1
	token1, err := db.Create(ctx, 1, "user1_1")
	if err != nil {
		t.Fatal(err)
	}
	token2, err := db.Create(ctx, 1, "user1_2")
	if err != nil {
		t.Fatal(err)
	}

	// Create one access token for user 2
	_, err = db.Create(ctx, 2, "user2_1")
	if err != nil {
		t.Fatal(err)
	}

	// List all access tokens for user 1
	tokens, err := db.List(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []*AccessToken{
		{
			ID:          token1.ID,
			UserID:      token1.UserID,
			Name:        token1.Name,
			Sha1:        token1.Sha1,
			Created:     db.NowFunc(),
			CreatedUnix: db.NowFunc().Unix(),
		}, {
			ID:          token2.ID,
			UserID:      token2.UserID,
			Name:        token2.Name,
			Sha1:        token2.Sha1,
			Created:     db.NowFunc(),
			CreatedUnix: db.NowFunc().Unix(),
		},
	}
	assert.Equal(t, want, tokens)
}

func testAccessTokensSave(t *testing.T, ctx context.Context, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(ctx, 1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// Updated field is zero now
	assert.True(t, token.Updated.IsZero())

	err = db.Save(ctx, token)
	if err != nil {
		t.Fatal(err)
	}

	// Get back from DB should have Updated set
	got, err := db.GetBySHA(ctx, token.Sha1)
	if err != nil {
		t.Fatal(err)
	}

	want := &AccessToken{
		ID:                token.ID,
		UserID:            token.UserID,
		Name:              token.Name,
		Sha1:              token.Sha1,
		Created:           db.NowFunc(),
		CreatedUnix:       db.NowFunc().Unix(),
		Updated:           db.NowFunc(),
		UpdatedUnix:       db.NowFunc().Unix(),
		HasRecentActivity: true,
	}
	assert.Equal(t, want, got)
}
