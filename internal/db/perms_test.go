// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_perms(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(Access)}
	db := &perms{
		DB: initTestDB(t, "perms", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(*testing.T, *perms)
	}{
		{"AccessMode", test_perms_AccessMode},
		{"Authorize", test_perms_Authorize},
		{"SetRepoPerms", test_perms_SetRepoPerms},
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
func test_perms_AccessMode(t *testing.T, db *perms) {
	// Set up permissions
	err := db.SetRepoPerms(1, map[int64]AccessMode{
		2: AccessModeWrite,
		3: AccessModeAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.SetRepoPerms(2, map[int64]AccessMode{
		1: AccessModeRead,
	})
	if err != nil {
		t.Fatal(err)
	}

	publicRepoID := int64(1)
	publicRepoOpts := AccessModeOptions{
		OwnerID: 98,
	}

	privateRepoID := int64(2)
	privateRepoOpts := AccessModeOptions{
		OwnerID: 99,
		Private: true,
	}

	tests := []struct {
		name          string
		userID        int64
		repoID        int64
		opts          AccessModeOptions
		expAccessMode AccessMode
	}{
		{
			name:          "nil repository",
			expAccessMode: AccessModeNone,
		},

		{
			name:          "anonymous user has read access to public repository",
			repoID:        publicRepoID,
			opts:          publicRepoOpts,
			expAccessMode: AccessModeRead,
		},
		{
			name:          "anonymous user has no access to private repository",
			repoID:        privateRepoID,
			opts:          privateRepoOpts,
			expAccessMode: AccessModeNone,
		},

		{
			name:          "user is the owner",
			userID:        98,
			repoID:        publicRepoID,
			opts:          publicRepoOpts,
			expAccessMode: AccessModeOwner,
		},
		{
			name:          "user 1 has read access to public repo",
			userID:        1,
			repoID:        publicRepoID,
			opts:          publicRepoOpts,
			expAccessMode: AccessModeRead,
		},
		{
			name:          "user 2 has write access to public repo",
			userID:        2,
			repoID:        publicRepoID,
			opts:          publicRepoOpts,
			expAccessMode: AccessModeWrite,
		},
		{
			name:          "user 3 has admin access to public repo",
			userID:        3,
			repoID:        publicRepoID,
			opts:          publicRepoOpts,
			expAccessMode: AccessModeAdmin,
		},

		{
			name:          "user 1 has read access to private repo",
			userID:        1,
			repoID:        privateRepoID,
			opts:          privateRepoOpts,
			expAccessMode: AccessModeRead,
		},
		{
			name:          "user 2 has no access to private repo",
			userID:        2,
			repoID:        privateRepoID,
			opts:          privateRepoOpts,
			expAccessMode: AccessModeNone,
		},
		{
			name:          "user 3 has no access to private repo",
			userID:        3,
			repoID:        privateRepoID,
			opts:          privateRepoOpts,
			expAccessMode: AccessModeNone,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode := db.AccessMode(test.userID, test.repoID, test.opts)
			assert.Equal(t, test.expAccessMode, mode)
		})
	}
}

func test_perms_Authorize(t *testing.T, db *perms) {
	// Set up permissions
	err := db.SetRepoPerms(1, map[int64]AccessMode{
		1: AccessModeRead,
		2: AccessModeWrite,
		3: AccessModeAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}

	repo := &Repository{
		ID:      1,
		OwnerID: 98,
	}

	tests := []struct {
		name          string
		userID        int64
		desired       AccessMode
		expAuthorized bool
	}{
		{
			name:          "user 1 has read and wants read",
			userID:        1,
			desired:       AccessModeRead,
			expAuthorized: true,
		},
		{
			name:          "user 1 has read and wants write",
			userID:        1,
			desired:       AccessModeWrite,
			expAuthorized: false,
		},

		{
			name:          "user 2 has write and wants read",
			userID:        2,
			desired:       AccessModeRead,
			expAuthorized: true,
		},
		{
			name:          "user 2 has write and wants write",
			userID:        2,
			desired:       AccessModeWrite,
			expAuthorized: true,
		},
		{
			name:          "user 2 has write and wants admin",
			userID:        2,
			desired:       AccessModeAdmin,
			expAuthorized: false,
		},

		{
			name:          "user 3 has admin and wants read",
			userID:        3,
			desired:       AccessModeRead,
			expAuthorized: true,
		},
		{
			name:          "user 3 has admin and wants write",
			userID:        3,
			desired:       AccessModeWrite,
			expAuthorized: true,
		},
		{
			name:          "user 3 has admin and wants admin",
			userID:        3,
			desired:       AccessModeAdmin,
			expAuthorized: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorized := db.Authorize(test.userID, repo.ID, test.desired, AccessModeOptions{
				OwnerID: repo.OwnerID,
				Private: repo.IsPrivate,
			})
			assert.Equal(t, test.expAuthorized, authorized)
		})
	}
}

func test_perms_SetRepoPerms(t *testing.T, db *perms) {
	for _, update := range []struct {
		repoID    int64
		accessMap map[int64]AccessMode
	}{
		{
			repoID: 1,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeWrite,
				3: AccessModeAdmin,
				4: AccessModeWrite,
			},
		},
		{
			repoID: 2,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeRead,
				4: AccessModeWrite,
				5: AccessModeWrite,
			},
		},
		{
			repoID: 1,
			accessMap: map[int64]AccessMode{
				2: AccessModeWrite,
				3: AccessModeAdmin,
			},
		},
		{
			repoID: 2,
			accessMap: map[int64]AccessMode{
				1: AccessModeWrite,
				2: AccessModeRead,
				5: AccessModeWrite,
			},
		},
	} {
		err := db.SetRepoPerms(update.repoID, update.accessMap)
		if err != nil {
			t.Fatal(err)
		}
	}

	var accesses []*Access
	err := db.Order("user_id, repo_id").Find(&accesses).Error
	if err != nil {
		t.Fatal(err)
	}

	// Ignore ID fields
	for _, a := range accesses {
		a.ID = 0
	}

	expAccesses := []*Access{
		{UserID: 1, RepoID: 2, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 1, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 2, Mode: AccessModeRead},
		{UserID: 3, RepoID: 1, Mode: AccessModeAdmin},
		{UserID: 5, RepoID: 2, Mode: AccessModeWrite},
	}
	assert.Equal(t, expAccesses, accesses)
}
