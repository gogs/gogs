package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func collaborationsAdd(t *testing.T, ctx context.Context, s *CollaborationsStore) {
	logger := logx.NewNoopLogger()
	repo := newTestIssueRepo(t, ctx, s.db, "co-add")
	other := newTestIssueRepo(t, ctx, s.db, "co-add-other")
	alice := newTestIssueUser(t, ctx, s.db, "co-add-alice")
	perms := newPermissionsStore(s.db)
	// A stale access is replaced when the accesses are recalculated.
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{9999: AccessModeAdmin}))

	err := s.Add(ctx, repo, alice.ID)
	require.NoError(t, err)
	assert.True(t, s.IsCollaborator(ctx, logger, repo.ID, alice.ID))
	assert.False(t, s.IsCollaborator(ctx, logger, other.ID, alice.ID))
	assert.False(t, s.IsCollaborator(ctx, logger, repo.ID, other.OwnerID))
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))

	// Adding a collaborator that the repository has keeps their mode.
	err = s.db.Model(new(Collaboration)).Where("user_id = ?", alice.ID).Update("mode", AccessModeRead).Error
	require.NoError(t, err)
	err = s.Add(ctx, repo, alice.ID)
	require.NoError(t, err)
	got, err := s.List(ctx, repo.ID)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, AccessModeRead, got[0].Collaboration.Mode)
	assert.Equal(t, map[int64]AccessMode{alice.ID: AccessModeWrite}, accessModes(t, ctx, perms, repo.ID))
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

	// Deleting a user who is not a collaborator does not recalculate.
	require.NoError(t, perms.SetRepoPerms(ctx, repo.ID, map[int64]AccessMode{9999: AccessModeRead}))
	err = s.Delete(ctx, repo, alice.ID)
	require.NoError(t, err)
	assert.Equal(t, map[int64]AccessMode{9999: AccessModeRead}, accessModes(t, ctx, perms, repo.ID))

	err = s.Delete(ctx, repo, bob.ID)
	require.NoError(t, err)
	assert.Empty(t, accessModes(t, ctx, perms, repo.ID))
}
