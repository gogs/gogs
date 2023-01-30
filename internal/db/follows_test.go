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
)

func TestFollows(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []interface{}{new(User), new(EmailAddress), new(Follow)}
	db := &follows{
		DB: dbtest.NewDB(t, "follows", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *follows)
	}{
		{"Follow", followsFollow},
		{"IsFollowing", followsIsFollowing},
		{"Unfollow", followsUnfollow},
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

func followsFollow(t *testing.T, db *follows) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = db.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	// It is OK to follow multiple times and just be noop.
	err = db.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	alice, err = usersStore.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, alice.NumFollowing)

	bob, err = usersStore.GetByID(ctx, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, bob.NumFollowers)
}

func followsIsFollowing(t *testing.T, db *follows) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	got := db.IsFollowing(ctx, alice.ID, bob.ID)
	assert.False(t, got)

	err = db.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	got = db.IsFollowing(ctx, alice.ID, bob.ID)
	assert.True(t, got)

	err = db.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	got = db.IsFollowing(ctx, alice.ID, bob.ID)
	assert.False(t, got)
}

func followsUnfollow(t *testing.T, db *follows) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	err = db.Follow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	// It is OK to unfollow multiple times and just be noop.
	err = db.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)
	err = db.Unfollow(ctx, alice.ID, bob.ID)
	require.NoError(t, err)

	alice, err = usersStore.GetByID(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, alice.NumFollowing)

	bob, err = usersStore.GetByID(ctx, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, 0, bob.NumFollowers)
}
