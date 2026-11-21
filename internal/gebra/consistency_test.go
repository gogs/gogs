package gebra

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

const blockedModel = `
model
  schema 1.1
type user
type doc
  relations
    define blocked: [user]
    define viewer: [user] but not blocked
`

// fallbacks returns the view fallbacks that the reader collected, by reason.
func fallbacks(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != "gebra.view.fallbacks" {
				continue
			}
			for _, dp := range sum.DataPoints {
				reason, _ := dp.Attributes.Value(attribute.Key("gebra.view.fallback"))
				out[reason.AsString()] += dp.Value
			}
		}
	}
	return out
}

// TestViewReadsAppliedTuples checks that a query that uses the view reads the
// tuples of relations that the view does not hold as of the same writes as
// the view, rather than mixing them with writes the applier has not applied.
func TestViewReadsAppliedTuples(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, blockedModel, Options{manualApply: true})
	check := func() bool {
		t.Helper()
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:alice", Relation: "viewer", Object: "doc:1"}})
		require.NoError(t, err)
		return ok
	}
	listObjects := func() []string {
		t.Helper()
		objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{Type: "doc", Relation: "viewer", User: "user:alice"})
		require.NoError(t, err)
		return objects
	}
	listUsers := func() []User {
		t.Helper()
		users, _, err := e.ListUsers(ctx, storeID, &ListUsersRequest{Object: "doc:1", Relation: "viewer", UserFilters: []UserFilter{{Type: "user"}}})
		require.NoError(t, err)
		return users
	}
	alice := []User{{Object: &UserObject{Type: "user", ID: "alice"}}}

	// No committed state allows alice: first she is blocked, then she
	// becomes a viewer while still blocked.
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "blocked", "doc:1")}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "viewer", "doc:1")}})
	assert.False(t, check())
	assert.Empty(t, listObjects())
	assert.Empty(t, listUsers())
	awaitApplied(t, e, storeID)
	assert.False(t, check())
	assert.Empty(t, listObjects())
	assert.Empty(t, listUsers())

	// While a delete is pending, the view and the tuples it reflects still
	// hold the deleted tuple.
	mustWrite(t, e, storeID, &WriteRequest{Deletes: []TupleKey{tk("user:alice", "blocked", "doc:1")}})
	awaitApplied(t, e, storeID)
	require.True(t, check())
	mustWrite(t, e, storeID, &WriteRequest{Deletes: []TupleKey{tk("user:alice", "viewer", "doc:1")}})
	assert.True(t, check())
	assert.Equal(t, []string{"doc:1"}, listObjects())
	assert.Equal(t, alice, listUsers())
	awaitApplied(t, e, storeID)
	assert.False(t, check())
	assert.Empty(t, listObjects())
	assert.Empty(t, listUsers())

	// A tuple deleted and written again with pending writes in between reads
	// as it was before the first of them.
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "viewer", "doc:1")}})
	awaitApplied(t, e, storeID)
	mustWrite(t, e, storeID, &WriteRequest{Deletes: []TupleKey{tk("user:alice", "viewer", "doc:1")}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "viewer", "doc:1")}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "blocked", "doc:1")}})
	assert.True(t, check())
	assert.Equal(t, alice, listUsers())
	awaitApplied(t, e, storeID)
	assert.False(t, check())
}

// TestCheckReadsOneSnapshot commits an applier batch between two statements
// of one check, and checks that the answer is that of the state before or
// after the batch. Mixing them grants: alice is a viewer and blocked before,
// and neither after.
func TestCheckReadsOneSnapshot(t *testing.T) {
	ctx := context.Background()
	var e *Engine
	var storeID string
	var armed atomic.Bool
	applied := make(chan error, 1)
	appliedDuringCheck := false
	e, storeID = newTestEngineWith(t, blockedModel, Options{manualApply: true, hooks: testHooks{
		afterTupleRead: func(context.Context) {
			if !armed.CompareAndSwap(true, false) {
				return
			}
			go func() {
				_, err := e.applyBatch(ctx, storeID)
				applied <- err
			}()
			// The batch commits at once unless the snapshot holds it off,
			// as on SQLite.
			select {
			case err := <-applied:
				applied <- err
				appliedDuringCheck = true
			case <-time.After(500 * time.Millisecond):
			}
		},
	}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:alice", "viewer", "doc:1"), tk("user:alice", "blocked", "doc:1")}})
	awaitApplied(t, e, storeID)
	mustWrite(t, e, storeID, &WriteRequest{Deletes: []TupleKey{tk("user:alice", "viewer", "doc:1"), tk("user:alice", "blocked", "doc:1")}})

	armed.Store(true)
	ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:alice", Relation: "viewer", Object: "doc:1"}})
	require.NoError(t, err)
	require.False(t, armed.Load(), "the check read tuples")
	require.NoError(t, <-applied)
	if e.db.dialect == DialectPostgres {
		require.True(t, appliedDuringCheck, "the batch committed while the check ran")
	}
	assert.False(t, ok, "neither the state before the batch nor the one after it allows alice")
	requireNothingPending(t, e, storeID)
}

// TestMaxStaleness checks that queries with the default consistency evaluate
// the graph once the oldest pending write is older than MaxStaleness, and use
// the view again once it caught up.
func TestMaxStaleness(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	const maxStaleness = 300 * time.Millisecond
	e, storeID := newTestEngineWith(t, testModel, Options{
		manualApply: true, MaxStaleness: maxStaleness,
		MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
	})
	check := func(c Consistency) bool {
		t.Helper()
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "member", Object: "group:eng"}, Consistency: c})
		require.NoError(t, err)
		return ok
	}
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})
	assert.False(t, check(Consistency{}), "the view lags less than MaxStaleness")
	assert.Zero(t, fallbacks(t, reader)["view_stale"])

	time.Sleep(maxStaleness + 2*oldestPendingTTL)
	assert.True(t, check(Consistency{}), "the view lags more than MaxStaleness")
	assert.Equal(t, int64(1), fallbacks(t, reader)["view_stale"])
	assert.False(t, check(Consistency{Preference: ConsistencyMinimizeLatency}), "MINIMIZE_LATENCY uses the view as it is")
	objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{Type: "group", Relation: "member", User: "user:anne"})
	require.NoError(t, err)
	assert.Equal(t, []string{"group:eng"}, objects)
	assert.Equal(t, int64(2), fallbacks(t, reader)["view_stale"])

	drainView(t, e, storeID)
	assert.True(t, check(Consistency{}))
	assert.Equal(t, int64(2), fallbacks(t, reader)["view_stale"], "the view is current again")
}

// TestApplierBackoff fails applier batches and checks that the store's
// applier retries them with growing delays, and applies the writes once a
// batch succeeds.
func TestApplierBackoff(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	const failures = 3
	var mu sync.Mutex
	var attempts []time.Time
	var fail atomic.Bool
	e, storeID := newTestEngineWith(t, testModel, Options{
		ApplyInterval: 10 * time.Millisecond,
		MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
		hooks: testHooks{
			beforeApplyCommit: func(context.Context, string) error {
				if !fail.Load() {
					return nil
				}
				mu.Lock()
				defer mu.Unlock()
				attempts = append(attempts, time.Now())
				if len(attempts) <= failures {
					return errors.New("injected failure")
				}
				return nil
			},
		},
	})
	fail.Store(true)
	_, err := e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})
	require.NoError(t, err)
	awaitAppliedWithin(t, e, storeID, 10*time.Second)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, attempts, failures+1)
	for i := 1; i < len(attempts); i++ {
		delay := minApplyBackoff << (i - 1)
		assert.GreaterOrEqual(t, attempts[i].Sub(attempts[i-1]), delay, "attempt %d", i+1)
	}
	// Waiters see the batch's commit before its applier resets the backoff.
	require.Eventually(t, func() bool {
		e.applyMu.Lock()
		defer e.applyMu.Unlock()
		return len(e.backoff) == 0
	}, 5*time.Second, time.Millisecond, "a successful batch resets the backoff")

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))
	var failed int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if sum, ok := m.Data.(metricdata.Sum[int64]); ok && m.Name == "gebra.applier.failures" {
				for _, dp := range sum.DataPoints {
					failed += dp.Value
				}
			}
		}
	}
	assert.Equal(t, int64(failures), failed)
}

// TestChecksDoNotWait checks that checks and Expand with a revision that the
// view lacks evaluate the graph at once, and that list queries wait for the
// view first.
func TestChecksDoNotWait(t *testing.T) {
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	const viewWait = 300 * time.Millisecond
	e, storeID := newTestEngineWith(t, testModel, Options{
		manualApply: true, ViewWait: viewWait,
		MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
	})
	rev := mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})

	for _, c := range []Consistency{{MinRevision: rev}, {Preference: ConsistencyHigher}} {
		start := time.Now()
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "member", Object: "group:eng"}, Consistency: c})
		require.NoError(t, err)
		assert.True(t, ok)
		assert.Less(t, time.Since(start), viewWait, "%+v", c)

		start = time.Now()
		results, err := e.BatchCheck(ctx, storeID, "", c, []BatchCheckItem{{
			TupleKey: CheckTupleKey{User: "user:anne", Relation: "member", Object: "group:eng"}, CorrelationID: "1",
		}})
		require.NoError(t, err)
		assert.True(t, results["1"].Allowed)
		assert.Less(t, time.Since(start), viewWait, "%+v", c)

		start = time.Now()
		node, err := e.Expand(ctx, storeID, &ExpandRequest{Object: "group:eng", Relation: "member", Consistency: c})
		require.NoError(t, err)
		assert.Equal(t, []string{"user:anne"}, node.Leaf.Users.Users)
		assert.Less(t, time.Since(start), viewWait, "%+v", c)
	}
	assert.Equal(t, int64(6), fallbacks(t, reader)["view_behind"])

	start := time.Now()
	objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{
		Type: "group", Relation: "member", User: "user:anne", Consistency: Consistency{MinRevision: rev},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"group:eng"}, objects)
	assert.GreaterOrEqual(t, time.Since(start), viewWait-20*time.Millisecond, "list queries wait for the view")

	node, err := e.Expand(ctx, storeID, &ExpandRequest{Object: "group:eng", Relation: "member"})
	require.NoError(t, err)
	assert.Empty(t, node.Leaf.Users.Users, "Expand reads the tuples that the view reflects")
}
