// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
)

func TestTwoFactors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(TwoFactor), new(TwoFactorRecoveryCode)}
	db, cleanup := newTestDB(t, "twoFactors", tables...)
	store := NewTwoFactorsStore(db)

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *twoFactors)
	}{
		{"Create", testTwoFactorsCreate},
		{"GetByUserID", testTwoFactorsGetByUserID},
		{"IsUserEnabled", testTwoFactorsIsUserEnabled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := cleanup()
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, context.Background(), store.(*twoFactors))
		})
	}
}

func testTwoFactorsCreate(t *testing.T, ctx context.Context, db *twoFactors) {
	// Create a 2FA token
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	// Get it back and check the Created field
	tf, err := db.GetByUserID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), tf.Created.Format(time.RFC3339))

	// Verify there are 10 recover codes generated
	var count int64
	err = db.Model(new(TwoFactorRecoveryCode)).Count(&count).Error
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(10), count)
}

func testTwoFactorsGetByUserID(t *testing.T, ctx context.Context, db *twoFactors) {
	// Create a 2FA token for user 1
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetByUserID(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}

	// Try to get a non-existent 2FA token
	_, err = db.GetByUserID(ctx, 2)
	expErr := ErrTwoFactorNotFound{args: errutil.Args{"userID": int64(2)}}
	assert.Equal(t, expErr, err)
}

func testTwoFactorsIsUserEnabled(t *testing.T, ctx context.Context, db *twoFactors) {
	// Create a 2FA token for user 1
	err := db.Create(ctx, 1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, db.IsUserEnabled(ctx, 1))
	assert.False(t, db.IsUserEnabled(ctx, 2))
}
