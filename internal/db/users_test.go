// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/auth"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/errutil"
	"gogs.io/gogs/internal/osutil"
	"gogs.io/gogs/internal/userutil"
	"gogs.io/gogs/public"
)

func TestUsers(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []interface{}{new(User), new(EmailAddress), new(Repository), new(Follow)}
	db := &users{
		DB: dbtest.NewDB(t, "users", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *users)
	}{
		{"Authenticate", usersAuthenticate},
		{"Create", usersCreate},
		{"DeleteCustomAvatar", usersDeleteCustomAvatar},
		{"GetByEmail", usersGetByEmail},
		{"GetByID", usersGetByID},
		{"GetByUsername", usersGetByUsername},
		{"HasForkedRepository", usersHasForkedRepository},
		{"ListFollowers", usersListFollowers},
		{"ListFollowings", usersListFollowings},
		{"UseCustomAvatar", usersUseCustomAvatar},
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

func usersAuthenticate(t *testing.T, db *users) {
	ctx := context.Background()

	password := "pa$$word"
	alice, err := db.Create(ctx, "alice", "alice@example.com",
		CreateUserOptions{
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

	t.Run("login source mismatch", func(t *testing.T) {
		_, err := db.Authenticate(ctx, alice.Email, password, 1)
		gotErr := fmt.Sprintf("%v", err)
		wantErr := ErrLoginSourceMismatch{args: map[string]interface{}{"actual": 0, "expect": 1}}.Error()
		assert.Equal(t, wantErr, gotErr)
	})

	t.Run("via login source", func(t *testing.T) {
		mockLoginSources := NewMockLoginSourcesStore()
		mockLoginSources.GetByIDFunc.SetDefaultHook(func(ctx context.Context, id int64) (*LoginSource, error) {
			mockProvider := NewMockProvider()
			mockProvider.AuthenticateFunc.SetDefaultReturn(&auth.ExternalAccount{}, nil)
			s := &LoginSource{
				IsActived: true,
				Provider:  mockProvider,
			}
			return s, nil
		})
		setMockLoginSourcesStore(t, mockLoginSources)

		bob, err := db.Create(ctx, "bob", "bob@example.com",
			CreateUserOptions{
				Password:    password,
				LoginSource: 1,
			},
		)
		require.NoError(t, err)

		user, err := db.Authenticate(ctx, bob.Email, password, 1)
		require.NoError(t, err)
		assert.Equal(t, bob.Name, user.Name)
	})

	t.Run("new user via login source", func(t *testing.T) {
		mockLoginSources := NewMockLoginSourcesStore()
		mockLoginSources.GetByIDFunc.SetDefaultHook(func(ctx context.Context, id int64) (*LoginSource, error) {
			mockProvider := NewMockProvider()
			mockProvider.AuthenticateFunc.SetDefaultReturn(
				&auth.ExternalAccount{
					Name:  "cindy",
					Email: "cindy@example.com",
				},
				nil,
			)
			s := &LoginSource{
				IsActived: true,
				Provider:  mockProvider,
			}
			return s, nil
		})
		setMockLoginSourcesStore(t, mockLoginSources)

		user, err := db.Authenticate(ctx, "cindy", password, 1)
		require.NoError(t, err)
		assert.Equal(t, "cindy", user.Name)

		user, err = db.GetByUsername(ctx, "cindy")
		require.NoError(t, err)
		assert.Equal(t, "cindy@example.com", user.Email)
	})
}

func usersCreate(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@example.com",
		CreateUserOptions{
			Activated: true,
		},
	)
	require.NoError(t, err)

	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.Create(ctx, "-", "", CreateUserOptions{})
		wantErr := ErrNameNotAllowed{args: errutil.Args{"reason": "reserved", "name": "-"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("name already exists", func(t *testing.T) {
		_, err := db.Create(ctx, alice.Name, "", CreateUserOptions{})
		wantErr := ErrUserAlreadyExist{args: errutil.Args{"name": alice.Name}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("email already exists", func(t *testing.T) {
		_, err := db.Create(ctx, "bob", alice.Email, CreateUserOptions{})
		wantErr := ErrEmailAlreadyUsed{args: errutil.Args{"email": alice.Email}}
		assert.Equal(t, wantErr, err)
	})

	user, err := db.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), user.Updated.UTC().Format(time.RFC3339))
}

func usersDeleteCustomAvatar(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	avatar, err := public.Files.ReadFile("img/avatar_default.png")
	require.NoError(t, err)

	avatarPath := userutil.CustomAvatarPath(alice.ID)
	_ = os.Remove(avatarPath)
	defer func() { _ = os.Remove(avatarPath) }()

	err = db.UseCustomAvatar(ctx, alice.ID, avatar)
	require.NoError(t, err)

	// Make sure avatar is saved and the user flag is updated.
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)

	alice, err = db.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, alice.UseCustomAvatar)

	// Delete avatar should remove the file and revert the user flag.
	err = db.DeleteCustomAvatar(ctx, alice.ID)
	require.NoError(t, err)

	got = osutil.IsFile(avatarPath)
	assert.False(t, got)

	alice, err = db.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.False(t, alice.UseCustomAvatar)
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
		org, err := db.Create(ctx, "gogs", "gogs@exmaple.com", CreateUserOptions{})
		require.NoError(t, err)

		err = db.Model(&User{}).Where("id", org.ID).UpdateColumn("type", UserTypeOrganization).Error
		require.NoError(t, err)

		_, err = db.GetByEmail(ctx, org.Email)
		wantErr := ErrUserNotExist{args: errutil.Args{"email": org.Email}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("by primary email", func(t *testing.T) {
		alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
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
		bob, err := db.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
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

	alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
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

	alice, err := db.Create(ctx, "alice", "alice@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	user, err := db.GetByUsername(ctx, alice.Name)
	require.NoError(t, err)
	assert.Equal(t, alice.Name, user.Name)

	_, err = db.GetByUsername(ctx, "bad_username")
	wantErr := ErrUserNotExist{args: errutil.Args{"name": "bad_username"}}
	assert.Equal(t, wantErr, err)
}

func usersHasForkedRepository(t *testing.T, db *users) {
	ctx := context.Background()

	has := db.HasForkedRepository(ctx, 1, 1)
	assert.False(t, has)

	_, err := NewReposStore(db.DB).Create(
		ctx,
		1,
		CreateRepoOptions{
			Name:   "repo1",
			ForkID: 1,
		},
	)
	require.NoError(t, err)

	has = db.HasForkedRepository(ctx, 1, 1)
	assert.True(t, has)
}

func usersListFollowers(t *testing.T, db *users) {
	ctx := context.Background()

	john, err := db.Create(ctx, "john", "john@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got, err := db.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	assert.Empty(t, got)

	alice, err := db.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := db.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	followsStore := NewFollowsStore(db.DB)
	err = followsStore.Follow(ctx, alice.ID, john.ID)
	require.NoError(t, err)
	err = followsStore.Follow(ctx, bob.ID, john.ID)
	require.NoError(t, err)

	// First page only has bob
	got, err = db.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bob.ID, got[0].ID)

	// Second page only has alice
	got, err = db.ListFollowers(ctx, john.ID, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, alice.ID, got[0].ID)
}

func usersListFollowings(t *testing.T, db *users) {
	ctx := context.Background()

	john, err := db.Create(ctx, "john", "john@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got, err := db.ListFollowers(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	assert.Empty(t, got)

	alice, err := db.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := db.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	followsStore := NewFollowsStore(db.DB)
	err = followsStore.Follow(ctx, john.ID, alice.ID)
	require.NoError(t, err)
	err = followsStore.Follow(ctx, john.ID, bob.ID)
	require.NoError(t, err)

	// First page only has bob
	got, err = db.ListFollowings(ctx, john.ID, 1, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, bob.ID, got[0].ID)

	// Second page only has alice
	got, err = db.ListFollowings(ctx, john.ID, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, alice.ID, got[0].ID)
}

func usersUseCustomAvatar(t *testing.T, db *users) {
	ctx := context.Background()

	alice, err := db.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	avatar, err := public.Files.ReadFile("img/avatar_default.png")
	require.NoError(t, err)

	avatarPath := userutil.CustomAvatarPath(alice.ID)
	_ = os.Remove(avatarPath)
	defer func() { _ = os.Remove(avatarPath) }()

	err = db.UseCustomAvatar(ctx, alice.ID, avatar)
	require.NoError(t, err)

	// Make sure avatar is saved and the user flag is updated.
	got := osutil.IsFile(avatarPath)
	assert.True(t, got)

	alice, err = db.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.True(t, alice.UseCustomAvatar)
}
