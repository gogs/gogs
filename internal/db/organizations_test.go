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
	tables := []any{
		new(User), new(EmailAddress), new(OrgUser), new(Team), new(TeamUser), new(Repository), new(Watch), new(Star),
		new(Follow), new(Issue), new(PublicKey), new(AccessToken), new(Collaboration), new(Access), new(Action),
		new(IssueUser), new(TeamRepo),
	}
	db := &organizations{
		DB: dbtest.NewDB(t, "orgs", tables...),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, db *organizations)
	}{
		{"Create", orgsCreate},
		{"GetByName", orgsGetByName},
		{"SearchByName", orgsSearchByName},
		{"List", orgsList},
		{"CountByUser", orgsCountByUser},
		{"Count", orgsCount},
		{"DeleteByID", orgsDeleteByID},
		{"AddMember", orgsAddMember},
		{"RemoveMember", orgsRemoveMember},
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

func orgsGetByName(t *testing.T, ctx context.Context, db *organizations) {
	t.Run("correct user type", func(t *testing.T) {
		tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "usersGetByUsername-tempPictureAvatarUploadPath")
		conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

		org1, err := db.Create(ctx, "org1", 1, CreateOrganizationOptions{})
		require.NoError(t, err)

		got, err := db.GetByName(ctx, org1.Name)
		require.NoError(t, err)
		assert.Equal(t, org1.Name, got.Name)

		_, err = db.GetByName(ctx, "bad_name")
		wantErr := ErrOrganizationNotExist{args: errutil.Args{"name": "bad_name"}}
		assert.Equal(t, wantErr, err)
	})

	t.Run("wrong user type", func(t *testing.T) {
		alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
		require.NoError(t, err)

		_, err = db.GetByName(ctx, alice.Name)
		wantErr := ErrOrganizationNotExist{args: errutil.Args{"name": alice.Name}}
		assert.Equal(t, wantErr, err)
	})
}

func orgsList(t *testing.T, ctx context.Context, db *organizations) {
	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsList-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1-alice-owned", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)

	org2, err := db.Create(ctx, "org2-alice-owned", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)
	err = db.SetMemberVisibility(ctx, org2.ID, alice.ID, true)
	require.NoError(t, err)
	err = db.AddMember(ctx, org2.ID, bob.ID)
	require.NoError(t, err)

	org3, err := db.Create(ctx, "org3-bob-owned", bob.ID, CreateOrganizationOptions{})
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
			name: "only public ownership for a user",
			opts: ListOrganizationsOptions{
				OwnerID:               alice.ID,
				IncludePrivateMembers: false,
			},
			wantOrgNames: []string{org2.Name},
		},
		{
			name: "all ownership for a user",
			opts: ListOrganizationsOptions{
				OwnerID:               alice.ID,
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

	t.Run("pagination", func(t *testing.T) {
		got, err := db.List(ctx, ListOrganizationsOptions{Page: 1, PageSize: 1})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, org1.ID, got[0].ID)

		got, err = db.List(ctx, ListOrganizationsOptions{Page: 2, PageSize: 1})
		require.NoError(t, err)
		require.Len(t, got, 1)
		assert.Equal(t, org2.ID, got[0].ID)

		got, err = db.List(ctx, ListOrganizationsOptions{Page: 1, PageSize: 4})
		require.NoError(t, err)
		require.Len(t, got, 3)
		assert.Equal(t, org1.ID, got[0].ID)
		assert.Equal(t, org2.ID, got[1].ID)
		assert.Equal(t, org3.ID, got[2].ID)
	})
}

func orgsSearchByName(t *testing.T, ctx context.Context, db *organizations) {
	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsSearchByName-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})
	tempRepositoryRoot := filepath.Join(os.TempDir(), "orgsSearchByName-tempRepositoryRoot")
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: tempRepositoryRoot})

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

func orgsCount(t *testing.T, db *organizations) {
	ctx := context.Background()

	// Has no organization initially
	got := db.Count(ctx)
	assert.Equal(t, int64(0), got)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "usersCount-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	_, err := db.Create(ctx, "org1", 1, CreateOrganizationOptions{})
	require.NoError(t, err)

	// Create a user shouldn't count
	_, err = NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	got = db.Count(ctx)
	assert.Equal(t, int64(1), got)
}

func orgsDeleteByID(t *testing.T, db *organizations) {
	ctx := context.Background()

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsDeleteByID-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	t.Run("organization still has repository ownership", func(t *testing.T) {
		org1, err := db.Create(ctx, "org1", 404, CreateOrganizationOptions{})
		require.NoError(t, err)

		_, err = NewRepositoriesStore(db.DB).Create(ctx, org1.ID, CreateRepoOptions{Name: "repo1"})
		require.NoError(t, err)

		err = db.DeleteByID(ctx, org1.ID)
		wantErr := ErrOrganizationOwnRepos{errutil.Args{"orgID": org1.ID}}
		assert.Equal(t, wantErr, err)
	})

	alice, err := NewUsersStore(db.DB).Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	org2, err := db.Create(ctx, "org2", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)

	// Mock team membership
	// TODO: Use Organizations.CreateTeam to replace SQL hack when the method is available.
	team1 := &Team{
		OrgID:      org2.ID,
		LowerName:  "team1",
		Name:       "team1",
		NumMembers: 1,
	}
	err = db.DB.Create(team1).Error
	require.NoError(t, err)
	// TODO: Use Organizations.AddTeamMember to replace SQL hack when the method is available.
	err = db.DB.Create(
		&TeamUser{
			OrgID:  org2.ID,
			TeamID: team1.ID,
			UID:    alice.ID,
		},
	).Error
	require.NoError(t, err)

	// Pull the trigger
	err = db.DeleteByID(ctx, org2.ID)
	require.NoError(t, err)

	// Verify after-the-fact data
	for _, table := range []any{
		&Team{OrgID: org2.ID},
		&TeamUser{OrgID: org2.ID},
	} {
		var count int64
		err = db.DB.Model(table).Where(table).Count(&count).Error
		require.NoError(t, err, "table for %T", table)
		assert.Equal(t, int64(0), count, "table for %T", table)
	}

	_, err = db.GetByName(ctx, org2.Name)
	wantErr := ErrOrganizationNotExist{errutil.Args{"name": org2.Name}}
	assert.Equal(t, wantErr, err)
}

func orgsAddMember(t *testing.T, db *organizations) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsAddMember-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)

	// Not yet a member
	gotOrgs, err := db.List(ctx, ListOrganizationsOptions{MemberID: bob.ID, IncludePrivateMembers: true})
	require.NoError(t, err)
	assert.Len(t, gotOrgs, 0)

	// Add member
	err = db.AddMember(ctx, org1.ID, bob.ID)
	require.NoError(t, err)

	// Now a member
	gotOrgs, err = db.List(ctx, ListOrganizationsOptions{MemberID: bob.ID, IncludePrivateMembers: true})
	require.NoError(t, err)
	assert.Len(t, gotOrgs, 1)
	assert.Equal(t, org1.ID, gotOrgs[0].ID)

	// Add member again shouldn't fail
	err = db.AddMember(ctx, org1.ID, bob.ID)
	require.NoError(t, err)

	gotOrg, err := db.GetByName(ctx, org1.Name)
	require.NoError(t, err)
	assert.Equal(t, 2, gotOrg.NumMembers)
}

func orgsRemoveMember(t *testing.T, db *organizations) {
	ctx := context.Background()

	usersStore := NewUsersStore(db.DB)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@exmaple.com", CreateUserOptions{})
	require.NoError(t, err)

	tempPictureAvatarUploadPath := filepath.Join(os.TempDir(), "orgsRemoveMember-tempPictureAvatarUploadPath")
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: tempPictureAvatarUploadPath})

	org1, err := db.Create(ctx, "org1", alice.ID, CreateOrganizationOptions{})
	require.NoError(t, err)

	t.Run("remove non-existent member", func(t *testing.T) {
		err = db.RemoveMember(ctx, org1.ID, bob.ID)
		require.NoError(t, err)
	})

	t.Run("remove last owner", func(t *testing.T) {
		err = db.RemoveMember(ctx, org1.ID, alice.ID)
		wantErr := ErrLastOrgOwner{errutil.Args{"orgID": org1.ID, "userID": alice.ID}}
		assert.Equal(t, wantErr, err)
	})

	err = db.AddMember(ctx, org1.ID, bob.ID)
	require.NoError(t, err)

	// Mock repository, watches, accesses and collaborations
	reposStore := NewRepositoriesStore(db.DB)
	repo1, err := reposStore.Create(ctx, org1.ID, CreateRepoOptions{Name: "repo1", Private: true})
	require.NoError(t, err)
	// TODO: Use Repositories.AddCollaborator to replace SQL hack when the method is available.
	err = db.DB.Create(
		&Collaboration{
			UserID: bob.ID,
			RepoID: repo1.ID,
			Mode:   AccessModeRead,
		},
	).Error
	require.NoError(t, err)

	// Mock team membership
	// TODO: Use Organizations.CreateTeam to replace SQL hack when the method is available.
	team1 := &Team{
		OrgID:      org1.ID,
		LowerName:  "team1",
		Name:       "team1",
		NumMembers: 1,
	}
	err = db.DB.Create(team1).Error
	require.NoError(t, err)
	// TODO: Use Organizations.AddTeamMember to replace SQL hack when the method is available.
	err = db.DB.Create(
		&TeamUser{
			OrgID:  org1.ID,
			TeamID: team1.ID,
			UID:    bob.ID,
		},
	).Error
	require.NoError(t, err)
	// TODO: Use Organizations.AddTeamRepository to replace SQL hack when the method is available.
	err = db.DB.Create(
		&TeamRepo{
			OrgID:  org1.ID,
			TeamID: team1.ID,
			RepoID: repo1.ID,
		},
	).Error
	require.NoError(t, err)

	permsStore := NewPermsStore(db.DB)
	err = permsStore.SetRepoPerms(ctx, repo1.ID, map[int64]AccessMode{bob.ID: AccessModeRead})
	require.NoError(t, err)
	err = reposStore.Watch(
		ctx,
		WatchRepositoryOptions{
			UserID:        bob.ID,
			RepoID:        repo1.ID,
			RepoOwnerID:   repo1.OwnerID,
			RepoIsPrivate: repo1.IsPrivate,
		},
	)
	require.NoError(t, err)

	// Pull the trigger
	err = db.RemoveMember(ctx, org1.ID, bob.ID)
	require.NoError(t, err)

	// Verify after-the-fact data
	gotRepo, err := reposStore.GetByID(ctx, repo1.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, gotRepo.NumWatches)

	gotAccessMode := permsStore.AccessMode(ctx, repo1.ID, bob.ID, AccessModeOptions{Private: repo1.IsPrivate})
	assert.Equal(t, AccessModeNone, gotAccessMode)

	// TODO: Use Repositories.ListCollaborators to replace SQL hack when the method is available.
	var count int64
	err = db.DB.Model(&Collaboration{}).Where(&Collaboration{RepoID: repo1.ID}).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)

	gotTeam, err := db.GetTeamByName(ctx, org1.ID, team1.Name)
	require.NoError(t, err)
	assert.Equal(t, 0, gotTeam.NumMembers)

	gotOrg, err := db.GetByName(ctx, org1.Name)
	require.NoError(t, err)
	assert.Equal(t, 1, gotOrg.NumMembers)
}
