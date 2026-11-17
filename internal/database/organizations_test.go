package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/osx"
	"gogs.io/gogs/internal/repox"
)

func TestOrgs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// NOTE: The suite does not run in parallel because it mocks global
	// configuration that other suites read without locks.

	ctx := context.Background()
	s := &OrganizationsStore{
		db: newTestDB(t, "OrganizationsStore"),
	}
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: t.TempDir()})

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *OrganizationsStore)
	}{
		{"List", orgsList},
		{"SearchByName", organizationsSearchByName},
		{"CountByUser", organizationsCountByUser},
		{"Create", organizationsCreate},
		{"GetByName", organizationsGetByName},
		{"ListByPage", organizationsListByPage},
		{"Delete", organizationsDelete},
		{"Members", organizationsMembers},
		{"ListOwnedBy", organizationsListOwnedBy},
		{"RemoveMember", organizationsRemoveMember},
		{"ListAccessibleRepos", organizationsListAccessibleRepos},
		{"ListAccessibleMirrors", organizationsListAccessibleMirrors},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() {
				err := clearTables(t, s.db)
				require.NoError(t, err)
			})
			tc.test(t, ctx, s)
		})
		if t.Failed() {
			break
		}
	}
}

func orgsList(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "alice", "alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "bob", "bob@example.com", CreateUserOptions{})
	require.NoError(t, err)

	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org1, err := usersStore.Create(ctx, "org1", "org1@example.com", CreateUserOptions{})
	require.NoError(t, err)
	org2, err := usersStore.Create(ctx, "org2", "org2@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(
		dbx.Quote("UPDATE %s SET type = ? WHERE id IN (?, ?)", "user"),
		UserTypeOrganization, org1.ID, org2.ID,
	).Error
	require.NoError(t, err)

	// TODO: Use Orgs.Join to replace SQL hack when the method is available.
	err = s.db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, alice.ID, org1.ID, false).Error
	require.NoError(t, err)
	err = s.db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, alice.ID, org2.ID, true).Error
	require.NoError(t, err)
	err = s.db.Exec(`INSERT INTO org_user (uid, org_id, is_public) VALUES (?, ?, ?)`, bob.ID, org2.ID, true).Error
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
			got, err := s.List(ctx, test.opts)
			require.NoError(t, err)

			gotOrgNames := make([]string, len(got))
			for i := range got {
				gotOrgNames[i] = got[i].Name
			}
			assert.Equal(t, test.wantOrgNames, gotOrgNames)
		})
	}
}

func organizationsSearchByName(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	usersStore := newUsersStore(s.db)
	org1, err := usersStore.Create(ctx, "org1", "org1@example.com", CreateUserOptions{FullName: "Acme Corp"})
	require.NoError(t, err)
	org2, err := usersStore.Create(ctx, "org2", "org2@example.com", CreateUserOptions{FullName: "Acme Corp 2"})
	require.NoError(t, err)
	err = s.db.Exec(
		dbx.Quote("UPDATE %s SET type = ? WHERE id IN (?, ?)", "user"),
		UserTypeOrganization, org1.ID, org2.ID,
	).Error
	require.NoError(t, err)

	t.Run("search for username org1", func(t *testing.T) {
		orgs, count, err := s.SearchByName(ctx, "G1", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, org1.ID, orgs[0].ID)
	})

	t.Run("search for username org2", func(t *testing.T) {
		orgs, count, err := s.SearchByName(ctx, "G2", 1, 1, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(1), count)
		assert.Equal(t, org2.ID, orgs[0].ID)
	})

	t.Run("search for full name acme", func(t *testing.T) {
		orgs, count, err := s.SearchByName(ctx, "ACME", 1, 10, "")
		require.NoError(t, err)
		require.Len(t, orgs, int(count))
		assert.Equal(t, int64(2), count)
	})

	t.Run("search for full name acme ORDER BY id DESC LIMIT 1", func(t *testing.T) {
		orgs, count, err := s.SearchByName(ctx, "ACME", 1, 1, "id DESC")
		require.NoError(t, err)
		require.Len(t, orgs, 1)
		assert.Equal(t, int64(2), count)
		assert.Equal(t, org2.ID, orgs[0].ID)
	})
}

func organizationsCountByUser(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	// TODO: Use Orgs.Join to replace SQL hack when the method is available.
	err := s.db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, 1, 1).Error
	require.NoError(t, err)
	err = s.db.Exec(`INSERT INTO org_user (uid, org_id) VALUES (?, ?)`, 2, 1).Error
	require.NoError(t, err)

	got, err := s.CountByUser(ctx, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), got)

	got, err = s.CountByUser(ctx, 404)
	require.NoError(t, err)
	assert.Equal(t, int64(0), got)
}

func organizationsCreate(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	owner := newTestIssueUser(t, ctx, s.db, "oc-owner")

	err := s.Create(ctx, &User{Name: "oc-owner", Email: "oc-taken@example.com"}, owner)
	assert.True(t, IsErrUserAlreadyExist(err), "got %v", err)
	err = s.Create(ctx, &User{Name: "new", Email: "oc-new@example.com"}, owner)
	assert.True(t, IsErrNameNotAllowed(err), "got %v", err)

	org := &User{Name: "OC-Org", Email: "oc-org@example.com"}
	require.NoError(t, s.Create(ctx, org, owner))
	got, err := s.GetByName(ctx, "oc-org")
	require.NoError(t, err)
	assert.Equal(t, org.ID, got.ID)
	assert.True(t, got.IsOrganization())
	assert.Equal(t, "oc-org", got.LowerName)
	assert.Equal(t, 1, got.NumTeams)
	assert.Equal(t, 1, got.NumMembers)
	assert.Equal(t, -1, got.MaxRepoCreation)
	assert.NotZero(t, got.CreatedUnix)
	assert.True(t, osx.IsDir(repox.UserPath(org.Name)))

	assert.True(t, s.IsOwner(ctx, org.ID, owner.ID))
	assert.False(t, s.IsPublicMember(ctx, org.ID, owner.ID))
	assert.Equal(t, 1, orgUser(t, s.db, org.ID, owner.ID).NumTeams)
	teams := newTeamsStore(s.db)
	owners, err := teams.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, AccessModeOwner, owners.Authorize)
	assert.Equal(t, 1, owners.NumMembers)
	assert.True(t, teams.IsMember(ctx, org.ID, owners.ID, owner.ID))
}

func organizationsGetByName(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, owner := newTestOrg(t, ctx, s.db, "og-org")

	got, err := s.GetByName(ctx, "OG-ORG")
	require.NoError(t, err)
	assert.Equal(t, org.ID, got.ID)

	for _, name := range []string{"", owner.Name, "og-missing"} {
		_, err = s.GetByName(ctx, name)
		assert.Equal(t, ErrOrgNotExist, err, "name %q", name)
	}
}

func organizationsListByPage(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org1, _ := newTestOrg(t, ctx, s.db, "ol-org1")
	org2, _ := newTestOrg(t, ctx, s.db, "ol-org2")
	org3, _ := newTestOrg(t, ctx, s.db, "ol-org3")

	assert.Equal(t, int64(3), s.Count(ctx))
	got, err := s.ListByPage(ctx, 1, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{org1.ID, org2.ID}, userIDs(got))
	got, err = s.ListByPage(ctx, 2, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{org3.ID}, userIDs(got))
}

func organizationsDelete(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, owner := newTestOrg(t, ctx, s.db, "od-org")
	kept, keptOwner := newTestOrg(t, ctx, s.db, "od-kept")
	teams := newTeamsStore(s.db)
	team := newTestTeam(t, ctx, teams, org.ID, "devs", AccessModeWrite)
	require.NoError(t, teams.AddMember(ctx, org.ID, team.ID, owner.ID))
	keptTeam := newTestTeam(t, ctx, teams, kept.ID, "devs", AccessModeWrite)
	require.NoError(t, teams.AddMember(ctx, kept.ID, keptTeam.ID, keptOwner.ID))
	// A team repository row is left by a repository the organization no
	// longer owns.
	require.NoError(t, s.db.Create(&TeamRepo{OrgID: org.ID, TeamID: team.ID, RepoID: 9999}).Error)
	require.NoError(t, s.db.Create(&TeamRepo{OrgID: kept.ID, TeamID: keptTeam.ID, RepoID: 9999}).Error)

	require.NoError(t, s.Delete(ctx, org))
	_, err := s.GetByName(ctx, org.Name)
	assert.Equal(t, ErrOrgNotExist, err)
	countOf := func(table any, orgID int64) int64 {
		var count int64
		require.NoError(t, s.db.Model(table).Where("org_id = ?", orgID).Count(&count).Error)
		return count
	}
	for _, table := range []any{new(OrgUser), new(Team), new(TeamUser), new(TeamRepo)} {
		assert.Zero(t, countOf(table, org.ID), "%T", table)
		assert.NotZero(t, countOf(table, kept.ID), "%T", table)
	}
	assert.False(t, s.IsMember(ctx, org.ID, owner.ID))
}

func organizationsMembers(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, owner := newTestOrg(t, ctx, s.db, "om-org")
	other, _ := newTestOrg(t, ctx, s.db, "om-other")
	alice := newTestIssueUser(t, ctx, s.db, "om-alice")
	bob := newTestIssueUser(t, ctx, s.db, "om-bob")

	require.NoError(t, s.AddMember(ctx, org.ID, bob.ID))
	require.NoError(t, s.AddMember(ctx, org.ID, alice.ID))
	require.NoError(t, s.AddMember(ctx, org.ID, alice.ID))
	require.NoError(t, s.AddMember(ctx, other.ID, alice.ID))
	assert.Equal(t, 3, reloadUser(t, ctx, s.db, org.ID).NumMembers)
	assert.True(t, s.IsMember(ctx, org.ID, alice.ID))
	assert.False(t, s.IsOwner(ctx, org.ID, alice.ID))
	assert.False(t, s.IsMember(ctx, 9999, alice.ID))

	got, err := s.ListMembers(ctx, org.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{owner.ID, bob.ID, alice.ID}, userIDs(got))
	got, err = s.ListMembers(ctx, org.ID, 2)
	require.NoError(t, err)
	assert.Equal(t, []int64{owner.ID, bob.ID}, userIDs(got))

	assert.False(t, s.IsPublicMember(ctx, org.ID, alice.ID))
	require.NoError(t, s.SetMemberVisibility(ctx, org.ID, alice.ID, true))
	assert.True(t, s.IsPublicMember(ctx, org.ID, alice.ID))
	assert.False(t, s.IsPublicMember(ctx, other.ID, alice.ID))
	assert.False(t, s.IsPublicMember(ctx, org.ID, bob.ID))
	require.NoError(t, s.SetMemberVisibility(ctx, org.ID, alice.ID, false))
	assert.False(t, s.IsPublicMember(ctx, org.ID, alice.ID))
}

func organizationsListOwnedBy(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org1, owner := newTestOrg(t, ctx, s.db, "oo-org1")
	org2, _ := newTestOrg(t, ctx, s.db, "oo-org2")
	org3, _ := newTestOrg(t, ctx, s.db, "oo-org3")
	teams := newTeamsStore(s.db)
	for _, org := range []*User{org2, org3} {
		owners, err := teams.GetOwnerTeam(ctx, org.ID)
		require.NoError(t, err)
		require.NoError(t, teams.AddMember(ctx, org.ID, owners.ID, owner.ID))
	}
	// Being a member does not make an owner.
	member, _ := newTestOrg(t, ctx, s.db, "oo-member")
	require.NoError(t, s.AddMember(ctx, member.ID, owner.ID))
	for unix, org := range map[int64]*User{300: org1, 100: org2, 200: org3} {
		require.NoError(t, s.db.Model(new(User)).Where("id = ?", org.ID).UpdateColumn("updated_unix", unix).Error)
	}

	got, err := s.ListOwnedBy(ctx, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{org1.ID, org3.ID, org2.ID}, userIDs(got))
}

func organizationsRemoveMember(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, owner := newTestOrg(t, ctx, s.db, "or-org")
	alice := newTestIssueUser(t, ctx, s.db, "or-alice")
	bob := newTestIssueUser(t, ctx, s.db, "or-bob")
	teams := newTeamsStore(s.db)
	repos := newReposStore(s.db)
	perms := newPermissionsStore(s.db)
	team := newTestTeam(t, ctx, teams, org.ID, "devs", AccessModeWrite)
	teamRepo := newTestTeamRepo(t, ctx, s.db, org, "team", true)
	public := newTestTeamRepo(t, ctx, s.db, org, "public", false)
	require.NoError(t, teams.AddRepo(ctx, team, teamRepo))
	require.NoError(t, teams.AddMember(ctx, org.ID, team.ID, alice.ID))
	require.NoError(t, repos.Watch(ctx, alice.ID, teamRepo.ID))
	require.NoError(t, repos.Watch(ctx, alice.ID, public.ID))

	require.NoError(t, s.RemoveMember(ctx, org.ID, alice.ID))
	assert.False(t, s.IsMember(ctx, org.ID, alice.ID))
	assert.False(t, teams.IsMember(ctx, org.ID, team.ID, alice.ID))
	assert.Equal(t, 0, reloadTeam(t, ctx, teams, team.ID).NumMembers)
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, org.ID).NumMembers)
	assert.False(t, repos.IsWatching(ctx, alice.ID, teamRepo.ID))
	assert.False(t, repos.IsWatching(ctx, alice.ID, public.ID))
	assert.NotContains(t, accessModes(t, ctx, perms, teamRepo.ID), alice.ID)

	// Removing a user who is not a member is a no-op.
	require.NoError(t, s.RemoveMember(ctx, org.ID, alice.ID))
	assert.Equal(t, 1, reloadUser(t, ctx, s.db, org.ID).NumMembers)

	err := s.RemoveMember(ctx, org.ID, owner.ID)
	assert.True(t, IsErrLastOrgOwner(err), "got %v", err)
	assert.True(t, s.IsOwner(ctx, org.ID, owner.ID))

	owners, err := teams.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	require.NoError(t, teams.AddMember(ctx, org.ID, owners.ID, bob.ID))
	require.NoError(t, s.RemoveMember(ctx, org.ID, owner.ID))
	assert.False(t, s.IsMember(ctx, org.ID, owner.ID))
	assert.Equal(t, 1, reloadTeam(t, ctx, teams, owners.ID).NumMembers)
	assert.True(t, s.IsOwner(ctx, org.ID, bob.ID))
}

func organizationsListAccessibleRepos(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "oa-org")
	other, _ := newTestOrg(t, ctx, s.db, "oa-other")
	alice := newTestIssueUser(t, ctx, s.db, "oa-alice")
	teams := newTeamsStore(s.db)
	team := newTestTeam(t, ctx, teams, org.ID, "devs", AccessModeRead)
	require.NoError(t, teams.AddMember(ctx, org.ID, team.ID, alice.ID))

	public := newTestTeamRepo(t, ctx, s.db, org, "public", false)
	unlisted := newTestTeamRepo(t, ctx, s.db, org, "unlisted", false)
	require.NoError(t, s.db.Model(new(Repository)).Where("id = ?", unlisted.ID).UpdateColumn("is_unlisted", true).Error)
	teamRepo := newTestTeamRepo(t, ctx, s.db, org, "team", true)
	require.NoError(t, teams.AddRepo(ctx, team, teamRepo))
	_ = newTestTeamRepo(t, ctx, s.db, org, "private", true)
	_ = newTestTeamRepo(t, ctx, s.db, other, "elsewhere", false)
	for unix, repo := range map[int64]*Repository{100: public, 300: teamRepo} {
		require.NoError(t, s.db.Model(new(Repository)).Where("id = ?", repo.ID).UpdateColumn("updated_unix", unix).Error)
	}

	got, count, err := s.ListAccessibleRepos(ctx, org.ID, alice.ID, 1, 0)
	require.NoError(t, err)
	assert.Equal(t, []int64{teamRepo.ID, public.ID}, repoIDs(got))
	assert.Equal(t, int64(2), count)

	got, count, err = s.ListAccessibleRepos(ctx, org.ID, alice.ID, 2, 1)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID}, repoIDs(got))
	assert.Equal(t, int64(2), count)

	got, count, err = s.ListAccessibleRepos(ctx, org.ID, 9999, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID}, repoIDs(got))
	assert.Equal(t, int64(1), count)
}

func organizationsListAccessibleMirrors(t *testing.T, ctx context.Context, s *OrganizationsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "oam-org")
	alice := newTestIssueUser(t, ctx, s.db, "oam-alice")
	teams := newTeamsStore(s.db)
	team := newTestTeam(t, ctx, teams, org.ID, "devs", AccessModeRead)
	require.NoError(t, teams.AddMember(ctx, org.ID, team.ID, alice.ID))

	newMirror := func(name string, private bool) *Repository {
		repo, err := newReposStore(s.db).create(ctx, org.ID, CreateRepoOptions{Name: name, Private: private, Mirror: true})
		require.NoError(t, err)
		return repo
	}
	public := newMirror("public", false)
	teamMirror := newMirror("team", true)
	require.NoError(t, teams.AddRepo(ctx, team, teamMirror))
	_ = newMirror("private", true)
	_ = newTestTeamRepo(t, ctx, s.db, org, "regular", false)
	for unix, repo := range map[int64]*Repository{100: public, 300: teamMirror} {
		require.NoError(t, s.db.Model(new(Repository)).Where("id = ?", repo.ID).UpdateColumn("updated_unix", unix).Error)
	}

	got, err := s.ListAccessibleMirrors(ctx, org.ID, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{teamMirror.ID, public.ID}, repoIDs(got))

	got, err = s.ListAccessibleMirrors(ctx, org.ID, 9999)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID}, repoIDs(got))
}
