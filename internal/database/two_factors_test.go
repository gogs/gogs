// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

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

	ctx := context.Background()
	s := &TwoFactorsStore{
		db: newTestDB(t, "TwoFactorsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *TwoFactorsStore)
	}{
		{"Create", twoFactorsCreate},
		{"GetByUserID", twoFactorsGetByUserID},
		{"IsEnabled", twoFactorsIsEnabled},
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

func twoFactorsCreate(t *testing.T, ctx context.Context, s *TwoFactorsStore) {
	// Create a 2FA token
	err := s.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	// Get it back and check the Created field
	tf, err := s.GetByUserID(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, s.db.NowFunc().Format(time.RFC3339), tf.Created.UTC().Format(time.RFC3339))

	// Verify there are 10 recover codes generated
	var count int64
	err = s.db.Model(new(TwoFactorRecoveryCode)).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(10), count)
}

func twoFactorsGetByUserID(t *testing.T, ctx context.Context, s *TwoFactorsStore) {
	// Create a 2FA token for user 1
	err := s.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	// We should be able to get it back
	_, err = s.GetByUserID(ctx, 1)
	require.NoError(t, err)

	// Try to get a non-existent 2FA token
	_, err = s.GetByUserID(ctx, 2)
	wantErr := ErrTwoFactorNotFound{args: errutil.Args{"userID": int64(2)}}
	assert.Equal(t, wantErr, err)
}

func twoFactorsIsEnabled(t *testing.T, ctx context.Context, s *TwoFactorsStore) {
	// Create a 2FA token for user 1
	err := s.Create(ctx, 1, "secure-key", "secure-secret")
	require.NoError(t, err)

	assert.True(t, s.IsEnabled(ctx, 1))
	assert.False(t, s.IsEnabled(ctx, 2))
}
