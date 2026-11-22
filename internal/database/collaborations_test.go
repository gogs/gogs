package database

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"
)

func TestCollaborations(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &CollaborationsStore{
		db: newTestDB(t, "CollaborationsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *CollaborationsStore)
	}{
		{"Add", collaborationsAdd},
		{"List", collaborationsList},
		{"ChangeMode", collaborationsChangeMode},
		{"Delete", collaborationsDelete},
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

// accessModes returns the access modes to the repository with given ID, keyed
// by user IDs.
func accessModes(t *testing.T, ctx context.Context, s *PermissionsStore, repoID int64) map[int64]AccessMode {
	t.Helper()

	var accesses []*Access
	err := s.db.WithContext(ctx).Where("repo_id = ?", repoID).Find(&accesses).Error
	require.NoError(t, err)
	modes := make(map[int64]AccessMode, len(accesses))
	for _, a := range accesses {
		modes[a.UserID] = a.Mode
	}
	return modes
}

// newTestCollabOrgRepo creates an organization and a repository it owns.
func newTestCollabOrgRepo(t *testing.T, ctx context.Context, db *gorm.DB, name string) *Repository {
	t.Helper()
	org := &User{Name: name, LowerName: name, Email: name + "@example.com", Type: UserTypeOrganization}
	require.NoError(t, db.Create(org).Error)
	repo, err := newReposStore(db).create(ctx, org.ID, CreateRepoOptions{Name: "repo1"})
	require.NoError(t, err)
	return repo
}

// newTestTeam creates a team of the organization with given repositories and
// members.
func newTestTeam(t *testing.T, db *gorm.DB, orgID int64, name string, authorize AccessMode, repos []*Repository, memberIDs ...int64) {
	t.Helper()
	team := &Team{OrgID: orgID, Name: name, LowerName: strings.ToLower(name), Authorize: authorize}
	require.NoError(t, db.Create(team).Error)
	for _, r := range repos {
		require.NoError(t, db.Create(&TeamRepo{OrgID: orgID, TeamID: team.ID, RepoID: r.ID}).Error)
	}
	for _, id := range memberIDs {
		require.NoError(t, db.Create(&TeamUser{OrgID: orgID, TeamID: team.ID, UID: id}).Error)
	}
}

// concurrently runs do for 0 to n-1 at the same time and requires that none
// fails.
func concurrently(t *testing.T, n int, do func(i int) error) {
	t.Helper()
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() { errs[i] = do(i) })
	}
	wg.Wait()
	for _, err := range errs {
		require.NoError(t, err)
	}
}

func collaborationsAdd(t *testing.T, ctx context.Context, s *CollaborationsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "co-add")
	other := newTestIssueRepo(t, ctx, s.db, "co-add-other")
	alice := newTestIssueUser(t, ctx, s.db, "co-add-alice")
	perms := newPermissionsStore(s.db)
	// Adding a collaborator changes only their own access.
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{9999: AccessModeAdmin}))

	err := s.Add(ctx, repo, alice.ID)
	require.NoError(t, err)
	assert.True(t, s.IsCollaborator(ctx, logger, repo.ID, alice.ID))
	assert.False(t, s.IsCollaborator(ctx, logger, other.ID, alice.ID))
	assert.False(t, s.IsCollaborator(ctx, logger, repo.ID, other.OwnerID))
	assert.Equal(t, map[int64]AccessMode{9999: AccessModeAdmin, alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))
	assert.Empty(t, accessModes(t, ctx, perms, other.ID))

	// A deleted repository gets no collaborators.
	deleted := newTestIssueRepo(t, ctx, s.db, "co-add-deleted")
	require.NoError(t, s.db.Delete(deleted).Error)
	err = s.Add(ctx, deleted, alice.ID)
	assert.True(t, IsErrRepoNotExist(err), "%+v", err)
	assert.False(t, s.IsCollaborator(ctx, logger, deleted.ID, alice.ID))
	assert.Empty(t, accessModes(t, ctx, perms, deleted.ID))

	// Adding a collaborator that the repository has keeps their mode.
	err = s.db.Model(new(Collaboration)).Where("user_id = ?", alice.ID).Update("mode", AccessModeRead).Error
	require.NoError(t, err)
	err = s.Add(ctx, repo, alice.ID)
	require.NoError(t, err)
	got, err := s.List(ctx, repo.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, AccessModeRead, got[0].Collaboration.Mode)
	assert.Equal(t, map[int64]AccessMode{9999: AccessModeAdmin, alice.ID: AccessModeRead}, accessModes(t, ctx, perms, repo.ID))

	// Concurrent adds each give their collaborator access, also when the same
	// user is added twice at once.
	const n = 8
	want := map[int64]AccessMode{9999: AccessModeAdmin, alice.ID: AccessModeRead}
	users := make([]*User, n)
	for i := range users {
		users[i] = newTestIssueUser(t, ctx, s.db, fmt.Sprintf("co-add-user%d", i))
		want[users[i].ID] = AccessModeWrite
	}
	concurrently(t, 2*n, func(i int) error { return s.Add(ctx, repo, users[i%n].ID) })
	assert.Equal(t, want, accessModes(t, ctx, perms, repo.ID))
	got, err = s.List(ctx, repo.ID)
	require.NoError(t, err)
	assert.Len(t, got, n+1)

	t.Run("organization", func(t *testing.T) {
		repo := newTestCollabOrgRepo(t, ctx, s.db, "co-add-org")
		otherRepo, err := newReposStore(s.db).create(ctx, repo.OwnerID, CreateRepoOptions{Name: "repo2"})
		require.NoError(t, err)
		owner := newTestIssueUser(t, ctx, s.db, "co-add-org-owner")
		admin := newTestIssueUser(t, ctx, s.db, "co-add-org-admin")
		reader := newTestIssueUser(t, ctx, s.db, "co-add-org-reader")
		elsewhere := newTestIssueUser(t, ctx, s.db, "co-add-org-elsewhere")
		// The owner team gives owner access without having the repository.
		newTestTeam(t, s.db, repo.OwnerID, ownerTeamName, AccessModeRead, nil, owner.ID)
		newTestTeam(t, s.db, repo.OwnerID, "admins", AccessModeAdmin, []*Repository{repo}, admin.ID)
		newTestTeam(t, s.db, repo.OwnerID, "readers", AccessModeRead, []*Repository{repo}, reader.ID)
		// A team without the repository gives nothing.
		newTestTeam(t, s.db, repo.OwnerID, "elsewhere", AccessModeAdmin, []*Repository{otherRepo}, elsewhere.ID)

		for _, u := range []*User{owner, admin, reader, elsewhere} {
			require.NoError(t, s.Add(ctx, repo, u.ID))
		}
		assert.Equal(t, map[int64]AccessMode{
			owner.ID:     AccessModeOwner,
			admin.ID:     AccessModeAdmin,
			reader.ID:    AccessModeWrite,
			elsewhere.ID: AccessModeWrite,
		}, accessModes(t, ctx, perms, repo.ID))
		assert.Empty(t, accessModes(t, ctx, perms, otherRepo.ID))
	})
}

func collaborationsList(t *testing.T, ctx context.Context, s *CollaborationsStore) {
	repo := newTestIssueRepo(t, ctx, s.db, "co-list")
	other := newTestIssueRepo(t, ctx, s.db, "co-list-other")
	bob := newTestIssueUser(t, ctx, s.db, "co-list-bob")
	alice := newTestIssueUser(t, ctx, s.db, "co-list-alice")
	require.NoError(t, s.Add(ctx, repo, bob.ID))
	require.NoError(t, s.Add(ctx, repo, alice.ID))
	require.NoError(t, s.Add(ctx, other, alice.ID))

	got, err := s.List(ctx, repo.ID)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, "co-list-bob", got[0].Name)
	assert.Equal(t, bob.ID, got[0].Collaboration.UserID)
	assert.Equal(t, "co-list-alice", got[1].Name)
	assert.Equal(t, AccessModeWrite, got[1].Collaboration.Mode)

	got, err = s.List(ctx, 9999)
	require.NoError(t, err)
	assert.Empty(t, got)

	// A collaborator who no longer exists is an error.
	err = s.db.Create(&Collaboration{RepoID: other.ID, UserID: 9999, Mode: AccessModeRead}).Error
	require.NoError(t, err)
	_, err = s.List(ctx, other.ID)
	assert.True(t, IsErrUserNotExist(err), "got %v", err)
}

func collaborationsChangeMode(t *testing.T, ctx context.Context, s *CollaborationsStore) {
	repo := newTestIssueRepo(t, ctx, s.db, "co-mode")
	alice := newTestIssueUser(t, ctx, s.db, "co-mode-alice")
	bob := newTestIssueUser(t, ctx, s.db, "co-mode-bob")
	require.NoError(t, s.Add(ctx, repo, alice.ID))
	perms := newPermissionsStore(s.db)

	mode := func() AccessMode {
		t.Helper()
		c := new(Collaboration)
		require.NoError(t, s.db.Where("repo_id = ? AND user_id = ?", repo.ID, alice.ID).First(c).Error)
		return c.Mode
	}

	err := s.ChangeMode(ctx, repo, AccessModeOwner, alice.ID, AccessModeAdmin)
	require.NoError(t, err)
	assert.Equal(t, AccessModeAdmin, mode())
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeAdmin}, accessModes(t, ctx, perms, repo.ID))

	err = s.ChangeMode(ctx, repo, AccessModeAdmin, alice.ID, AccessModeRead)
	require.NoError(t, err)
	assert.Equal(t, AccessModeRead, mode())
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeRead}, accessModes(t, ctx, perms, repo.ID))

	// Modes above the actor's, above admin, or none are ignored.
	for _, tc := range []struct{ actor, mode AccessMode }{
		{AccessModeRead, AccessModeWrite},
		{AccessModeOwner, AccessModeOwner},
		{AccessModeOwner, AccessModeNone},
	} {
		err = s.ChangeMode(ctx, repo, tc.actor, alice.ID, tc.mode)
		require.NoError(t, err)
		assert.Equal(t, AccessModeRead, mode(), "actor %v, mode %v", tc.actor, tc.mode)
	}

	// A missing access is created.
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, nil))
	err = s.ChangeMode(ctx, repo, AccessModeAdmin, alice.ID, AccessModeWrite)
	require.NoError(t, err)
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))

	// Changing the mode of a user who is not a collaborator is a no-op.
	err = s.ChangeMode(ctx, repo, AccessModeOwner, bob.ID, AccessModeAdmin)
	require.NoError(t, err)
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))
	assert.False(t, s.IsCollaborator(ctx, logx.NewNoopLogger(), repo.ID, bob.ID))

	t.Run("organization", func(t *testing.T) {
		repo := newTestCollabOrgRepo(t, ctx, s.db, "co-mode-org")
		otherRepo, err := newReposStore(s.db).create(ctx, repo.OwnerID, CreateRepoOptions{Name: "repo2"})
		require.NoError(t, err)
		carol := newTestIssueUser(t, ctx, s.db, "co-mode-org-carol")
		dave := newTestIssueUser(t, ctx, s.db, "co-mode-org-dave")
		newTestTeam(t, s.db, repo.OwnerID, "writers", AccessModeWrite, []*Repository{repo}, carol.ID)
		newTestTeam(t, s.db, repo.OwnerID, "elsewhere", AccessModeAdmin, []*Repository{otherRepo}, carol.ID, dave.ID)
		require.NoError(t, s.Add(ctx, repo, carol.ID))
		require.NoError(t, s.Add(ctx, repo, dave.ID))

		// The access is the highest of the new mode and the modes of the
		// teams that have the repository.
		require.NoError(t, s.ChangeMode(ctx, repo, AccessModeOwner, carol.ID, AccessModeRead))
		require.NoError(t, s.ChangeMode(ctx, repo, AccessModeOwner, dave.ID, AccessModeRead))
		assert.Equal(t, map[int64]AccessMode{
			carol.ID: AccessModeWrite,
			dave.ID:  AccessModeRead,
		}, accessModes(t, ctx, perms, repo.ID))

		require.NoError(t, s.ChangeMode(ctx, repo, AccessModeOwner, carol.ID, AccessModeAdmin))
		assert.Equal(t, AccessModeAdmin, accessModes(t, ctx, perms, repo.ID)[carol.ID])
	})
}

func collaborationsDelete(t *testing.T, ctx context.Context, s *CollaborationsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "co-delete")
	other := newTestIssueRepo(t, ctx, s.db, "co-delete-other")
	alice := newTestIssueUser(t, ctx, s.db, "co-delete-alice")
	bob := newTestIssueUser(t, ctx, s.db, "co-delete-bob")
	require.NoError(t, s.Add(ctx, repo, alice.ID))
	require.NoError(t, s.Add(ctx, repo, bob.ID))
	require.NoError(t, s.Add(ctx, other, alice.ID))
	perms := newPermissionsStore(s.db)

	err := s.Delete(ctx, repo, alice.ID)
	require.NoError(t, err)
	assert.False(t, s.IsCollaborator(ctx, logger, repo.ID, alice.ID))
	assert.True(t, s.IsCollaborator(ctx, logger, other.ID, alice.ID))
	assert.Equal(t, map[int64]AccessMode{bob.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, other.ID))

	// Deleting a collaborator changes only their own access, and deleting a
	// user who is not a collaborator changes nothing.
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{9999: AccessModeRead, alice.ID: AccessModeAdmin, bob.ID: AccessModeWrite}))
	err = s.Delete(ctx, repo, alice.ID)
	require.NoError(t, err)
	err = s.Delete(ctx, repo, bob.ID)
	require.NoError(t, err)
	assert.Equal(t, map[int64]AccessMode{9999: AccessModeRead, alice.ID: AccessModeAdmin}, accessModes(t, ctx, perms, repo.ID))

	// Concurrent adds and deletes each change their own collaborator.
	const n = 8
	users := make([]*User, 2*n)
	for i := range users {
		users[i] = newTestIssueUser(t, ctx, s.db, fmt.Sprintf("co-delete-user%d", i))
		if i >= n {
			require.NoError(t, s.Add(ctx, repo, users[i].ID))
		}
	}
	concurrently(t, 2*n, func(i int) error {
		if i < n {
			return s.Add(ctx, repo, users[i].ID)
		}
		return s.Delete(ctx, repo, users[i].ID)
	})
	want := map[int64]AccessMode{9999: AccessModeRead, alice.ID: AccessModeAdmin}
	for _, u := range users[:n] {
		want[u.ID] = AccessModeWrite
	}
	assert.Equal(t, want, accessModes(t, ctx, perms, repo.ID))

	t.Run("organization", func(t *testing.T) {
		repo := newTestCollabOrgRepo(t, ctx, s.db, "co-delete-org")
		owner := newTestIssueUser(t, ctx, s.db, "co-delete-org-owner")
		writer := newTestIssueUser(t, ctx, s.db, "co-delete-org-writer")
		plain := newTestIssueUser(t, ctx, s.db, "co-delete-org-plain")
		newTestTeam(t, s.db, repo.OwnerID, ownerTeamName, AccessModeOwner, nil, owner.ID)
		newTestTeam(t, s.db, repo.OwnerID, "writers", AccessModeWrite, []*Repository{repo}, writer.ID)
		for _, u := range []*User{owner, writer, plain} {
			require.NoError(t, s.Add(ctx, repo, u.ID))
			require.NoError(t, s.ChangeMode(ctx, repo, AccessModeOwner, u.ID, AccessModeAdmin))
		}

		// A deleted collaborator keeps the access of their teams.
		for _, u := range []*User{owner, writer, plain} {
			require.NoError(t, s.Delete(ctx, repo, u.ID))
		}
		assert.Equal(t, map[int64]AccessMode{
			owner.ID:  AccessModeOwner,
			writer.ID: AccessModeWrite,
		}, accessModes(t, ctx, perms, repo.ID))
	})
}
