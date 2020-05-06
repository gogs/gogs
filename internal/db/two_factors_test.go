// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
)

func Test_twoFactors(t *testing.T) {
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
		{"Create", test_twoFactors_Create},
		{"GetByUserID", test_twoFactors_GetByUserID},
		{"IsUserEnabled", test_twoFactors_IsUserEnabled},
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

func test_twoFactors_Create(t *testing.T, db *twoFactors) {
	// Create a 2FA token
	err := db.Create(1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	// Get it back and check the Created field
	tf, err := db.GetByUserID(1)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, gorm.NowFunc().Format(time.RFC3339), tf.Created.UTC().Format(time.RFC3339))

	// Verify there are 10 recover codes generated
	var count int64
	err = db.Model(new(TwoFactorRecoveryCode)).Count(&count).Error
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, int64(10), count)
}

func test_twoFactors_GetByUserID(t *testing.T, db *twoFactors) {
	// Create a 2FA token for user 1
	err := db.Create(1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	// We should be able to get it back
	_, err = db.GetByUserID(1)
	if err != nil {
		t.Fatal(err)
	}

	// Try to get a non-existent 2FA token
	_, err = db.GetByUserID(2)
	expErr := ErrTwoFactorNotFound{args: errutil.Args{"userID": int64(2)}}
	assert.Equal(t, expErr, err)
}

func test_twoFactors_IsUserEnabled(t *testing.T, db *twoFactors) {
	// Create a 2FA token for user 1
	err := db.Create(1, "secure-key", "secure-secret")
	if err != nil {
		t.Fatal(err)
	}

	assert.True(t, db.IsUserEnabled(1))
	assert.False(t, db.IsUserEnabled(2))
}
