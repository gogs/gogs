package gebra

import (
	"context"
	"math/rand/v2"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"unknwon.dev/x/logx"
)

// testModelVariant returns testModel with one definition replaced.
func testModelVariant(t *testing.T, old, replacement string) *AuthorizationModel {
	t.Helper()
	dsl := strings.Replace(testModel, old, replacement, 1)
	require.NotEqual(t, testModel, dsl)
	m, err := ParseDSL(dsl)
	require.NoError(t, err)
	return m
}

// withoutOwnerViewers is testModel where folder owners are not viewers.
func withoutOwnerViewers(t *testing.T) *AuthorizationModel {
	return testModelVariant(t, "define viewer: [user, group#member] or owner or viewer from parent",
		"define viewer: [user, group#member] or viewer from parent")
}

// withoutEditorViewers is testModel where doc editors are not viewers.
func withoutEditorViewers(t *testing.T) *AuthorizationModel {
	return testModelVariant(t, "define viewer: [user, user:*] or editor or viewer from parent",
		"define viewer: [user, user:*] or viewer from parent")
}

// genRows returns how many view rows of the store's generation the tables
// hold.
func genRows(t *testing.T, db dbtx, storeID string, gen int64) int64 {
	t.Helper()
	var total int64
	for _, table := range []string{"gebra_view_member", "gebra_view_edge", "gebra_view_reach"} {
		var n int64
		_, err := db.queryRow(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE store_id = ? AND gen = ?", []any{storeID, gen}, &n)
		require.NoError(t, err)
		total += n
	}
	return total
}

// viewGens returns the generations that the store's view tables hold rows of.
func viewGens(t *testing.T, db dbtx, storeID string) []int64 {
	t.Helper()
	gens, err := queryColumn[int64](context.Background(), db, `SELECT gen FROM gebra_view_member WHERE store_id = ?
		UNION SELECT gen FROM gebra_view_edge WHERE store_id = ?
		UNION SELECT gen FROM gebra_view_reach WHERE store_id = ? ORDER BY gen`, storeID, storeID, storeID)
	require.NoError(t, err)
	return gens
}

// writeAllCandidates writes every tuple of viewCandidates.
func writeAllCandidates(t *testing.T, e *Engine, storeID string) {
	t.Helper()
	_, _, _, _, candidates := viewCandidates()
	mustWrite(t, e, storeID, &WriteRequest{Writes: candidates})
}

// TestViewBuildUnderConcurrentWrites writes a model while writers run, builds
// its view in chunks of three tuples or sets, and checks that until the flip
// default queries answer under the old model from the view, that the view
// after the flip equals a full build at the applied state, and that every
// query answers as graph evaluation does.
func TestViewBuildUnderConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	const chunkSize = 3
	var armed atomic.Bool
	var chunks, reachChunks, oversized atomic.Int64
	paused, release := make(chan struct{}), make(chan struct{})
	var pauseOnce sync.Once
	e, storeID := newTestEngineWith(t, testModel, Options{buildChunkSize: chunkSize, hooks: testHooks{
		beforeBuildChunk: func(context.Context, string, int64) {
			if armed.Load() && chunks.Load() >= 4 {
				pauseOnce.Do(func() {
					close(paused)
					<-release
				})
			}
		},
		beforeBuildCommit: func(_ context.Context, c buildChunkInfo) error {
			if !armed.Load() {
				return nil
			}
			chunks.Add(1)
			if c.phase == buildReach {
				reachChunks.Add(1)
			}
			if c.items > chunkSize {
				oversized.Add(1)
			}
			return nil
		},
	}})
	oldModel := servingState(t, e.db.handle(), storeID).ModelID
	writeAllCandidates(t, e, storeID)
	solo := CheckTupleKey{User: "user:solo", Relation: "viewer", Object: "folder:solo"}
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:solo", "owner", "folder:solo")}})

	_, _, _, _, candidates := viewCandidates()
	stop := make(chan struct{})
	var g errgroup.Group
	for w := range 4 {
		rng := rand.New(rand.NewPCG(uint64(w), 21))
		g.Go(func() error {
			for {
				select {
				case <-stop:
					return nil
				default:
				}
				if _, err := e.Write(ctx, storeID, randomWrite(rng, candidates)); err != nil {
					return err
				}
			}
		})
	}

	armed.Store(true)
	// The graph model has the new model's definition but never goes live,
	// so queries that name it evaluate the graph.
	newModel := withoutOwnerViewers(t)
	graphModel, err := e.WriteAuthorizationModel(ctx, storeID, newModel)
	require.NoError(t, err)
	newModelID, err := e.WriteAuthorizationModel(ctx, storeID, newModel)
	require.NoError(t, err)
	select {
	case <-paused:
	case <-time.After(time.Minute):
		t.Fatal("the build did not reach its fifth chunk")
	}

	ev, err := e.newEvaluator(ctx, storeID, "", Consistency{}, nil, nil, false)
	require.NoError(t, err)
	assert.True(t, ev.useView, "default queries use the view before the flip")
	assert.Equal(t, oldModel, ev.m.id, "default queries use the live model before the flip")
	ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: solo})
	require.NoError(t, err)
	assert.True(t, ok, "folder owners are viewers in the live model")
	ok, err = e.Check(ctx, storeID, &CheckRequest{TupleKey: solo, AuthorizationModelID: newModelID})
	require.NoError(t, err)
	assert.False(t, ok, "folder owners are not viewers in the new model")
	objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{Type: "folder", Relation: "viewer", User: "user:solo"})
	require.NoError(t, err)
	// Writers can make every user a viewer of other folders, through
	// group:g3, which has user:* as a member.
	assert.Contains(t, objects, "folder:solo")
	objects, _, err = e.ListObjects(ctx, storeID, &ListObjectsRequest{Type: "folder", Relation: "viewer", User: "user:solo", AuthorizationModelID: newModelID})
	require.NoError(t, err)
	assert.NotContains(t, objects, "folder:solo")
	status, err := e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	assert.Equal(t, oldModel, status.AuthorizationModelID)
	require.NotNil(t, status.Migration)
	assert.Equal(t, newModelID, status.Migration.TargetModelID)
	assert.Equal(t, oldModel, status.Migration.LiveModelID)
	assert.Contains(t, []string{"pending", "building_members", "building_reach"}, status.Migration.Phase)
	close(release)

	require.Eventually(t, func() bool { return modelLive(t, e, storeID) }, time.Minute, 10*time.Millisecond)
	close(stop)
	require.NoError(t, g.Wait())
	awaitApplied(t, e, storeID)

	ok, err = e.Check(ctx, storeID, &CheckRequest{TupleKey: solo})
	require.NoError(t, err)
	assert.False(t, ok, "the new model is live")
	assert.Zero(t, oversized.Load(), "every chunk covers at most the chunk size")
	assert.Greater(t, chunks.Load(), int64(15), "the build runs in many chunks")
	assert.Greater(t, reachChunks.Load(), int64(1))
	requireNothingPending(t, e, storeID)
	require.Equal(t, rebuiltSnapshotFor(t, e, storeID, newModelID), viewSnapshot(t, e.db.handle(), storeID))
	requireViewCounts(t, e.db.handle(), storeID)
	requireChangeLogMatchesTuples(t, e, storeID)
	compareWithGraph(t, e, storeID, graphModel)

	live := servingState(t, e.db.handle(), storeID).Gen
	require.NoError(t, e.maintain(ctx, time.Now().Add(2*viewGCGrace)))
	assert.Equal(t, []int64{live}, viewGens(t, e.db.handle(), storeID), "maintenance deletes every other generation")
	var retired int64
	_, err = e.db.handle().queryRow(ctx, "SELECT COUNT(*) FROM gebra_view_gc WHERE store_id = ?", []any{storeID}, &retired)
	require.NoError(t, err)
	assert.Zero(t, retired)
}

// TestViewBuildCancelled starts a build for one model, cancels it with a
// newer model while a chunk of it waits to start, and checks that the chunk
// writes nothing, that the newer model goes live, and that maintenance
// deletes the cancelled build.
func TestViewBuildCancelled(t *testing.T) {
	ctx := context.Background()
	var block atomic.Int64
	entered, release := make(chan struct{}), make(chan struct{})
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, buildChunkSize: 2, hooks: testHooks{
		beforeBuildChunk: func(_ context.Context, _ string, gen int64) {
			if block.CompareAndSwap(gen, 0) {
				close(entered)
				<-release
			}
		},
	}})
	oldGen := servingState(t, e.db.handle(), storeID).Gen
	writeAllCandidates(t, e, storeID)
	drainView(t, e, storeID)

	bID, err := e.WriteAuthorizationModel(ctx, storeID, withoutOwnerViewers(t))
	require.NoError(t, err)
	for range 3 {
		more, err := e.applyBatch(ctx, storeID)
		require.NoError(t, err)
		require.True(t, more)
	}
	vs := servingState(t, e.db.handle(), storeID)
	require.Equal(t, bID, vs.BuildModelID)
	bGen := vs.BuildGen

	// The next chunk of B was chosen before C is written, and waits.
	block.Store(bGen)
	late := make(chan error, 1)
	go func() {
		_, err := e.applyBatch(ctx, storeID)
		late <- err
	}()
	<-entered
	bRows := genRows(t, e.db.handle(), storeID, bGen)
	require.NotZero(t, bRows)

	cID, err := e.WriteAuthorizationModel(ctx, storeID, withoutEditorViewers(t))
	require.NoError(t, err)
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:solo", "editor", "doc:solo")}})
	more, err := e.applyBatch(ctx, storeID)
	require.NoError(t, err)
	require.True(t, more)
	status, err := e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	require.NotNil(t, status.Migration)
	assert.Equal(t, cID, status.Migration.TargetModelID, "C replaced the build of B")
	drainView(t, e, storeID)

	close(release)
	require.NoError(t, <-late)
	assert.Equal(t, bRows, genRows(t, e.db.handle(), storeID, bGen), "the cancelled build's late chunk wrote nothing")

	vs = servingState(t, e.db.handle(), storeID)
	assert.Equal(t, cID, vs.ModelID)
	require.Equal(t, rebuiltSnapshotFor(t, e, storeID, cID), viewSnapshot(t, e.db.handle(), storeID))
	requireViewCounts(t, e.db.handle(), storeID)
	ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:solo", Relation: "viewer", Object: "doc:solo"}})
	require.NoError(t, err)
	assert.False(t, ok, "doc editors are not viewers in C")

	require.NoError(t, e.maintain(ctx, time.Now()))
	assert.Zero(t, genRows(t, e.db.handle(), storeID, bGen), "maintenance deleted the cancelled build at once")
	assert.NotZero(t, genRows(t, e.db.handle(), storeID, oldGen), "the former serving generation stays for a while")
	require.NoError(t, e.maintain(ctx, time.Now().Add(2*viewGCGrace)))
	assert.Equal(t, []int64{vs.Gen}, viewGens(t, e.db.handle(), storeID))
}

// TestFirstModelView checks that a store's queries evaluate the graph with its
// first model until the view for it is complete.
func TestFirstModelView(t *testing.T) {
	ctx := context.Background()
	db, dialect := newTestDB(t, testSuite(t.Name()))
	e, err := New(ctx, logx.NewNoopLogger(), db, dialect, Options{manualApply: true, buildChunkSize: 1})
	require.NoError(t, err)
	t.Cleanup(e.Close)
	st, err := e.CreateStore(ctx, "first")
	require.NoError(t, err)
	m, err := ParseDSL(testModel)
	require.NoError(t, err)
	modelID, err := e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	mustWrite(t, e, st.ID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng"), tk("group:eng#member", "viewer", "folder:plans")}})

	check := func() bool {
		t.Helper()
		ok, err := e.Check(ctx, st.ID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "folder:plans"}})
		require.NoError(t, err)
		return ok
	}
	status, err := e.ViewStatus(ctx, st.ID)
	require.NoError(t, err)
	assert.Empty(t, status.AuthorizationModelID)
	require.NotNil(t, status.Migration)
	assert.Equal(t, "pending", status.Migration.Phase)
	assert.Equal(t, modelID, status.Migration.TargetModelID)
	assert.True(t, check(), "the graph answers with the first model")

	more, err := e.applyBatch(ctx, st.ID)
	require.NoError(t, err)
	require.True(t, more)
	ev, err := e.newEvaluator(ctx, st.ID, "", Consistency{}, nil, nil, false)
	require.NoError(t, err)
	assert.False(t, ev.useView, "no view is live before the first build completes")
	assert.True(t, check())
	status, err = e.ViewStatus(ctx, st.ID)
	require.NoError(t, err)
	require.NotNil(t, status.Migration)
	assert.Equal(t, "building_members", status.Migration.Phase)
	assert.Equal(t, int64(1), status.Migration.DoneChunks)
	assert.Equal(t, int64(2), status.Migration.TotalChunks)

	drainView(t, e, st.ID)
	assert.True(t, modelLive(t, e, st.ID))
	assert.True(t, check())
	require.Equal(t, rebuiltSnapshot(t, e, st.ID), viewSnapshot(t, e.db.handle(), st.ID))
}

// TestViewBuildFailover kills the applier's session in the middle of a view
// build while writes continue, and checks that the other engine resumes the
// build where it stopped and ends with an exact view.
func TestViewBuildFailover(t *testing.T) {
	skipOnSQLite(t)
	ctx := context.Background()
	db, dialect := newTestDB(t, testSuite(t.Name()))
	var stallModel atomic.Value
	stallModel.Store("")
	var firstChunks atomic.Int64
	stalled := make(chan int64, 1)
	first, err := New(ctx, logx.NewNoopLogger(), db, dialect, Options{ApplyInterval: 50 * time.Millisecond, buildChunkSize: 2, hooks: testHooks{
		beforeBuildCommit: func(ctx context.Context, c buildChunkInfo) error {
			if c.modelID != stallModel.Load().(string) || firstChunks.Add(1) < 4 {
				return nil
			}
			select {
			case stalled <- c.gen:
			default:
			}
			<-ctx.Done()
			return ctx.Err()
		},
	}})
	require.NoError(t, err)
	closed := false
	t.Cleanup(func() {
		if !closed {
			first.Close()
		}
	})
	st, err := first.CreateStore(ctx, "failover")
	require.NoError(t, err)
	m, err := ParseDSL(testModel)
	require.NoError(t, err)
	_, err = first.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	awaitApplied(t, first, st.ID)
	writeAllCandidates(t, first, st.ID)

	var mu sync.Mutex
	secondGens := map[int64]bool{}
	second, err := New(ctx, logx.NewNoopLogger(), openSameDB(t, first), dialect, Options{ApplyInterval: 50 * time.Millisecond, buildChunkSize: 2, hooks: testHooks{
		beforeBuildCommit: func(_ context.Context, c buildChunkInfo) error {
			mu.Lock()
			defer mu.Unlock()
			secondGens[c.gen] = true
			return nil
		},
	}})
	require.NoError(t, err)
	t.Cleanup(second.Close)
	require.True(t, first.lead(ctx, st.ID))
	require.False(t, second.lead(ctx, st.ID), "the first engine is the applier")

	_, _, _, _, candidates := viewCandidates()
	stop := make(chan struct{})
	var g errgroup.Group
	for w := range 4 {
		rng := rand.New(rand.NewPCG(uint64(w), 23))
		g.Go(func() error {
			for {
				select {
				case <-stop:
					return nil
				default:
				}
				if _, err := second.Write(ctx, st.ID, randomWrite(rng, candidates)); err != nil {
					return err
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
	newModel := withoutOwnerViewers(t)
	graphModel, err := second.WriteAuthorizationModel(ctx, st.ID, newModel)
	require.NoError(t, err)
	newModelID, err := second.WriteAuthorizationModel(ctx, st.ID, newModel)
	require.NoError(t, err)
	stallModel.Store(newModelID)
	var gen int64
	select {
	case gen = <-stalled:
	case <-time.After(time.Minute):
		t.Fatal("the first engine ran no build chunk")
	}
	killLeaderSession(t, second.db.handle(), first)
	require.Eventually(t, func() bool { return second.lead(ctx, st.ID) }, 10*time.Second, 10*time.Millisecond)
	first.Close()
	closed = true

	// The build must finish while writes keep coming, but the inbox is only
	// empty once the writers stop.
	require.Eventually(t, func() bool {
		var modelID string
		_, err := second.db.handle().queryRow(ctx, "SELECT model_id FROM gebra_view_state WHERE store_id = ?", []any{st.ID}, &modelID)
		return err == nil && modelID == newModelID
	}, time.Minute, 50*time.Millisecond, "the build did not go live while writes continued")
	close(stop)
	require.NoError(t, g.Wait())
	awaitAppliedWithin(t, second, st.ID, time.Minute)
	mu.Lock()
	assert.Equal(t, map[int64]bool{gen: true}, secondGens, "the second engine resumed the build of the first one")
	mu.Unlock()
	vs := servingState(t, second.db.handle(), st.ID)
	assert.Equal(t, newModelID, vs.ModelID)
	assert.Equal(t, gen, vs.Gen)
	requireConsistent(t, second, st.ID, graphModel)
}

// TestRevisionAcrossFlip checks that a query that must see a write reads the
// generation that the write was applied to, when the view flipped to a new
// generation between the query's resolving its model and the write's being
// applied.
func TestRevisionAcrossFlip(t *testing.T) {
	ctx := context.Background()
	var e *Engine
	var storeID string
	var writeR, raceQuery atomic.Bool
	rWritten, releaseChunk, flipped := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	e, storeID = newTestEngineWith(t, testModel, Options{manualApply: true, hooks: testHooks{
		beforeBuildChunk: func(context.Context, string, int64) {
			if writeR.CompareAndSwap(true, false) {
				// R commits while the flipping chunk waits to start.
				_, err := e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})
				assert.NoError(t, err)
				close(rWritten)
				<-releaseChunk
			}
		},
		afterStoreModels: func(context.Context) {
			if raceQuery.CompareAndSwap(true, false) {
				close(releaseChunk)
				assert.NoError(t, <-flipped)
				// R applies to the new generation only.
				_, err := e.applyBatch(ctx, storeID)
				assert.NoError(t, err)
			}
		},
	}})
	_, err := e.WriteAuthorizationModel(ctx, storeID, withoutOwnerViewers(t))
	require.NoError(t, err)
	more, err := e.applyBatch(ctx, storeID)
	require.NoError(t, err)
	require.True(t, more)
	require.Equal(t, buildReach, servingState(t, e.db.handle(), storeID).BuildPhase, "the next chunk flips")

	writeR.Store(true)
	go func() {
		_, err := e.applyBatch(ctx, storeID)
		flipped <- err
	}()
	<-rWritten
	raceQuery.Store(true)
	ok, err := e.Check(ctx, storeID, &CheckRequest{
		TupleKey:    CheckTupleKey{User: "user:anne", Relation: "member", Object: "group:eng"},
		Consistency: Consistency{Preference: ConsistencyHigher},
	})
	require.NoError(t, err)
	require.False(t, raceQuery.Load(), "the query raced with the flip")
	assert.True(t, ok, "the query sees the write that committed before it")
}

// TestListAcrossFlip lists a page, which reads the view that the engine saw
// serving, then flips the view to a model where folder owners are not
// viewers, and checks that the next page answers under the new model rather
// than from the old generation, whose rows remain until they are collected.
func TestListAcrossFlip(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:bob", "owner", "folder:plans")}})
	drainView(t, e, storeID)
	list := func() []string {
		t.Helper()
		objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{Type: "folder", Relation: "viewer", User: "user:bob", PageSize: 10})
		require.NoError(t, err)
		return objects
	}
	require.Equal(t, []string{"folder:plans"}, list())

	_, err := e.WriteAuthorizationModel(ctx, storeID, withoutOwnerViewers(t))
	require.NoError(t, err)
	drainView(t, e, storeID)
	require.True(t, modelLive(t, e, storeID))
	assert.Empty(t, list())
}
