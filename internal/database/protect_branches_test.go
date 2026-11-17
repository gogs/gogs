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
	t.Parallel()

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
		{"CheckPush", protectBranchesCheckPush},
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

func protectBranchesCheckPush(t *testing.T, ctx context.Context, s *ProtectBranchesStore) {
	const repoID = 1
	team := &Team{OrgID: 10, LowerName: "writers", Name: "writers", Authorize: AccessModeWrite}
	require.NoError(t, s.db.Create(team).Error)
	require.NoError(t, s.db.Create(&TeamRepo{OrgID: 10, TeamID: team.ID, RepoID: repoID}).Error)
	require.NoError(t, s.db.Create(&TeamUser{OrgID: 10, TeamID: team.ID, UID: 9}).Error)
	// The team of an organization the repository was transferred from, which
	// no longer has the repository.
	oldTeam := &Team{OrgID: 11, LowerName: "old", Name: "old", Authorize: AccessModeWrite}
	require.NoError(t, s.db.Create(oldTeam).Error)
	require.NoError(t, s.db.Create(&TeamUser{OrgID: 11, TeamID: oldTeam.ID, UID: 8}).Error)

	for _, pb := range []*ProtectBranch{
		{RepoID: repoID, Name: "open", Protected: true},
		{RepoID: repoID, Name: "pulls", Protected: true, RequirePullRequest: true},
		{RepoID: repoID, Name: "unprotected", Protected: false, RequirePullRequest: true, EnableAllowlist: true},
		{
			RepoID: repoID, Name: "allowlist", Protected: true, RequirePullRequest: true, EnableAllowlist: true,
			AllowlistUserIDs: "5", AllowlistTeamIDs: strconv.FormatInt(team.ID, 10) + "," + strconv.FormatInt(oldTeam.ID, 10),
		},
	} {
		require.NoError(t, s.db.Create(pb).Error)
	}

	for _, tc := range []struct {
		name           string
		branch         string
		userID         int64
		viaPullRequest bool
		want           PushDenial
	}{
		{name: "no protection", branch: "none", userID: 6},
		{name: "protection turned off", branch: "unprotected", userID: 6},
		{name: "protected without rules", branch: "open", userID: 6},
		{name: "push to pull request branch", branch: "pulls", userID: 6, want: PushDeniedRequiresPullRequest},
		{name: "merge into pull request branch", branch: "pulls", userID: 6, viaPullRequest: true},
		{name: "listed user pushes past pull requests", branch: "allowlist", userID: 5},
		{name: "member of listed team", branch: "allowlist", userID: 9},
		{name: "member of team without the repository", branch: "allowlist", userID: 8, want: PushDeniedNotInAllowlist},
		{name: "unlisted user", branch: "allowlist", userID: 6, want: PushDeniedNotInAllowlist},
		{name: "unlisted user merges", branch: "allowlist", userID: 6, viaPullRequest: true, want: PushDeniedNotInAllowlist},
		{name: "no user", branch: "allowlist", userID: 0, want: PushDeniedNotInAllowlist},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.CheckPushByName(ctx, repoID, tc.branch, tc.userID, tc.viaPullRequest)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

// newTestRepoOnDisk creates a repository of given owner in the database and
// the directory of its hooks on disk, and returns it with its owner loaded.
//
// NOTE: Owners must have names that no other test uses, because tests that
// delete users remove their directories under the repository root that is
// mocked at that time.
func newTestRepoOnDisk(t *testing.T, ctx context.Context, s *ProtectBranchesStore, owner *User, name string) *Repository {
	t.Helper()

	repo, err := newReposStore(s.db).create(ctx, owner.ID, CreateRepoOptions{Name: name})
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
