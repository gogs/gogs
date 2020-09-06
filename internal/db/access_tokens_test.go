// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
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
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	t.Run("CreatedUnix has been set", func(t *testing.T) {
		token := &AccessToken{CreatedUnix: 1}
		_ = token.BeforeCreate(db)
		assert.Equal(t, int64(1), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		token := &AccessToken{}
		_ = token.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), token.CreatedUnix)
		assert.Equal(t, int64(0), token.UpdatedUnix)
	})
}

func Test_accessTokens(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(AccessToken)}
	db := &accessTokens{
		DB: initTestDB(t, "accessTokens", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *accessTokens)
	}{
		{"Create", test_accessTokens_Create},
		{"DeleteByID", test_accessTokens_DeleteByID},
		{"GetBySHA", test_accessTokens_GetBySHA},
		{"List", test_accessTokens_List},
		{"Save", test_accessTokens_Save},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, db)
		})
	}
}

func test_accessTokens_Create(t *testing.T, db *accessTokens) {
	// Create first access token with name "Test"
	token, err := db.Create(1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, int64(1), token.UserID)
	assert.Equal(t, "Test", token.Name)
	assert.Equal(t, 40, len(token.Sha1), "sha1 length")

	// Get it back and check the Created field
	token, err = db.GetBySHA(token.Sha1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), token.Created.UTC().Format(time.RFC3339))

	// Try create second access token with same name should fail
	_, err = db.Create(token.UserID, token.Name)
	expErr := ErrAccessTokenAlreadyExist{args: errutil.Args{"userID": token.UserID, "name": token.Name}}
	assert.Equal(t, expErr, err)
}

func test_accessTokens_DeleteByID(t *testing.T, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// Delete a token with mismatched user ID is noop
	err = db.DeleteByID(2, token.ID)
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetBySHA(token.Sha1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.GetBySHA(token.Sha1)
	if err != nil {
		t.Fatal(err)
	}

	// Now delete this token with correct user ID
	err = db.DeleteByID(token.UserID, token.ID)
	if err != nil {
		t.Fatal(err)
	}

	// We should get token not found error
	_, err = db.GetBySHA(token.Sha1)
	expErr := ErrAccessTokenNotExist{args: errutil.Args{"sha": token.Sha1}}
	assert.Equal(t, expErr, err)
}

func test_accessTokens_GetBySHA(t *testing.T, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetBySHA(token.Sha1)
	if err != nil {
		t.Fatal(err)
	}

	// Try to get a non-existent token
	_, err = db.GetBySHA("bad_sha")
	expErr := ErrAccessTokenNotExist{args: errutil.Args{"sha": "bad_sha"}}
	assert.Equal(t, expErr, err)
}

func test_accessTokens_List(t *testing.T, db *accessTokens) {
	// Create two access tokens for user 1
	_, err := db.Create(1, "user1_1")
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Create(1, "user1_2")
	if err != nil {
		t.Fatal(err)
	}

	// Create one access token for user 2
	_, err = db.Create(2, "user2_1")
	if err != nil {
		t.Fatal(err)
	}

	// List all access tokens for user 1
	tokens, err := db.List(1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, 2, len(tokens), "number of tokens")

	assert.Equal(t, int64(1), tokens[0].UserID)
	assert.Equal(t, "user1_1", tokens[0].Name)

	assert.Equal(t, int64(1), tokens[1].UserID)
	assert.Equal(t, "user1_2", tokens[1].Name)
}

func test_accessTokens_Save(t *testing.T, db *accessTokens) {
	// Create an access token with name "Test"
	token, err := db.Create(1, "Test")
	if err != nil {
		t.Fatal(err)
	}

	// Updated field is zero now
	assert.True(t, token.Updated.IsZero())

	err = db.Save(token)
	if err != nil {
		t.Fatal(err)
	}

	// Get back from DB should have Updated set
	token, err = db.GetBySHA(token.Sha1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), token.Updated.UTC().Format(time.RFC3339))
}
