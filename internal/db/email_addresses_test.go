// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/errutil"
)

func TestEmailAddresses(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(EmailAddress)}
	db := &emailAddresses{
		DB: dbtest.NewDB(t, "emailAddresses", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *emailAddresses)
	}{
		{"GetByEmail", emailAddressesGetByEmail},
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

func emailAddressesGetByEmail(t *testing.T, db *emailAddresses) {
	ctx := context.Background()

	const testEmail = "alice@example.com"
	_, err := db.GetByEmail(ctx, testEmail, false)
	wantErr := ErrEmailNotExist{
		args: errutil.Args{
			"email": testEmail,
		},
	}
	assert.Equal(t, wantErr, err)

	// TODO: Use EmailAddresses.Create to replace SQL hack when the method is available.
	err = db.Exec(`INSERT INTO email_address (uid, email, is_activated) VALUES (1, ?, FALSE)`, testEmail).Error
	require.NoError(t, err)
	got, err := db.GetByEmail(ctx, testEmail, false)
	require.NoError(t, err)
	assert.Equal(t, testEmail, got.Email)

	// Should not return if we only want activated emails
	_, err = db.GetByEmail(ctx, testEmail, true)
	assert.Equal(t, wantErr, err)

	// TODO: Use EmailAddresses.MarkActivated to replace SQL hack when the method is available.
	err = db.Exec(`UPDATE email_address SET is_activated = TRUE WHERE email = ?`, testEmail).Error
	require.NoError(t, err)
	got, err = db.GetByEmail(ctx, testEmail, true)
	require.NoError(t, err)
	assert.Equal(t, testEmail, got.Email)
}
