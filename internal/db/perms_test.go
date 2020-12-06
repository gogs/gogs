// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPerms(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	t.Parallel()

	tables := []interface{}{new(Access)}
	db, cleanup := newTestDB(t, "perms", tables...)
	store := NewPermsStore(db)

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *perms)
	}{
		{"AccessMode", testPermsAccessMode},
		{"Authorize", testPermsAuthorize},
		{"SetRepoPerms", testPermsSetRepoPerms},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := cleanup()
				if err != nil {
					t.Fatal(err)
				}
			})
			tc.test(t, context.Background(), store.(*perms))
		})
	}
}
func testPermsAccessMode(t *testing.T, ctx context.Context, db *perms) {
	// Set up permissions
	err := db.SetRepoPerms(ctx,
		1,
		map[int64]AccessMode{
			2: AccessModeWrite,
			3: AccessModeAdmin,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	err = db.SetRepoPerms(ctx,
		2,
		map[int64]AccessMode{
			1: AccessModeRead,
		},
	)
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
		name   string
		userID int64
		repoID int64
		opts   AccessModeOptions
		want   AccessMode
	}{
		{
			name: "nil repository",
			want: AccessModeNone,
		},

		{
			name:   "anonymous user has read access to public repository",
			repoID: publicRepoID,
			opts:   publicRepoOpts,
			want:   AccessModeRead,
		},
		{
			name:   "anonymous user has no access to private repository",
			repoID: privateRepoID,
			opts:   privateRepoOpts,
			want:   AccessModeNone,
		},

		{
			name:   "user is the owner",
			userID: 98,
			repoID: publicRepoID,
			opts:   publicRepoOpts,
			want:   AccessModeOwner,
		},
		{
			name:   "user 1 has read access to public repo",
			userID: 1,
			repoID: publicRepoID,
			opts:   publicRepoOpts,
			want:   AccessModeRead,
		},
		{
			name:   "user 2 has write access to public repo",
			userID: 2,
			repoID: publicRepoID,
			opts:   publicRepoOpts,
			want:   AccessModeWrite,
		},
		{
			name:   "user 3 has admin access to public repo",
			userID: 3,
			repoID: publicRepoID,
			opts:   publicRepoOpts,
			want:   AccessModeAdmin,
		},

		{
			name:   "user 1 has read access to private repo",
			userID: 1,
			repoID: privateRepoID,
			opts:   privateRepoOpts,
			want:   AccessModeRead,
		},
		{
			name:   "user 2 has no access to private repo",
			userID: 2,
			repoID: privateRepoID,
			opts:   privateRepoOpts,
			want:   AccessModeNone,
		},
		{
			name:   "user 3 has no access to private repo",
			userID: 3,
			repoID: privateRepoID,
			opts:   privateRepoOpts,
			want:   AccessModeNone,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mode := db.AccessMode(ctx, test.userID, test.repoID, test.opts)
			assert.Equal(t, test.want, mode)
		})
	}
}

func testPermsAuthorize(t *testing.T, ctx context.Context, db *perms) {
	// Set up permissions
	err := db.SetRepoPerms(ctx,
		1,
		map[int64]AccessMode{
			1: AccessModeRead,
			2: AccessModeWrite,
			3: AccessModeAdmin,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	repo := &Repository{
		ID:      1,
		OwnerID: 98,
	}

	tests := []struct {
		name    string
		userID  int64
		desired AccessMode
		want    bool
	}{
		{
			name:    "user 1 has read and wants read",
			userID:  1,
			desired: AccessModeRead,
			want:    true,
		},
		{
			name:    "user 1 has read and wants write",
			userID:  1,
			desired: AccessModeWrite,
			want:    false,
		},

		{
			name:    "user 2 has write and wants read",
			userID:  2,
			desired: AccessModeRead,
			want:    true,
		},
		{
			name:    "user 2 has write and wants write",
			userID:  2,
			desired: AccessModeWrite,
			want:    true,
		},
		{
			name:    "user 2 has write and wants admin",
			userID:  2,
			desired: AccessModeAdmin,
			want:    false,
		},

		{
			name:    "user 3 has admin and wants read",
			userID:  3,
			desired: AccessModeRead,
			want:    true,
		},
		{
			name:    "user 3 has admin and wants write",
			userID:  3,
			desired: AccessModeWrite,
			want:    true,
		},
		{
			name:    "user 3 has admin and wants admin",
			userID:  3,
			desired: AccessModeAdmin,
			want:    true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			authorized := db.Authorize(ctx,
				test.userID,
				repo.ID,
				test.desired,
				AccessModeOptions{
					OwnerID: repo.OwnerID,
					Private: repo.IsPrivate,
				},
			)
			assert.Equal(t, test.want, authorized)
		})
	}
}

func testPermsSetRepoPerms(t *testing.T, ctx context.Context, db *perms) {
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
		err := db.SetRepoPerms(ctx, update.repoID, update.accessMap)
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

	want := []*Access{
		{UserID: 1, RepoID: 2, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 1, Mode: AccessModeWrite},
		{UserID: 2, RepoID: 2, Mode: AccessModeRead},
		{UserID: 3, RepoID: 1, Mode: AccessModeAdmin},
		{UserID: 5, RepoID: 2, Mode: AccessModeWrite},
	}
	assert.Equal(t, want, accesses)
}
