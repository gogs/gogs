package gebra

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func storeChangeIDs(t *testing.T, e *Engine, storeID string) []int64 {
	t.Helper()
	ids, err := queryColumn[int64](context.Background(), e.db.handle(), "SELECT id FROM gebra_change WHERE store_id = ? ORDER BY id", storeID)
	require.NoError(t, err)
	return ids
}

func TestChangeRetention(t *testing.T) {
	ctx := context.Background()
	retention := 24 * time.Hour
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, ChangeRetention: retention})

	_, emptyToken, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{})
	require.NoError(t, err)
	for _, user := range []string{"user:anne", "user:bob", "user:carl", "user:dana"} {
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk(user, "viewer", "doc:1")}})
	}
	awaitApplied(t, e, storeID)
	ids := storeChangeIDs(t, e, storeID)
	require.Len(t, ids, 4)

	now := time.Now().UTC().Truncate(time.Second)
	before := now.Add(-retention)
	// The third change is of an old write that was applied late.
	for i, createdAt := range []time.Time{before.Add(-time.Second), before, before.Add(-time.Hour), now.Add(-time.Hour)} {
		_, err := e.db.handle().exec(ctx, "UPDATE gebra_change SET created_at = ? WHERE id = ?", createdAt, ids[i])
		require.NoError(t, err)
	}
	_, firstToken, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{PageSize: 1})
	require.NoError(t, err)

	require.NoError(t, e.maintain(ctx, now))
	assert.Equal(t, ids[1:], storeChangeIDs(t, e, storeID),
		"the change exactly at the cutoff is kept, and so is the old change after it")

	_, _, err = e.ReadChanges(ctx, storeID, &ReadChangesRequest{ContinuationToken: emptyToken})
	assert.True(t, errors.Is(err, ErrChangesExpired), "want ErrChangesExpired, got %v", err)
	assert.False(t, errors.Is(err, ErrInvalidArgument))

	// The token of the last deleted change misses nothing.
	changes, _, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{ContinuationToken: firstToken})
	require.NoError(t, err)
	require.Len(t, changes, 3)
	assert.Equal(t, "user:bob", changes[0].TupleKey.User)

	changes, next, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{})
	require.NoError(t, err)
	require.Len(t, changes, 3)
	assert.Equal(t, "user:bob", changes[0].TupleKey.User)
	_, _, err = e.ReadChanges(ctx, storeID, &ReadChangesRequest{ContinuationToken: next})
	require.NoError(t, err)

	// Another run deletes nothing more, and a later one deletes the change
	// that was exactly at the cutoff and the old change after it.
	require.NoError(t, e.maintain(ctx, now))
	assert.Equal(t, ids[1:], storeChangeIDs(t, e, storeID))
	require.NoError(t, e.maintain(ctx, now.Add(time.Second)))
	assert.Equal(t, ids[3:], storeChangeIDs(t, e, storeID))
	_, _, err = e.ReadChanges(ctx, storeID, &ReadChangesRequest{ContinuationToken: firstToken})
	assert.True(t, errors.Is(err, ErrChangesExpired), "want ErrChangesExpired, got %v", err)
}

func TestChangeRetentionDisabled(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, ChangeRetention: -1})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "viewer", "doc:1")}})
	awaitApplied(t, e, storeID)
	require.NoError(t, e.maintain(ctx, time.Now().Add(100*365*24*time.Hour)))
	assert.Len(t, storeChangeIDs(t, e, storeID), 1)
}

func TestDeleteInBatches(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true})
	for i := range 7 {
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:u"+strconv.Itoa(i), "viewer", "doc:1")}})
	}
	awaitApplied(t, e, storeID)
	require.Len(t, storeChangeIDs(t, e, storeID), 7)
	require.NoError(t, e.deleteInBatches(ctx, "gebra_change", 3, "store_id = ?", storeID))
	assert.Empty(t, storeChangeIDs(t, e, storeID))
}

// storeRowCounts returns how many rows of the store each table that a
// deleted store must not keep holds.
func storeRowCounts(t *testing.T, e *Engine, storeID string) map[string]int64 {
	t.Helper()
	counts := map[string]int64{}
	for _, table := range []string{
		"gebra_inbox", "gebra_pending", "gebra_change", "gebra_change_cutoff", "gebra_view_state",
		"gebra_view_member", "gebra_view_edge", "gebra_view_reach", "gebra_view_gc",
	} {
		var n int64
		_, err := e.db.handle().queryRow(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE store_id = ?", []any{storeID}, &n)
		require.NoError(t, err)
		counts[table] = n
	}
	return counts
}

func TestDeleteStorePurges(t *testing.T) {
	ctx := context.Background()
	var e *Engine
	var raceStoreID atomic.Value
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, ChangeRetention: time.Hour, hooks: testHooks{
		beforeWriteCommit: func(ctx context.Context, _ *WriteRequest) {
			if id, _ := raceStoreID.Load().(string); id != "" {
				raceStoreID.Store("")
				// The write read the store before it was deleted and commits
				// after it.
				assert.NoError(t, e.DeleteStore(ctx, id))
			}
		},
	}})

	otherStore, err := e.CreateStore(ctx, "other")
	require.NoError(t, err)
	model, err := ParseDSL(testModel)
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, otherStore.ID, model)
	require.NoError(t, err)
	for _, id := range []string{storeID, otherStore.ID} {
		mustWrite(t, e, id, &WriteRequest{Writes: []TupleKey{
			tk("group:eng#member", "viewer", "folder:1"), tk("user:anne", "member", "group:eng"),
		}})
		awaitApplied(t, e, id)
	}
	require.NoError(t, e.maintain(ctx, time.Now().Add(2*time.Hour)))
	// A new model retires the store's view generation, which maintenance
	// keeps for a while after the flip.
	_, err = e.WriteAuthorizationModel(ctx, storeID, model)
	require.NoError(t, err)
	awaitApplied(t, e, storeID)
	for _, id := range []string{storeID, otherStore.ID} {
		mustWrite(t, e, id, &WriteRequest{Writes: []TupleKey{tk("user:bob", "viewer", "doc:1")}})
		awaitApplied(t, e, id)
		mustWrite(t, e, id, &WriteRequest{Writes: []TupleKey{tk("user:carl", "viewer", "doc:1")}})
	}
	for table, n := range storeRowCounts(t, e, storeID) {
		require.NotZero(t, n, "%s has rows of the store before it is deleted", table)
	}
	otherBefore := storeRowCounts(t, e, otherStore.ID)

	require.NoError(t, e.DeleteStore(ctx, storeID))
	require.NoError(t, e.maintain(ctx, time.Now()))
	for table, n := range storeRowCounts(t, e, storeID) {
		assert.Zero(t, n, "%s has rows of the deleted store", table)
	}
	var tuples int64
	_, err = e.db.handle().queryRow(ctx, "SELECT COUNT(*) FROM gebra_tuple WHERE store_id = ?", []any{storeID}, &tuples)
	require.NoError(t, err)
	assert.Equal(t, int64(4), tuples, "tuples of the deleted store stay")
	assert.Equal(t, otherBefore, storeRowCounts(t, e, otherStore.ID), "other stores keep their data")

	t.Run("racing write", func(t *testing.T) {
		st, err := e.CreateStore(ctx, "racing")
		require.NoError(t, err)
		_, err = e.WriteAuthorizationModel(ctx, st.ID, model)
		require.NoError(t, err)
		awaitApplied(t, e, st.ID)
		write := &WriteRequest{Writes: []TupleKey{tk("user:anne", "viewer", "doc:1")}}
		if e.db.dialect == DialectSQLite {
			// SQLite runs one write transaction at a time, so the deletion
			// commits before the write starts, and the inbox row is what a
			// write that read the store earlier would leave.
			require.NoError(t, e.DeleteStore(ctx, st.ID))
			require.NoError(t, e.maintain(ctx, time.Now()))
			payload := `[{"op":"` + OperationWrite + `","ot":"doc","oi":"1","r":"viewer","ut":"user","ui":"anne"}]`
			_, err = e.db.handle().exec(ctx, "INSERT INTO gebra_inbox (store_id, kind, changes, tuples, created_at) VALUES (?, ?, ?, ?, ?)",
				st.ID, inboxTuples, payload, 1, time.Now().UTC())
			require.NoError(t, err)
		} else {
			raceStoreID.Store(st.ID)
			revision, err := e.Write(ctx, st.ID, write)
			require.NoError(t, err)
			require.NotEmpty(t, revision)
		}
		_, err = e.GetStore(ctx, st.ID)
		require.True(t, errors.Is(err, ErrNotFound), "the store is deleted")
		require.NotZero(t, storeRowCounts(t, e, st.ID)["gebra_inbox"], "the write left a pending write")

		require.NoError(t, e.maintain(ctx, time.Now()))
		for table, n := range storeRowCounts(t, e, st.ID) {
			assert.Zero(t, n, "%s has rows of the deleted store", table)
		}
	})
}

func TestMySQLLockName(t *testing.T) {
	storeID := newID()
	name := mysqlLockName("gogs", storeID)
	assert.LessOrEqual(t, len(name), 64)
	assert.True(t, strings.HasSuffix(name, storeID), "the name %q shows the store ID", name)
	assert.Equal(t, name, mysqlLockName("gogs", storeID))
	assert.NotEqual(t, name, mysqlLockName("gogs_staging", storeID), "databases on one server use different names")
	assert.NotEqual(t, name, mysqlLockName("gogs", newID()))

	long := strings.Repeat("x", 100)
	assert.LessOrEqual(t, len(mysqlLockName(strings.Repeat("d", 64), long)), 64)
	assert.NotEqual(t, mysqlLockName("gogs", long), mysqlLockName("gogs", long+"y"))
}
