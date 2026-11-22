package database

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
)

func TestTeams(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// NOTE: The suite does not run in parallel because it mocks global
	// configuration that other suites read without locks.

	ctx := context.Background()
	s := &TeamsStore{
		db: newTestDB(t, "TeamsStore"),
	}
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})
	conf.SetMockPicture(t, conf.PictureOpts{AvatarUploadPath: t.TempDir()})

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *TeamsStore)
	}{
		{"Create", teamsCreate},
		{"Get", teamsGet},
		{"List", teamsList},
		{"Update", teamsUpdate},
		{"Delete", teamsDelete},
		{"Members", teamsMembers},
		{"Repos", teamsRepos},
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

// newTestOrg creates an organization of given name with a new user of the
// name suffixed by "-owner" as its owner.
func newTestOrg(t *testing.T, ctx context.Context, db *gorm.DB, name string) (org, owner *User) {
	t.Helper()

	owner = newTestIssueUser(t, ctx, db, name+"-owner")
	org = &User{Name: name, Email: name + "@example.com"}
	require.NoError(t, newOrganizationsStore(db).Create(ctx, org, owner))
	return org, owner
}

// newTestTeamRepo creates a repository of given name in the organization.
func newTestTeamRepo(t *testing.T, ctx context.Context, db *gorm.DB, org *User, name string, private bool) *Repository {
	t.Helper()

	repo, err := newReposStore(db).create(ctx, org.ID, CreateRepoOptions{Name: name, Private: private})
	require.NoError(t, err)
	return repo
}

func createTestTeam(t *testing.T, ctx context.Context, s *TeamsStore, orgID int64, name string, mode AccessMode) *Team {
	t.Helper()

	team := &Team{OrgID: orgID, Name: name, Authorize: mode}
	require.NoError(t, s.Create(ctx, team))
	return team
}

func teamIDs(teams []*Team) []int64 {
	ids := make([]int64, 0, len(teams))
	for _, t := range teams {
		ids = append(ids, t.ID)
	}
	return ids
}

func reloadTeam(t *testing.T, ctx context.Context, s *TeamsStore, id int64) *Team {
	t.Helper()

	team, err := s.GetByID(ctx, id)
	require.NoError(t, err)
	return team
}

func reloadUser(t *testing.T, ctx context.Context, db *gorm.DB, id int64) *User {
	t.Helper()

	u, err := newUsersStore(db).GetByID(ctx, id)
	require.NoError(t, err)
	return u
}

func orgUser(t *testing.T, db *gorm.DB, orgID, userID int64) *OrgUser {
	t.Helper()

	ou := new(OrgUser)
	require.NoError(t, db.Where("org_id = ? AND uid = ?", orgID, userID).First(ou).Error)
	return ou
}

func teamsCreate(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "tc-org")
	other, _ := newTestOrg(t, ctx, s.db, "tc-other")

	assert.Error(t, s.Create(ctx, &Team{OrgID: org.ID}), "empty name")
	assert.Error(t, s.Create(ctx, &Team{Name: "devs"}), "no organization")
	assert.True(t, IsErrNameNotAllowed(s.Create(ctx, &Team{OrgID: org.ID, Name: "new"})))
	assert.Equal(t, ErrOrgNotExist, s.Create(ctx, &Team{OrgID: 9999, Name: "devs"}))

	team := &Team{OrgID: org.ID, Name: "Devs", Authorize: AccessModeWrite}
	require.NoError(t, s.Create(ctx, team))
	got := reloadTeam(t, ctx, s, team.ID)
	assert.Equal(t, "devs", got.LowerName)
	assert.Equal(t, "Devs", got.Name)
	assert.Equal(t, AccessModeWrite, got.Authorize)
	assert.Equal(t, 2, reloadUser(t, ctx, s.db, org.ID).NumTeams)

	err := s.Create(ctx, &Team{OrgID: org.ID, Name: "DEVS"})
	assert.True(t, IsErrTeamAlreadyExist(err), "got %v", err)
	assert.Equal(t, 2, reloadUser(t, ctx, s.db, org.ID).NumTeams)

	// Names only need to be unique within an organization.
	require.NoError(t, s.Create(ctx, &Team{OrgID: other.ID, Name: "devs"}))
	assert.Equal(t, 2, reloadUser(t, ctx, s.db, other.ID).NumTeams)
}

func teamsGet(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "tg-org")
	other, _ := newTestOrg(t, ctx, s.db, "tg-other")
	team := createTestTeam(t, ctx, s, org.ID, "Devs", AccessModeRead)

	got, err := s.GetByID(ctx, team.ID)
	require.NoError(t, err)
	assert.Equal(t, "Devs", got.Name)
	_, err = s.GetByID(ctx, 9999)
	assert.True(t, IsErrTeamNotExist(err), "got %v", err)

	got, err = s.GetByName(ctx, org.ID, "DEVS")
	require.NoError(t, err)
	assert.Equal(t, team.ID, got.ID)
	_, err = s.GetByName(ctx, other.ID, "devs")
	assert.True(t, IsErrTeamNotExist(err), "got %v", err)

	owners, err := s.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	assert.True(t, owners.IsOwnerTeam())
	assert.Equal(t, org.ID, owners.OrgID)
	assert.Equal(t, AccessModeOwner, owners.Authorize)
}

func teamsList(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "tl-org")
	other, _ := newTestOrg(t, ctx, s.db, "tl-other")
	alice := newTestIssueUser(t, ctx, s.db, "tl-alice")
	owners, err := s.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	readers := createTestTeam(t, ctx, s, org.ID, "readers", AccessModeRead)
	writers := createTestTeam(t, ctx, s, org.ID, "writers", AccessModeWrite)
	admins := createTestTeam(t, ctx, s, org.ID, "admins", AccessModeAdmin)
	otherTeam := createTestTeam(t, ctx, s, other.ID, "writers", AccessModeWrite)

	got, err := s.ListByOrg(ctx, org.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{owners.ID, readers.ID, writers.ID, admins.ID}, teamIDs(got))

	// Teams are listed in the order they were created, not joined.
	require.NoError(t, s.AddMember(ctx, org.ID, writers.ID, alice.ID))
	require.NoError(t, s.AddMember(ctx, org.ID, readers.ID, alice.ID))
	require.NoError(t, s.AddMember(ctx, other.ID, otherTeam.ID, alice.ID))
	got, err = s.ListByMember(ctx, org.ID, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{readers.ID, writers.ID}, teamIDs(got))

	repo := newTestTeamRepo(t, ctx, s.db, org, "repo1", true)
	repo2 := newTestTeamRepo(t, ctx, s.db, org, "repo2", true)
	for _, team := range []*Team{admins, writers, readers} {
		require.NoError(t, s.AddRepo(ctx, team, repo))
	}
	require.NoError(t, s.AddRepo(ctx, readers, repo2))
	got, err = s.ListByRepo(ctx, org.ID, repo.ID, AccessModeWrite)
	require.NoError(t, err)
	assert.Equal(t, []int64{writers.ID, admins.ID}, teamIDs(got))
	got, err = s.ListByRepo(ctx, org.ID, repo.ID, AccessModeRead)
	require.NoError(t, err)
	assert.Equal(t, []int64{readers.ID, writers.ID, admins.ID}, teamIDs(got))
	got, err = s.ListByRepo(ctx, org.ID, repo2.ID, AccessModeWrite)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func teamsUpdate(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "tu-org")
	alice := newTestIssueUser(t, ctx, s.db, "tu-alice")
	repo := newTestTeamRepo(t, ctx, s.db, org, "repo1", true)
	team := createTestTeam(t, ctx, s, org.ID, "devs", AccessModeRead)
	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, alice.ID))
	require.NoError(t, s.AddRepo(ctx, team, repo))
	perms := newPermissionsStore(s.db)
	assert.Equal(t, AccessModeRead, accessModes(t, ctx, perms, repo.ID)[alice.ID])

	assert.Error(t, s.Update(ctx, &Team{ID: team.ID, OrgID: org.ID}, false), "empty name")
	assert.Equal(t, "devs", reloadTeam(t, ctx, s, team.ID).Name)

	team.Name = "Owners"
	err := s.Update(ctx, team, false)
	assert.True(t, IsErrTeamAlreadyExist(err), "got %v", err)
	assert.Equal(t, "devs", reloadTeam(t, ctx, s, team.ID).Name)

	// Changing the case of its own name is not a conflict.
	team.Name = "DEVS"
	team.Description = strings.Repeat("a", 300)
	team.Authorize = AccessModeAdmin
	require.NoError(t, s.Update(ctx, team, false))
	got := reloadTeam(t, ctx, s, team.ID)
	assert.Equal(t, "DEVS", got.Name)
	assert.Equal(t, "devs", got.LowerName)
	assert.Equal(t, strings.Repeat("a", 255), got.Description)
	assert.Equal(t, AccessModeAdmin, got.Authorize)
	assert.Equal(t, AccessModeRead, accessModes(t, ctx, perms, repo.ID)[alice.ID], "accesses are kept when the access mode is said to be unchanged")

	team.Authorize = AccessModeWrite
	require.NoError(t, s.Update(ctx, team, true))
	assert.Equal(t, AccessModeWrite, accessModes(t, ctx, perms, repo.ID)[alice.ID])
}

func teamsDelete(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "td-org")
	alice := newTestIssueUser(t, ctx, s.db, "td-alice")
	bob := newTestIssueUser(t, ctx, s.db, "td-bob")
	repo := newTestTeamRepo(t, ctx, s.db, org, "repo1", true)
	team := createTestTeam(t, ctx, s, org.ID, "devs", AccessModeWrite)
	kept := createTestTeam(t, ctx, s, org.ID, "ops", AccessModeRead)
	for _, tm := range []*Team{team, kept} {
		require.NoError(t, s.AddMember(ctx, org.ID, tm.ID, alice.ID))
		require.NoError(t, s.AddRepo(ctx, tm, repo))
	}
	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, bob.ID))
	perms := newPermissionsStore(s.db)
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite, bob.ID: AccessModeWrite}, withoutOwners(accessModes(t, ctx, perms, repo.ID)))

	require.NoError(t, s.Delete(ctx, team))
	_, err := s.GetByID(ctx, team.ID)
	assert.True(t, IsErrTeamNotExist(err), "got %v", err)
	assert.False(t, s.IsMember(ctx, org.ID, team.ID, alice.ID))
	assert.False(t, s.HasRepo(ctx, org.ID, team.ID, repo.ID))
	assert.True(t, s.IsMember(ctx, org.ID, kept.ID, alice.ID))
	assert.True(t, s.HasRepo(ctx, org.ID, kept.ID, repo.ID))
	assert.Equal(t, 2, reloadUser(t, ctx, s.db, org.ID).NumTeams)
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeRead}, withoutOwners(accessModes(t, ctx, perms, repo.ID)))
}

// withoutOwners returns the accesses without those of the owners of the
// organization, who have every repository.
func withoutOwners(modes map[int64]AccessMode) map[int64]AccessMode {
	for id, mode := range modes {
		if mode == AccessModeOwner {
			delete(modes, id)
		}
	}
	return modes
}

func teamsMembers(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, owner := newTestOrg(t, ctx, s.db, "tm-org")
	alice := newTestIssueUser(t, ctx, s.db, "tm-alice")
	bob := newTestIssueUser(t, ctx, s.db, "tm-bob")
	repo := newTestTeamRepo(t, ctx, s.db, org, "repo1", true)
	team := createTestTeam(t, ctx, s, org.ID, "devs", AccessModeWrite)
	require.NoError(t, s.AddRepo(ctx, team, repo))
	owners, err := s.GetOwnerTeam(ctx, org.ID)
	require.NoError(t, err)
	orgs := newOrganizationsStore(s.db)
	perms := newPermissionsStore(s.db)

	// Joining a team makes the user a member of the organization.
	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, alice.ID))
	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, alice.ID))
	assert.True(t, s.IsMember(ctx, org.ID, team.ID, alice.ID))
	assert.False(t, s.IsMember(ctx, org.ID, owners.ID, alice.ID))
	assert.True(t, orgs.IsMember(ctx, org.ID, alice.ID))
	assert.False(t, orgs.IsOwner(ctx, org.ID, alice.ID))
	assert.Equal(t, 1, reloadTeam(t, ctx, s, team.ID).NumMembers)
	assert.Equal(t, 2, reloadUser(t, ctx, s.db, org.ID).NumMembers)
	assert.Equal(t, 1, orgUser(t, s.db, org.ID, alice.ID).NumTeams)
	assert.Equal(t, AccessModeWrite, accessModes(t, ctx, perms, repo.ID)[alice.ID])

	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, bob.ID))
	require.NoError(t, s.AddMember(ctx, org.ID, owners.ID, bob.ID))
	members, err := s.ListMembers(ctx, team.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{alice.ID, bob.ID}, userIDs(members))
	assert.True(t, orgs.IsOwner(ctx, org.ID, bob.ID))
	assert.Equal(t, 2, orgUser(t, s.db, org.ID, bob.ID).NumTeams)
	assert.Equal(t, 3, reloadUser(t, ctx, s.db, org.ID).NumMembers)
	assert.Equal(t, 2, reloadTeam(t, ctx, s, owners.ID).NumMembers)

	// Leaving a team keeps the organization membership.
	require.NoError(t, s.RemoveMember(ctx, org.ID, team.ID, alice.ID))
	require.NoError(t, s.RemoveMember(ctx, org.ID, team.ID, alice.ID))
	assert.False(t, s.IsMember(ctx, org.ID, team.ID, alice.ID))
	assert.True(t, orgs.IsMember(ctx, org.ID, alice.ID))
	assert.Equal(t, 1, reloadTeam(t, ctx, s, team.ID).NumMembers)
	assert.Equal(t, 0, orgUser(t, s.db, org.ID, alice.ID).NumTeams)
	assert.NotContains(t, accessModes(t, ctx, perms, repo.ID), alice.ID)

	require.NoError(t, s.RemoveMember(ctx, org.ID, owners.ID, bob.ID))
	assert.False(t, orgs.IsOwner(ctx, org.ID, bob.ID))
	assert.True(t, s.IsMember(ctx, org.ID, team.ID, bob.ID), "leaving a team keeps the other teams")
	assert.Equal(t, 1, orgUser(t, s.db, org.ID, bob.ID).NumTeams)

	err = s.RemoveMember(ctx, org.ID, owners.ID, owner.ID)
	assert.True(t, IsErrLastOrgOwner(err), "got %v", err)
	assert.True(t, orgs.IsOwner(ctx, org.ID, owner.ID))
}

func userIDs(users []*User) []int64 {
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	return ids
}

func teamsRepos(t *testing.T, ctx context.Context, s *TeamsStore) {
	org, _ := newTestOrg(t, ctx, s.db, "tr-org")
	alice := newTestIssueUser(t, ctx, s.db, "tr-alice")
	private := newTestTeamRepo(t, ctx, s.db, org, "private", true)
	public := newTestTeamRepo(t, ctx, s.db, org, "public", false)
	foreign := newTestIssueRepo(t, ctx, s.db, "tr-foreign")
	team := createTestTeam(t, ctx, s, org.ID, "devs", AccessModeWrite)
	require.NoError(t, s.AddMember(ctx, org.ID, team.ID, alice.ID))
	repos := newReposStore(s.db)
	perms := newPermissionsStore(s.db)

	assert.Error(t, s.AddRepo(ctx, team, foreign))
	assert.False(t, s.HasRepo(ctx, org.ID, team.ID, foreign.ID))
	// The owner that counts is the one in the database, e.g., after the
	// repository was transferred away while the caller held it.
	moved := newTestTeamRepo(t, ctx, s.db, org, "moved", false)
	require.NoError(t, s.db.Model(moved).UpdateColumn("owner_id", alice.ID).Error)
	assert.Error(t, s.AddRepo(ctx, team, moved))
	assert.False(t, s.HasRepo(ctx, org.ID, team.ID, moved.ID))

	require.NoError(t, s.AddRepo(ctx, team, public))
	require.NoError(t, s.AddRepo(ctx, team, private))
	require.NoError(t, s.AddRepo(ctx, team, private))
	assert.True(t, s.HasRepo(ctx, org.ID, team.ID, private.ID))
	assert.Equal(t, 2, team.NumRepos)
	assert.Equal(t, 2, reloadTeam(t, ctx, s, team.ID).NumRepos)
	got, err := s.ListRepos(ctx, team.ID)
	require.NoError(t, err)
	assert.Equal(t, []int64{public.ID, private.ID}, repoIDs(got))
	assert.True(t, repos.IsWatching(ctx, alice.ID, private.ID))
	assert.True(t, repos.IsWatching(ctx, alice.ID, public.ID))
	assert.Equal(t, AccessModeWrite, accessModes(t, ctx, perms, private.ID)[alice.ID])

	// Members keep watching the repositories they can still read.
	require.NoError(t, s.RemoveRepo(ctx, team, public.ID))
	require.NoError(t, s.RemoveRepo(ctx, team, private.ID))
	require.NoError(t, s.RemoveRepo(ctx, team, private.ID))
	assert.False(t, s.HasRepo(ctx, org.ID, team.ID, private.ID))
	assert.Equal(t, 0, team.NumRepos)
	assert.Equal(t, 0, reloadTeam(t, ctx, s, team.ID).NumRepos)
	assert.False(t, repos.IsWatching(ctx, alice.ID, private.ID))
	assert.True(t, repos.IsWatching(ctx, alice.ID, public.ID))
	assert.NotContains(t, accessModes(t, ctx, perms, private.ID), alice.ID)
}

func repoIDs(repos []*Repository) []int64 {
	ids := make([]int64, 0, len(repos))
	for _, r := range repos {
		ids = append(ids, r.ID)
	}
	return ids
}
