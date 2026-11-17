package database

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

func TestMirrors(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	s := &MirrorsStore{
		db: newTestDB(t, "MirrorsStore"),
	}

	for _, tc := range []struct {
		name string
		test func(t *testing.T, ctx context.Context, s *MirrorsStore)
	}{
		{"GetByRepoID", mirrorsGetByRepoID},
		{"LoadForRepos", mirrorsLoadForRepos},
		{"Update", mirrorsUpdate},
		{"DeleteByRepoID", mirrorsDeleteByRepoID},
		{"listDueRepoIDs", mirrorsListDueRepoIDs},
		{"markSynced", mirrorsMarkSynced},
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

// newTestMirror creates a mirror repository of a new user, and its mirror that
// syncs every given hours and is next due at given time.
func newTestMirror(t *testing.T, ctx context.Context, s *MirrorsStore, name string, interval int, nextSync time.Time) *Mirror {
	t.Helper()

	owner, err := newUsersStore(s.db).Create(ctx, "mirror-"+name, "mirror-"+name+"@example.com", CreateUserOptions{})
	require.NoError(t, err)
	repo, err := newReposStore(s.db).Create(ctx, owner.ID, CreateRepoOptions{Name: name, Mirror: true})
	require.NoError(t, err)
	m := &Mirror{RepoID: repo.ID, Interval: interval, EnablePrune: true, NextSync: nextSync}
	err = s.db.Create(m).Error
	require.NoError(t, err)
	return m
}

func mirrorsGetByRepoID(t *testing.T, ctx context.Context, s *MirrorsStore) {
	nextSync := time.Unix(1_800_000_000, 0)
	m := newTestMirror(t, ctx, s, "repo1", 8, nextSync)

	got, err := s.GetByRepoID(ctx, m.RepoID)
	require.NoError(t, err)
	assert.Equal(t, m.ID, got.ID)
	assert.Equal(t, 8, got.Interval)
	assert.Equal(t, nextSync.Unix(), got.NextSync.Unix())
	require.NotNil(t, got.Repo)
	assert.Equal(t, m.RepoID, got.Repo.ID)
	require.NotNil(t, got.Repo.Owner)
	assert.Equal(t, "mirror-repo1", got.Repo.Owner.Name)

	_, err = s.GetByRepoID(ctx, 404)
	assert.True(t, IsMirrorNotExist(err), "got %v", err)
}

func mirrorsLoadForRepos(t *testing.T, ctx context.Context, s *MirrorsStore) {
	m1 := newTestMirror(t, ctx, s, "repo1", 8, time.Now())
	m2 := newTestMirror(t, ctx, s, "repo2", 24, time.Now())

	reposStore := newReposStore(s.db)
	repo1, err := reposStore.GetByID(ctx, m1.RepoID)
	require.NoError(t, err)
	repo2, err := reposStore.GetByID(ctx, m2.RepoID)
	require.NoError(t, err)
	// A mirror whose mirror settings are gone.
	repo3, err := reposStore.Create(ctx, repo1.OwnerID, CreateRepoOptions{Name: "repo3", Mirror: true})
	require.NoError(t, err)
	// A regular repository whose ID has a mirror is not looked up.
	regular := &Repository{ID: m2.RepoID}

	err = s.LoadForRepos(ctx, repo1, repo2, repo3, regular)
	require.NoError(t, err)
	require.NotNil(t, repo1.Mirror)
	assert.Equal(t, m1.ID, repo1.Mirror.ID)
	assert.Same(t, repo1, repo1.Repo)
	require.NotNil(t, repo2.Mirror)
	assert.Equal(t, 24, repo2.Interval)
	assert.Nil(t, repo3.Mirror)
	assert.Nil(t, regular.Mirror)
}

func mirrorsUpdate(t *testing.T, ctx context.Context, s *MirrorsStore) {
	m := newTestMirror(t, ctx, s, "repo1", 8, time.Now())
	got, err := s.GetByRepoID(ctx, m.RepoID)
	require.NoError(t, err)

	nextSync := time.Unix(1_900_000_000, 0)
	got.Interval = 48
	got.EnablePrune = false
	got.NextSync = nextSync
	err = s.Update(ctx, got)
	require.NoError(t, err)

	got, err = s.GetByRepoID(ctx, m.RepoID)
	require.NoError(t, err)
	assert.Equal(t, 48, got.Interval)
	assert.False(t, got.EnablePrune)
	assert.Equal(t, nextSync.Unix(), got.NextSyncUnix)
}

func mirrorsDeleteByRepoID(t *testing.T, ctx context.Context, s *MirrorsStore) {
	m1 := newTestMirror(t, ctx, s, "repo1", 8, time.Now())
	m2 := newTestMirror(t, ctx, s, "repo2", 8, time.Now())

	err := s.DeleteByRepoID(ctx, m1.RepoID)
	require.NoError(t, err)
	_, err = s.GetByRepoID(ctx, m1.RepoID)
	assert.True(t, IsMirrorNotExist(err), "got %v", err)
	_, err = s.GetByRepoID(ctx, m2.RepoID)
	require.NoError(t, err)

	// Deleting a mirror that does not exist is not an error.
	err = s.DeleteByRepoID(ctx, m1.RepoID)
	require.NoError(t, err)
}

func mirrorsListDueRepoIDs(t *testing.T, ctx context.Context, s *MirrorsStore) {
	now := time.Unix(1_800_000_000, 0)
	due := newTestMirror(t, ctx, s, "due", 8, now.Add(-time.Hour))
	dueNow := newTestMirror(t, ctx, s, "due-now", 8, now)
	newTestMirror(t, ctx, s, "later", 8, now.Add(time.Second))
	disconnected := newTestMirror(t, ctx, s, "disconnected", 8, now.Add(-time.Hour))
	err := s.db.Where("id = ?", disconnected.RepoID).Delete(new(Repository)).Error
	require.NoError(t, err)
	// More due mirrors than any supported database binds parameters in one
	// statement.
	const manyDue = 65_536
	many := make([]*Mirror, 0, manyDue)
	for i := range manyDue {
		many = append(many, &Mirror{RepoID: 1_000_000 + int64(i), Interval: 8, NextSync: now.Add(-time.Hour)})
	}
	err = s.db.CreateInBatches(many, 1000).Error
	require.NoError(t, err)

	got, err := s.listDueRepoIDs(ctx, logx.NewNoopLogger(), now)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{due.RepoID, dueNow.RepoID}, got)
}

func mirrorsMarkSynced(t *testing.T, ctx context.Context, s *MirrorsStore) {
	nextSync := time.Unix(1_800_000_000, 0)
	m1 := newTestMirror(t, ctx, s, "repo1", 8, nextSync)
	m2 := newTestMirror(t, ctx, s, "repo2", 8, nextSync)

	syncedAt := time.Unix(1_700_000_000, 0)
	err := s.markSynced(ctx, m1.RepoID, syncedAt)
	require.NoError(t, err)

	got, err := s.GetByRepoID(ctx, m1.RepoID)
	require.NoError(t, err)
	assert.Equal(t, syncedAt.Unix(), got.LastSync.Unix())
	assert.Equal(t, nextSync.Unix(), got.NextSync.Unix(), "next sync changed")

	got, err = s.GetByRepoID(ctx, m2.RepoID)
	require.NoError(t, err)
	assert.NotEqual(t, syncedAt.Unix(), got.LastSync.Unix())
}
