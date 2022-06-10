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

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/errutil"
)

func TestUsers(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(User), new(EmailAddress)}
	db := &users{
		DB: initTestDB(t, "users", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *users)
	}{
		{"Authenticate", usersAuthenticate},
		{"Create", usersCreate},
		{"GetByEmail", usersGetByEmail},
		{"GetByID", usersGetByID},
		{"GetByUsername", usersGetByUsername},
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

// TODO: Only local account is tested, tests for external account will be added
//  along with addressing https://github.com/gogs/gogs/issues/6115.
func usersAuthenticate(t *testing.T, db *users) {
	ctx := context.Background()

	password := "pa$$word"
	alice, err := db.Create(ctx, "alice", "alice@example.com",
		CreateUserOpts{
			Password: password,
		},
	)
	require.NoError(t, err)

	t.Run("user not found", func(t *testing.T) {
		_, err := db.Authenticate(ctx, "bob", password, -1)
		wantErr := auth.ErrBadCredentials{Args: map[string]interface{}{"login": "bob"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("invalid password", func(t *testing.T) {
		_, err := db.Authenticate(ctx, alice.Name, "bad_password", -1)
		wantErr := auth.ErrBadCredentials{Args: map[string]interface{}{"login": alice.Name, "userID": alice.ID}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("via email and password", func(t *testing.T) {
		user, err := db.Authenticate(ctx, alice.Email, password, -1)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("via username and password", func(t *testing.T) {
		user, err := db.Authenticate(ctx, alice.Name, password, -1)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})
}

func usersCreate(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@example.com",
		CreateUserOpts{
			Activated: true,
		},
	)
	require.NoError(t, err)

	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.Create(ctx, "-", "", CreateUserOpts{})
		wantErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "name": "-"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("name already exists", func(t *testing.T) {
		_, err := db.Create(ctx, alice.Name, "", CreateUserOpts{})
		wantErr := ErrUserAlreadyExist{args: errutil.Args{"name": alice.Name}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("email already exists", func(t *testing.T) {
		_, err := db.Create(ctx, "bob", alice.Email, CreateUserOpts{})
		wantErr := ErrEmailAlreadyUsed{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, wantErr, err)
	})

	user, err := db.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Updated.UTC().Format(time.RFC3339))
}

func usersGetByEmail(t *testing.T, db *users) {
	ctx := context.Background()

	t.Run("empty email", func(t *testing.T) {
		_, err := db.GetByEmail(ctx, "")
		wantErr := ErrUserNotExist{args: errutil.Args{"email": ""}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("ignore organization", func(t *testing.T) {
		// TODO: Use Orgs.Create to replace SQL hack when the method is available.
		org, err := db.Create(ctx, "gogs", "gogs@exmaple.com", CreateUserOpts{})
		require.NoError(t, err)

		err = db.Model(&User{}).Where("id", org.ID).UpdateColumn("type", UserOrganization).Error
		require.NoError(t, err)

		_, err = db.GetByEmail(ctx, org.Email)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": org.Email}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("by primary email", func(t *testing.T) {
		alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOpts{})
		require.NoError(t, err)

		_, err = db.GetByEmail(ctx, alice.Email)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, wantErr, err)

		// Mark user as activated
		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = db.Model(&User{}).Where("id", alice.ID).UpdateColumn("is_active", true).Error
		require.NoError(t, err)

		user, err := db.GetByEmail(ctx, alice.Email)
		require.NoError(t, err)
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("by secondary email", func(t *testing.T) {
		bob, err := db.Create(ctx, "bob", "bob@example.com", CreateUserOpts{})
		require.NoError(t, err)

		// TODO: Use UserEmails.Create to replace SQL hack when the method is available.
		email2 := "bob2@exmaple.com"
		err = db.Exec(`INSERT INTO email_address (uid, email) VALUES (?, ?)`, bob.ID, email2).Error
		require.NoError(t, err)

		_, err = db.GetByEmail(ctx, email2)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": email2}}
		assert.Equal(t, wantErr, err)

		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = db.Exec(`UPDATE email_address SET is_activated = ? WHERE email = ?`, true, email2).Error
		require.NoError(t, err)

		user, err := db.GetByEmail(ctx, email2)
		require.NoError(t, err)
		assert.Equal(t, bob.Name, user.Name)
	})
}

func usersGetByID(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOpts{})
	require.NoError(t, err)

	user, err := db.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = db.GetByID(ctx, 404)
	wantErr := ErrUserNotExist{args: errutil.Args{"userID": int64(404)}}
	assert.Equal(t, wantErr, err)
}

func usersGetByUsername(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOpts{})
	require.NoError(t, err)

	user, err := db.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = db.GetByUsername(ctx, "bad_username")
	wantErr := ErrUserNotExist{args: errutil.Args{"name": "bad_username"}}
	assert.Equal(t, wantErr, err)
}
