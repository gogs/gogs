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
	"gogs.io/gogs/internal/dbutil"
)

func TestOrgs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	tables := []any{new(User), new(EmailAddress), new(OrgUser)}
	db := &orgs{
		DB: dbtest.NewDB(t, "orgs", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, db *orgs)
	}{
		{"List", orgsList},
		{"SearchByName", orgsSearchByName},
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

func orgsList(t *testing.T, db *orgs) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org1, err := usersStore.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
	require.NoError(t, err)
	org2, err := usersStore.Create(ctx, "org2", "org2@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = db.Exec(
		dbutil.Quote("UPDATE %s SET type = ? WHERE id IN (?, ?)", "user"),
		UserTypeOrganization, org1.ID, org2.ID,
	).Error
	require.NoError(t, err)

	// TODO: Use OrgUsers.Join to replace SQL hack when the method is available.
	err = db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, alice.ID, org1.ID, false).Error
	require.NoError(t, err)
	err = db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, alice.ID, org2.ID, true).Error
	require.NoError(t, err)
	err = db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, bob.ID, org2.ID, true).Error
	require.NoError(t, err)

	tests := []struct {
		name         string
		opts         ListOrgsOptions
		wantOrgNames []string
	}{
		{
			name: "only public memberships for a user",
			opts: ListOrgsOptions{
				MemberID:              alice.ID,
				IncludePrivateMembers: false,
			},
			wantOrgNames: []string{org2.Name},
		},
		{
			name: "all memberships for a user",
			opts: ListOrgsOptions{
				MemberID:              alice.ID,
				IncludePrivateMembers: true,
			},
			wantOrgNames: []string{org1.Name, org2.Name},
		},
		{
			name: "no membership for a non-existent user",
			opts: ListOrgsOptions{
				MemberID:              404,
				IncludePrivateMembers: true,
			},
			wantOrgNames: []string{},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := db.List(ctx, test.opts)
			require.NoError(t, err)

			gotOrgNames := make([]string, len(got))
			for i := range got {
				gotOrgNames[i] = got[i].Name
			}
			assert.Equal(t, test.wantOrgNames, gotOrgNames)
		})
	}
}

func orgsSearchByName(t *testing.T, db *orgs) {
	ctx := context.Background()

	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	usersStore := NewUsersStore(db.DB)
	org1, err := usersStore.Create(ctx, "org1", "org1@example.com", CreateUserOptions{FullName: "Acme Corp"})
	require.NoError(t, err)
	org2, err := usersStore.Create(ctx, "org2", "org2@example.com", CreateUserOptions{FullName: "Acme Corp 2"})
	require.NoError(t, err)
	err = db.Exec(
		dbutil.Quote("UPDATE %s SET type = ? WHERE id IN (?, ?)", "user"),
		UserTypeOrganization, org1.ID, org2.ID,
	).Error
	require.NoError(t, err)

	t.Run("search for username org1", func(t *testing.T) {
		orgs, count, err := db.SearchByName(ctx, "G1", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, org1.ID, orgs[0].ID)
	})

	t.Run("search for username org2", func(t *testing.T) {
		orgs, count, err := db.SearchByName(ctx, "G2", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, org2.ID, orgs[0].ID)
	})

	t.Run("search for full name acme", func(t *testing.T) {
		orgs, count, err := db.SearchByName(ctx, "ACME", 1, 10, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(2), count)
	})

	t.Run("search for full name acme ORDER BY id DESC LIMIT 1", func(t *testing.T) {
		orgs, count, err := db.SearchByName(ctx, "ACME", 1, 1, "id DESC")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		assert.Equal(t, int64(2), count)
		assert.Equal(t, org2.ID, orgs[0].ID)
	})
}
