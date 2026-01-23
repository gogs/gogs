// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/errutil"
)

func TestTwoFactor_BeforeCreate(t *testing.T) {
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
		tf := &TwoFactor{
			CreatedUnix: 1,
		}
		_ = tf.BeforeCreate(db)
		assert.Equal(t, int64(1), tf.CreatedUnix)
	})

	t.Run("CreatedUnix has not been set", func(t *testing.T) {
		tf := &TwoFactor{}
		_ = tf.BeforeCreate(db)
		assert.Equal(t, db.NowFunc().Unix(), tf.CreatedUnix)
	})
}

func TestTwoFactor_AfterFind(t *testing.T) {
	now := time.Now()
	db := &gorm.DB{
		Config: &gorm.Config{
			SkipDefaultTransaction: true,
			NowFunc: func() time.Time {
				return now
			},
		},
	}

	tf := &TwoFactor{
		CreatedUnix: now.Unix(),
	}
	_ = tf.AfterFind(db)
	assert.Equal(t, tf.CreatedUnix, tf.Created.Unix())
}

func TestTwoFactors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(TwoFactor), new(TwoFactorRecoveryCode)}
	db := &twoFactors{
		DB: dbtest.NewDB(t, "twoFactors", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *twoFactors)
	}{
		{"Create", twoFactorsCreate},
		{"GetByUserID", twoFactorsGetByUserID},
		{"IsEnabled", twoFactorsIsEnabled},
		{"UseRecoveryCode", twoFactorsUseRecoveryCode},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				require.NoError(t, err)
			})
			tc.test(t, db)
		})
		if t.Failed() {
			break
		}
	}
}

func twoFactorsCreate(t *testing.T, db *twoFactors) {
	ctx := context.Background()

	// Create a 2FA token
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	// Get it back and check the Created field
	tf, err := db.GetByUserID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), tf.Created.UTC().Format(time.RFC3339))

	// Verify there are 10 recover codes generated
	var count int64
	err = db.Model(new(TwoFactorRecoveryCode)).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

func twoFactorsGetByUserID(t *testing.T, db *twoFactors) {
	ctx := context.Background()

	// Create a 2FA token for user 1
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	// We should be able to get it back
	_, err = db.GetByUserID(ctx, 1)
	require.NoError(t, err)

	// Try to get a non-existent 2FA token
	_, err = db.GetByUserID(ctx, 2)
	wantErr := ErrTwoFactorNotFound{args: errutil.Args{"userID": int64(2)}}
	assert.Equal(t, wantErr, err)
}

func twoFactorsIsEnabled(t *testing.T, db *twoFactors) {
	ctx := context.Background()

	// Create a 2FA token for user 1
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	assert.True(t, db.IsEnabled(ctx, 1))
	assert.False(t, db.IsEnabled(ctx, 2))
}

func twoFactorsUseRecoveryCode(t *testing.T, db *twoFactors) {
	ctx := context.Background()

	// Create 2FA tokens for two users
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)
	err = db.Create(ctx, 2, "secure-key", "secure-secret")
	require.NoError(t, err)

	// Get recovery codes for both users
	var user1Codes []TwoFactorRecoveryCode
	err = db.DB.Where("user_id = ?", 1).Find(&user1Codes).Error
	require.NoError(t, err)
	require.NotEmpty(t, user1Codes)

	var user2Codes []TwoFactorRecoveryCode
	err = db.DB.Where("user_id = ?", 2).Find(&user2Codes).Error
	require.NoError(t, err)
	require.NotEmpty(t, user2Codes)

	// User 1 should be able to use their own recovery code
	err = db.UseRecoveryCode(ctx, 1, user1Codes[0].Code)
	require.NoError(t, err)

	// Verify the code is now marked as used
	var usedCode TwoFactorRecoveryCode
	err = db.DB.Where("id = ?", user1Codes[0].ID).First(&usedCode).Error
	require.NoError(t, err)
	assert.True(t, usedCode.IsUsed)

	// User 1 should NOT be able to use user 2's recovery code
	// This is the key security test - recovery codes must be scoped by user
	err = db.UseRecoveryCode(ctx, 1, user2Codes[0].Code)
	assert.True(t, IsTwoFactorRecoveryCodeNotFound(err), "expected recovery code not found error when using another user's code")

	// User 2's code should still be unused
	var user2Code TwoFactorRecoveryCode
	err = db.DB.Where("id = ?", user2Codes[0].ID).First(&user2Code).Error
	require.NoError(t, err)
	assert.False(t, user2Code.IsUsed, "user 2's recovery code should not be marked as used")

	// User 2 should be able to use their own code
	err = db.UseRecoveryCode(ctx, 2, user2Codes[0].Code)
	require.NoError(t, err)

	// Using an already-used code should fail
	err = db.UseRecoveryCode(ctx, 1, user1Codes[0].Code)
	assert.True(t, IsTwoFactorRecoveryCodeNotFound(err), "expected error when reusing a recovery code")

	// Using a non-existent code should fail
	err = db.UseRecoveryCode(ctx, 1, "invalid-code")
	assert.True(t, IsTwoFactorRecoveryCodeNotFound(err), "expected error for invalid recovery code")
}
