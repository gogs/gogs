// Copyright 2022 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/errutil"
)

func TestOrganizations(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	tables := []any{new(User), new(EmailAddress), new(OrgUser), new(Team), new(TeamUser)}
	db := &organizations{
		DB: dbtest.NewDB(t, "orgs", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *organizations)
	}{
		{"Create", orgsCreate},
		{"List", orgsList},
		{"SearchByName", orgsSearchByName},
		{"CountByUser", orgsCountByUser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, db.DB, tables...)
				require.NoError(t, err)
			})
			tc.test(t, ctx, db)
		})
		if t.Failed() {
			break
		}
	}
}

func orgsCreate(t *testing.T, ctx context.Context, db *organizations) {
	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)

	t.Run("name not allowed", func(t *testing.T) {
		_, err := db.Create(ctx, "-", alice.ID, CreateOrganizationOptions{})
		wantErr := ErrNameNotAllowed{
			args: errutil.Args{
				"reason": "reserved",
				"name":   "-",
			},
		}
		assert.Equal(t, wantErr, err)
	})

	// Users and organizations share the same namespace for names.
	t.Run("name already exists", func(t *testing.T) {
		_, err := db.Create(ctx, alice.Name, alice.ID, CreateOrganizationOptions{})
		wantErr := ErrOrganizationAlreadyExist{
			args: errutil.Args{
				"name": alice.Name,
			},
		}
		assert.Equal(t, wantErr, err)
	})

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsCreate-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org, err := db.Create(
		ctx,
		"acme",
		alice.ID,
		CreateOrganizationOptions{
			FullName:    "Acme Corp",
			Email:       "admin@acme.com",
			Location:    "Earth",
			Website:     "acme.com",
			Description: "A popcorn company",
		},
	)
	require.NoError(t, err)

	got, err := db.GetByName(ctx, org.Name)
	require.NoError(t, err)
	assert.Equal(t, org.Name, got.Name)
	assert.Equal(t, org.FullName, got.FullName)
	assert.Equal(t, org.Email, got.Email)
	assert.Equal(t, org.Location, got.Location)
	assert.Equal(t, org.Website, got.Website)
	assert.Equal(t, org.Description, got.Description)
	assert.Equal(t, -1, got.MaxRepoCreation)
	assert.Equal(t, 1, got.NumTeams)
	assert.Equal(t, 1, got.NumMembers)
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), got.Created.UTC().Format(time.RFC3339))
	assert.Equal(t, db.NowFunc().Format(time.RFC3339), got.Updated.UTC().Format(time.RFC3339))
}

func orgsList(t *testing.T, ctx context.Context, db *organizations) {
	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsList-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)
	org2, err := db.Create(ctx, "org2", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)
	err = db.SetMemberVisibility(ctx, org2.ID, alice.ID, true)
	require.NoError(t, err)
	err = db.AddMember(ctx, org2.ID, bob.ID)
	require.NoError(t, err)
	err = db.SetMemberVisibility(ctx, org2.ID, alice.ID, true)
	require.NoError(t, err)

	tests := []struct {
		name         string
		opts         ListOrganizationsOptions
		wantOrgNames []string
	}{
		{
			name: "only public memberships for a user",
			opts: ListOrganizationsOptions{
				MemberID:              alice.ID,
				IncludePrivateMembers: false,
			},
			wantOrgNames: []string{org2.Name},
		},
		{
			name: "all memberships for a user",
			opts: ListOrganizationsOptions{
				MemberID:              alice.ID,
				IncludePrivateMembers: true,
			},
			wantOrgNames: []string{org1.Name, org2.Name},
		},
		{
			name: "no membership for a non-existent user",
			opts: ListOrganizationsOptions{
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

func orgsSearchByName(t *testing.T, ctx context.Context, db *organizations) {
	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsSearchByName-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1", 1, CreateOrganizationOptions{FullName: "Acme Corp"})
	require.NoError(t, err)
	org2, err := db.Create(ctx, "org2", 1, CreateOrganizationOptions{FullName: "Acme Corp 2"})
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

func orgsCountByUser(t *testing.T, ctx context.Context, db *organizations) {
	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsCountByUser-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)
	err = db.AddMember(ctx, org1.ID, bob.ID)
	require.NoError(t, err)

	got, err := db.CountByUser(ctx, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)

	got, err = db.CountByUser(ctx, 404)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)
}
