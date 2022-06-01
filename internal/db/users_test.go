// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/errutil"
)

func Test_users(t *testing.T) {
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
		{"Authenticate", test_users_Authenticate},
		{"Create", test_users_Create},
		{"GetByEmail", test_users_GetByEmail},
		{"GetByID", test_users_GetByID},
		{"GetByUsername", test_users_GetByUsername},
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
		if t.Failed() {
			break
		}
	}
}

// TODO: Only local account is tested, tests for external account will be added
//  along with addressing https://github.com/gogs/gogs/issues/6115.
func test_users_Authenticate(t *testing.T, db *users) {
	password := "pa$$word"
	alice, err := db.Create("alice", "alice@example.com", CreateUserOpts{
		Password: password,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("user not found", func(t *testing.T) {
		_, err := db.Authenticate("bob", password, -1)
		expErr := auth.ErrBadCredentials{Args: map[string]interface{}{"login": "bob"}}
		assert.Equal(t, expErr, err)
	})

	t.Run("invalid password", func(t *testing.T) {
		_, err := db.Authenticate(alice.Name, "bad_password", -1)
		expErr := auth.ErrBadCredentials{Args: map[string]interface{}{"login": alice.Name, "userID": alice.ID}}
		assert.Equal(t, expErr, err)
	})

	t.Run("via email and password", func(t *testing.T) {
		user, err := db.Authenticate(alice.Email, password, -1)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("via username and password", func(t *testing.T) {
		user, err := db.Authenticate(alice.Name, password, -1)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, alice.Name, user.Name)
	})
}

func test_users_Create(t *testing.T, db *users) {
	alice, err := db.Create("alice", "alice@example.com", CreateUserOpts{
		Activated: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.Create("-", "", CreateUserOpts{})
		expErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "name": "-"}}
		assert.Equal(t, expErr, err)
	})

	t.Run("name already exists", func(t *testing.T) {
		_, err := db.Create(alice.Name, "", CreateUserOpts{})
		expErr := ErrUserAlreadyExist{args: errutil.Args{"name": alice.Name}}
		assert.Equal(t, expErr, err)
	})

	t.Run("email already exists", func(t *testing.T) {
		_, err := db.Create("bob", alice.Email, CreateUserOpts{})
		expErr := ErrEmailAlreadyUsed{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, expErr, err)
	})

	user, err := db.GetByUsername(alice.Name)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Updated.UTC().Format(time.RFC3339))
}

func test_users_GetByEmail(t *testing.T, db *users) {
	t.Run("empty email", func(t *testing.T) {
		_, err := db.GetByEmail("")
		expErr := ErrUserNotExist{args: errutil.Args{"email": ""}}
		assert.Equal(t, expErr, err)
	})

	t.Run("ignore organization", func(t *testing.T) {
		// TODO: Use Orgs.Create to replace SQL hack when the method is available.
		org, err := db.Create("gogs", "gogs@exmaple.com", CreateUserOpts{})
		if err != nil {
			t.Fatal(err)
		}

		err = db.Model(&User{}).Where("id", org.ID).UpdateColumn("type", UserOrganization).Error
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.GetByEmail(org.Email)
		expErr := ErrUserNotExist{args: errutil.Args{"email": org.Email}}
		assert.Equal(t, expErr, err)
	})

	t.Run("by primary email", func(t *testing.T) {
		alice, err := db.Create("alice", "alice@exmaple.com", CreateUserOpts{})
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.GetByEmail(alice.Email)
		expErr := ErrUserNotExist{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, expErr, err)

		// Mark user as activated
		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = db.Model(&User{}).Where("id", alice.ID).UpdateColumn("is_active", true).Error
		if err != nil {
			t.Fatal(err)
		}

		user, err := db.GetByEmail(alice.Email)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, alice.Name, user.Name)
	})

	t.Run("by secondary email", func(t *testing.T) {
		bob, err := db.Create("bob", "bob@example.com", CreateUserOpts{})
		if err != nil {
			t.Fatal(err)
		}

		// TODO: Use UserEmails.Create to replace SQL hack when the method is available.
		email2 := "bob2@exmaple.com"
		err = db.Exec(`INSERT INTO email_address (uid, email) VALUES (?, ?)`, bob.ID, email2).Error
		if err != nil {
			t.Fatal(err)
		}

		_, err = db.GetByEmail(email2)
		expErr := ErrUserNotExist{args: errutil.Args{"email": email2}}
		assert.Equal(t, expErr, err)

		// TODO: Use UserEmails.Verify to replace SQL hack when the method is available.
		err = db.Exec(`UPDATE email_address SET is_activated = ? WHERE email = ?`, true, email2).Error
		if err != nil {
			t.Fatal(err)
		}

		user, err := db.GetByEmail(email2)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, bob.Name, user.Name)
	})
}

func test_users_GetByID(t *testing.T, db *users) {
	alice, err := db.Create("alice", "alice@exmaple.com", CreateUserOpts{})
	if err != nil {
		t.Fatal(err)
	}

	user, err := db.GetByID(alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, alice.Name, user.Name)

	_, err = db.GetByID(404)
	expErr := ErrUserNotExist{args: errutil.Args{"userID": int64(404)}}
	assert.Equal(t, expErr, err)
}

func test_users_GetByUsername(t *testing.T, db *users) {
	alice, err := db.Create("alice", "alice@exmaple.com", CreateUserOpts{})
	if err != nil {
		t.Fatal(err)
	}

	user, err := db.GetByUsername(alice.Name)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, alice.Name, user.Name)

	_, err = db.GetByUsername("bad_username")
	expErr := ErrUserNotExist{args: errutil.Args{"name": "bad_username"}}
	assert.Equal(t, expErr, err)
}
