package database

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
	"gogs.io/gogs/internal/git"
	"gogs.io/gogs/internal/osx"
)

func TestProtectBranches(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	// NOTE: The suite does not run in parallel because it mocks global
	// configuration that other suites read without locks.

	ctx := context.Background()
	s := &ProtectBranchesStore{
		db: newTestDB(t, "ProtectBranchesStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *ProtectBranchesStore)
	}{
		{"GetByName", protectBranchesGetByName},
		{"ListProtected", protectBranchesListProtected},
		{"RequiresPullRequest", protectBranchesRequiresPullRequest},
		{"IsUserInAllowlist", protectBranchesIsUserInAllowlist},
		{"Save", protectBranchesSave},
		{"SaveForOrg", protectBranchesSaveForOrg},
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

func protectBranchesGetByName(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	err := s.db.Create(&ProtectBranch{RepoID: 1, Name: "main", Protected: true, AllowlistUserIDs: "3"}).Error
	require.NoError(t, err)

	got, err := s.GetByName(ctx, 1, "main")
	require.NoError(t, err)
	assert.True(t, got.Protected)
	assert.Equal(t, "3", got.AllowlistUserIDs)

	_, err = s.GetByName(ctx, 2, "main")
	assert.True(t, IsErrBranchNotExist(err), "got %v", err)
	_, err = s.GetByName(ctx, 1, "dev")
	assert.True(t, IsErrBranchNotExist(err), "got %v", err)
}

func protectBranchesListProtected(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	for _, pb := range []*ProtectBranch{
		{RepoID: 1, Name: "release", Protected: true},
		{RepoID: 1, Name: "main", Protected: true},
		{RepoID: 1, Name: "dev", Protected: false},
		{RepoID: 2, Name: "other", Protected: true},
	} {
		err := s.db.Create(pb).Error
		require.NoError(t, err)
	}

	got, err := s.ListProtected(ctx, 1)
	require.NoError(t, err)
	var names []string
	for _, pb := range got {
		names = append(names, pb.Name)
	}
	assert.Equal(t, []string{"main", "release"}, names)
}

func protectBranchesRequiresPullRequest(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	for _, pb := range []*ProtectBranch{
		{RepoID: 1, Name: "main", Protected: true, RequirePullRequest: true},
		{RepoID: 1, Name: "dev", Protected: true, RequirePullRequest: false},
		{RepoID: 1, Name: "old", Protected: false, RequirePullRequest: true},
	} {
		err := s.db.Create(pb).Error
		require.NoError(t, err)
	}

	logger := logx.NewNoopLogger()
	assert.True(t, s.RequiresPullRequest(ctx, logger, 1, "main"))
	assert.False(t, s.RequiresPullRequest(ctx, logger, 1, "dev"))
	assert.False(t, s.RequiresPullRequest(ctx, logger, 1, "old"))
	assert.False(t, s.RequiresPullRequest(ctx, logger, 1, "none"))
	assert.False(t, s.RequiresPullRequest(ctx, logger, 2, "main"))
}

func protectBranchesIsUserInAllowlist(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	team := &Team{OrgID: 10, LowerName: "writers", Name: "writers", Authorize: AccessModeWrite}
	err := s.db.Create(team).Error
	require.NoError(t, err)
	err = s.db.Create(&TeamUser{OrgID: 10, TeamID: team.ID, UID: 9}).Error
	require.NoError(t, err)
	err = s.db.Create(&TeamUser{OrgID: 10, TeamID: team.ID + 1, UID: 8}).Error
	require.NoError(t, err)

	pb := &ProtectBranch{
		AllowlistUserIDs: "5,7",
		AllowlistTeamIDs: strconv.FormatInt(team.ID, 10),
	}
	for _, tc := range []struct {
		userID int64
		want   bool
	}{
		{userID: 5, want: true},
		{userID: 7, want: true},
		{userID: 9, want: true},  // Member of an allowlisted team
		{userID: 8, want: false}, // Member of another team
		{userID: 6, want: false},
		{userID: 0, want: false},
	} {
		got, err := s.IsUserInAllowlist(ctx, pb, tc.userID)
		require.NoError(t, err)
		assert.Equal(t, tc.want, got, "user %d", tc.userID)
	}

	got, err := s.IsUserInAllowlist(ctx, &ProtectBranch{}, 9)
	require.NoError(t, err)
	assert.False(t, got)
}

// newTestRepoOnDisk creates a repository of given owner in the database and
// the directory of its hooks on disk, and returns it with its owner loaded.
//
// NOTE: Owners must have names that no other test uses, because tests that
// delete users remove their directories under the repository root that is
// mocked at that time.
func newTestRepoOnDisk(t *testing.T, ctx context.Context, s *ProtectBranchesStore, owner *User, name string) *Repository {
	t.Helper()

	repo, err := newReposStore(s.db).Create(ctx, owner.ID, CreateRepoOptions{Name: name})
	require.NoError(t, err)
	repo.Owner = owner
	err = os.MkdirAll(filepath.Join(repo.RepoPath(), "hooks"), os.ModePerm)
	require.NoError(t, err)
	return repo
}

func protectBranchesSave(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})

	alice, err := newUsersStore(s.db).Create(ctx, "pb-alice", "pb-alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo := newTestRepoOnDisk(t, ctx, s, alice, "repo1")
	preReceive := filepath.Join(repo.RepoPath(), "hooks", string(git.HookPreReceive))

	pb := &ProtectBranch{RepoID: repo.ID, Name: "main", Protected: true, RequirePullRequest: true}
	err = s.Save(ctx, pb)
	require.NoError(t, err)
	require.NotZero(t, pb.ID)
	assert.True(t, osx.IsFile(preReceive), "pre-receive hook not installed")
	got, err := s.GetByName(ctx, repo.ID, "main")
	require.NoError(t, err)
	assert.Equal(t, pb.ID, got.ID)
	assert.True(t, got.RequirePullRequest)

	// Unprotecting the only protected branch updates the same row, and the hook
	// is not needed anymore.
	pb.Protected = false
	err = s.Save(ctx, pb)
	require.NoError(t, err)
	assert.False(t, osx.IsFile(preReceive), "pre-receive hook not removed")
	got, err = s.GetByName(ctx, repo.ID, "main")
	require.NoError(t, err)
	assert.Equal(t, pb.ID, got.ID)
	assert.False(t, got.Protected)
	var count int64
	err = s.db.Model(new(ProtectBranch)).Count(&count).Error
	require.NoError(t, err)
	assert.Equal(t, int64(1), count)

	// Nothing is saved for a repository that does not exist.
	err = s.Save(ctx, &ProtectBranch{RepoID: 404, Name: "main", Protected: true})
	require.Error(t, err)
	_, err = s.GetByName(ctx, 404, "main")
	assert.True(t, IsErrBranchNotExist(err), "got %v", err)
}

func protectBranchesSaveForOrg(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	conf.SetMockRepository(t, conf.RepositoryOpts{Root: t.TempDir()})

	usersStore := newUsersStore(s.db)
	alice, err := usersStore.Create(ctx, "pb-alice", "pb-alice@example.com", CreateUserOptions{})
	require.NoError(t, err)
	bob, err := usersStore.Create(ctx, "pb-bob", "pb-bob@example.com", CreateUserOptions{})
	require.NoError(t, err)
	// TODO: Use Orgs.Create to replace SQL hack when the method is available.
	org, err := usersStore.Create(ctx, "pb-org", "pb-org@example.com", CreateUserOptions{})
	require.NoError(t, err)
	err = s.db.Exec(dbx.Quote("UPDATE %s SET type = ? WHERE id = ?", "user"), UserTypeOrganization, org.ID).Error
	require.NoError(t, err)
	org.Type = UserTypeOrganization

	repo := newTestRepoOnDisk(t, ctx, s, org, "repo1")
	err = s.db.Create(&Access{UserID: alice.ID, RepoID: repo.ID, Mode: AccessModeWrite}).Error
	require.NoError(t, err)
	err = s.db.Create(&Access{UserID: bob.ID, RepoID: repo.ID, Mode: AccessModeRead}).Error
	require.NoError(t, err)

	newTeam := func(name string, mode AccessMode, hasRepo bool) *Team {
		team := &Team{OrgID: org.ID, LowerName: name, Name: name, Authorize: mode}
		err := s.db.Create(team).Error
		require.NoError(t, err)
		if hasRepo {
			err = s.db.Create(&TeamRepo{OrgID: org.ID, TeamID: team.ID, RepoID: repo.ID}).Error
			require.NoError(t, err)
		}
		return team
	}
	writers := newTeam("writers", AccessModeWrite, true)
	readers := newTeam("readers", AccessModeRead, true)
	elsewhere := newTeam("elsewhere", AccessModeWrite, false)
	newTeam("unlisted", AccessModeWrite, true)

	pb := &ProtectBranch{RepoID: repo.ID, Name: "main", Protected: true, EnableAllowlist: true}
	err = s.SaveForOrg(ctx, logx.NewNoopLogger(), pb,
		[]int64{bob.ID, alice.ID, 404},
		[]int64{readers.ID, writers.ID, elsewhere.ID},
	)
	require.NoError(t, err)

	got, err := s.GetByName(ctx, repo.ID, "main")
	require.NoError(t, err)
	assert.True(t, got.EnableAllowlist)
	assert.Equal(t, strconv.FormatInt(alice.ID, 10), got.AllowlistUserIDs)
	assert.Equal(t, strconv.FormatInt(writers.ID, 10), got.AllowlistTeamIDs)
	assert.True(t, osx.IsFile(filepath.Join(repo.RepoPath(), "hooks", string(git.HookPreReceive))))

	// Repositories of users have no teams.
	userRepo := newTestRepoOnDisk(t, ctx, s, alice, "repo2")
	err = s.SaveForOrg(ctx, logx.NewNoopLogger(), &ProtectBranch{RepoID: userRepo.ID, Name: "main"}, nil, nil)
	require.Error(t, err)
	_, err = s.GetByName(ctx, userRepo.ID, "main")
	assert.True(t, IsErrBranchNotExist(err), "got %v", err)
}
