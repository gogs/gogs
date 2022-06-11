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

	"gogs.io/gogs/internal/errutil"
)

func TestTwoFactors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(TwoFactor), new(TwoFactorRecoveryCode)}
	db := &twoFactors{
		DB: initTestDB(t, "twoFactors", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *twoFactors)
	}{
		{"Create", twoFactorsCreate},
		{"GetByUserID", twoFactorsGetByUserID},
		{"IsUserEnabled", twoFactorsIsUserEnabled},
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

func twoFactorsIsUserEnabled(t *testing.T, db *twoFactors) {
	ctx := context.Background()

	// Create a 2FA token for user 1
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	assert.True(t, db.IsUserEnabled(ctx, 1))
	assert.False(t, db.IsUserEnabled(ctx, 2))
}
