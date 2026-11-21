package gebra

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

func skipOnSQLite(t *testing.T) {
	t.Helper()
	if typ := os.Getenv("GOGS_DATABASE_TYPE"); typ != "postgres" && typ != "mysql" {
		t.Skip("needs concurrent write transactions, which SQLite does not run")
	}
}

// withGraphModel writes the store's latest model again and waits for the
// view to follow it. Queries that name the returned older copy evaluate the
// graph, and queries with the latest model use the view.
func withGraphModel(t *testing.T, e *Engine, storeID string) string {
	t.Helper()
	ctx := context.Background()
	st, err := e.storeRow(ctx, e.db.handle(), storeID)
	require.NoError(t, err)
	m, err := e.model(ctx, e.db.handle(), storeID, st.ModelID)
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, storeID, m.source)
	require.NoError(t, err)
	awaitApplied(t, e, storeID)
	return st.ModelID
}

// compareWithGraph checks that every check, ListObjects, and ListUsers of the
// test model's objects answers the same from the view as from graph
// evaluation with graphModelID.
func compareWithGraph(t *testing.T, e *Engine, storeID, graphModelID string) {
	t.Helper()
	ctx := context.Background()
	status, err := e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	latest, err := e.storeRow(ctx, e.db.handle(), storeID)
	require.NoError(t, err)
	require.Equal(t, latest.ModelID, status.AuthorizationModelID, "the view follows the latest model")

	users, groups, folders, docs, _ := viewCandidates()
	subjects := append(slices.Clone(users), "user:*")
	for _, g := range groups {
		subjects = append(subjects, g+"#member")
	}
	type target struct {
		typ, relation string
		objects       []string
	}
	targets := []target{
		{"group", "member", groups},
		{"folder", "owner", folders}, {"folder", "viewer", folders},
		{"doc", "editor", docs}, {"doc", "viewer", docs}, {"doc", "can_view", docs}, {"doc", "can_edit", docs},
	}
	for _, tg := range targets {
		for _, obj := range tg.objects {
			for _, sub := range subjects {
				key := CheckTupleKey{User: sub, Relation: tg.relation, Object: obj}
				fromView, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: key})
				require.NoError(t, err)
				fromGraph, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: key, AuthorizationModelID: graphModelID})
				require.NoError(t, err)
				require.Equal(t, fromGraph, fromView, "check %s %s %s", sub, tg.relation, obj)
			}
			for _, filter := range []UserFilter{{Type: "user"}, {Type: "group", Relation: "member"}} {
				req := &ListUsersRequest{Object: obj, Relation: tg.relation, UserFilters: []UserFilter{filter}}
				fromView, _, err := e.ListUsers(ctx, storeID, req)
				require.NoError(t, err)
				req.AuthorizationModelID = graphModelID
				fromGraph, _, err := e.ListUsers(ctx, storeID, req)
				require.NoError(t, err)
				require.Equal(t, fromGraph, fromView, "list users %s %s %v", obj, tg.relation, filter)
			}
		}
		for _, sub := range subjects {
			if sub == "user:*" {
				continue
			}
			req := &ListObjectsRequest{Type: tg.typ, Relation: tg.relation, User: sub}
			fromView, _, err := e.ListObjects(ctx, storeID, req)
			require.NoError(t, err)
			req.AuthorizationModelID = graphModelID
			fromGraph, _, err := e.ListObjects(ctx, storeID, req)
			require.NoError(t, err)
			require.Equal(t, fromGraph, fromView, "list objects %s %s %s", tg.typ, tg.relation, sub)
		}
	}
}

// requireChangeLogMatchesTuples replays the store's change log and checks
// that every write adds a missing tuple and every delete removes a present
// one, which a lost or repeated change breaks, and that the result equals
// the stored tuples.
func requireChangeLogMatchesTuples(t *testing.T, e *Engine, storeID string) {
	t.Helper()
	ctx := context.Background()
	key := func(k TupleKey) string { return k.User + " " + k.Relation + " " + k.Object }
	replayed := map[string]bool{}
	token := ""
	for {
		changes, next, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{PageSize: 100, ContinuationToken: token})
		require.NoError(t, err)
		if len(changes) == 0 {
			break
		}
		for i, c := range changes {
			k := key(c.TupleKey)
			switch c.Operation {
			case OperationWrite:
				require.False(t, replayed[k], "change %d writes %s, which exists", i, k)
				replayed[k] = true
			case OperationDelete:
				require.True(t, replayed[k], "change %d deletes %s, which does not exist", i, k)
				delete(replayed, k)
			}
		}
		token = next
	}
	stored := map[string]bool{}
	token = ""
	for {
		tuples, next, err := e.Read(ctx, storeID, &ReadRequest{PageSize: 500, ContinuationToken: token})
		require.NoError(t, err)
		for _, tu := range tuples {
			stored[key(tu.Key)] = true
		}
		if next == "" {
			break
		}
		token = next
	}
	require.Equal(t, stored, replayed)
}

// requireNothingPending checks that the store's inbox is empty.
func requireNothingPending(t *testing.T, e *Engine, storeID string) {
	t.Helper()
	var n int64
	_, err := e.db.handle().queryRow(context.Background(), "SELECT COUNT(*) FROM gebra_inbox WHERE store_id = ?", []any{storeID}, &n)
	require.NoError(t, err)
	require.Zero(t, n)
}

// requireConsistent checks the view, its row counts, the change log, and
// every query against graph evaluation after the applier drained the store.
func requireConsistent(t *testing.T, e *Engine, storeID, graphModelID string) {
	t.Helper()
	awaitApplied(t, e, storeID)
	requireNothingPending(t, e, storeID)
	require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID))
	requireViewCounts(t, e.db.handle(), storeID)
	requireChangeLogMatchesTuples(t, e, storeID)
	compareWithGraph(t, e, storeID, graphModelID)
}

// randomWrite returns a write of 1 to 4 tuples. Half of them come from a few
// hot tuples, so that concurrent writers write and delete the same tuples.
func randomWrite(rng *rand.Rand, candidates []TupleKey) *WriteRequest {
	req := &WriteRequest{OnDuplicate: "ignore", OnMissing: "ignore"}
	seen := map[TupleKey]bool{}
	for range 1 + rng.IntN(4) {
		pool := candidates
		if rng.IntN(2) == 0 {
			pool = candidates[:6]
		}
		k := pool[rng.IntN(len(pool))]
		if seen[k] {
			continue
		}
		seen[k] = true
		if rng.IntN(2) == 0 {
			req.Writes = append(req.Writes, k)
		} else {
			req.Deletes = append(req.Deletes, k)
		}
	}
	return req
}

// TestConcurrentWrites runs writers in parallel on overlapping tuples while
// the applier updates the view, and checks that once it drained the store the
// view equals a rebuilt one and every query answers as graph evaluation does.
// Writers only race on databases with row locks, since SQLite runs one writer
// at a time.
func TestConcurrentWrites(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, testModel)
	graphModel := withGraphModel(t, e, storeID)
	_, _, _, _, candidates := viewCandidates()

	var g errgroup.Group
	for w := range 8 {
		rng := rand.New(rand.NewPCG(uint64(w), 3))
		g.Go(func() error {
			for range 60 {
				req := randomWrite(rng, candidates)
				req.Wait = rng.IntN(4) == 0
				if _, err := e.Write(ctx, storeID, req); err != nil {
					return err
				}
			}
			return nil
		})
	}
	require.NoError(t, g.Wait())
	requireConsistent(t, e, storeID, graphModel)
}

// TestSameTupleWritesOrderInbox checks that of two writes of the same tuple,
// the one that commits later has the higher inbox ID, which the applier
// relies on to apply the writes of a tuple in the order they happened.
func TestSameTupleWritesOrderInbox(t *testing.T) {
	skipOnSQLite(t)
	ctx := context.Background()

	// pause holds a write at a hook until release is closed, once paused
	// reports that it got there.
	type pause struct {
		req     *WriteRequest
		paused  chan struct{}
		release chan struct{}
	}
	var mu sync.Mutex
	pauses := map[string]*pause{}
	hold := func(at string, req *WriteRequest) {
		mu.Lock()
		p := pauses[at]
		if p == nil || p.req != req {
			mu.Unlock()
			return
		}
		delete(pauses, at)
		mu.Unlock()
		close(p.paused)
		<-p.release
	}
	pauseAt := func(at string, req *WriteRequest) *pause {
		p := &pause{req: req, paused: make(chan struct{}), release: make(chan struct{})}
		mu.Lock()
		pauses[at] = p
		mu.Unlock()
		return p
	}
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true, hooks: testHooks{
		beforeTuples:      func(_ context.Context, req *WriteRequest) { hold("tuples", req) },
		beforeWriteCommit: func(_ context.Context, req *WriteRequest) { hold("commit", req) },
	}})
	revisionID := func(rev string) int64 {
		t.Helper()
		id, err := decodeIntToken(rev)
		require.NoError(t, err)
		return id
	}
	// write runs the request in the background and returns its revision on
	// the channel.
	write := func(req *WriteRequest) <-chan string {
		done := make(chan string, 1)
		go func() {
			rev, err := e.Write(ctx, storeID, req)
			assert.NoError(t, err)
			done <- rev
		}()
		return done
	}
	finish := func(name string) {
		t.Helper()
		drainView(t, e, storeID)
		require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID), name)
		requireChangeLogMatchesTuples(t, e, storeID)
	}

	t.Run("a write that reads a committed write orders after it", func(t *testing.T) {
		k := tk("user:anne", "member", "group:g1")
		del := &WriteRequest{Deletes: []TupleKey{k}}
		p := pauseAt("tuples", del)
		delDone := write(del)
		<-p.paused
		// The delete started first but reads the tuple only after the write
		// committed.
		wrote := mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{k}})
		close(p.release)
		deleted := <-delDone
		require.NotEmpty(t, deleted)
		assert.Greater(t, revisionID(deleted), revisionID(wrote))
		finish(t.Name())
	})

	t.Run("a delete waits on the row that another delete holds", func(t *testing.T) {
		k, other := tk("user:bob", "member", "group:g1"), tk("user:bob", "member", "group:g2")
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{k}})
		first := &WriteRequest{Deletes: []TupleKey{k}}
		p := pauseAt("commit", first)
		firstDone := write(first)
		<-p.paused
		start := time.Now()
		secondDone := write(&WriteRequest{Deletes: []TupleKey{k}, Writes: []TupleKey{other}, OnMissing: "ignore"})
		time.Sleep(200 * time.Millisecond)
		close(p.release)
		firstRev, secondRev := <-firstDone, <-secondDone
		assert.GreaterOrEqual(t, time.Since(start), 200*time.Millisecond)
		assert.Greater(t, revisionID(secondRev), revisionID(firstRev))
		finish(t.Name())
	})

	t.Run("a write waits on the index entry that another write holds", func(t *testing.T) {
		k, other := tk("user:carl", "member", "group:g1"), tk("user:carl", "member", "group:g2")
		first := &WriteRequest{Writes: []TupleKey{k}}
		p := pauseAt("commit", first)
		firstDone := write(first)
		<-p.paused
		secondDone := write(&WriteRequest{Writes: []TupleKey{k, other}, OnDuplicate: "ignore"})
		time.Sleep(200 * time.Millisecond)
		close(p.release)
		firstRev, secondRev := <-firstDone, <-secondDone
		assert.Greater(t, revisionID(secondRev), revisionID(firstRev))
		finish(t.Name())
	})
}

// TestRevisionsOutOfOrder commits a write after one with a higher inbox ID
// was applied, and checks that the view handles the order and that
// MinRevision and HIGHER_CONSISTENCY never answer from a view that lacks it.
func TestRevisionsOutOfOrder(t *testing.T) {
	skipOnSQLite(t)
	ctx := context.Background()
	release := make(chan struct{})
	paused := make(chan struct{})
	var held atomic.Pointer[WriteRequest]
	e, storeID := newTestEngineWith(t, testModel, Options{ViewWait: 20 * time.Millisecond, manualApply: true, hooks: testHooks{
		beforeWriteCommit: func(_ context.Context, req *WriteRequest) {
			if held.CompareAndSwap(req, nil) {
				close(paused)
				<-release
			}
		},
	}})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("group:eng#member", "viewer", "folder:plans")}})
	drainView(t, e, storeID)

	slow := &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}}
	held.Store(slow)
	slowDone := make(chan string, 1)
	go func() {
		rev, err := e.Write(ctx, storeID, slow)
		assert.NoError(t, err)
		slowDone <- rev
	}()
	<-paused
	fast := mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:bob", "member", "group:eng")}})
	// The applier only sees the committed write with the higher ID.
	drainView(t, e, storeID)
	close(release)
	slowRev := <-slowDone
	slowID, err := decodeIntToken(slowRev)
	require.NoError(t, err)
	fastID, err := decodeIntToken(fast)
	require.NoError(t, err)
	require.Less(t, slowID, fastID, "the slow write took its inbox ID first")
	status, err := e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	require.Equal(t, fast, status.AppliedRevision)
	require.Equal(t, int64(1), status.PendingWrites)

	check := func(user string, c Consistency) bool {
		t.Helper()
		ok, err := e.Check(ctx, storeID, &CheckRequest{
			TupleKey:    CheckTupleKey{User: user, Relation: "viewer", Object: "folder:plans"},
			Consistency: c,
		})
		require.NoError(t, err)
		return ok
	}
	assert.True(t, check("user:bob", Consistency{}))
	assert.False(t, check("user:anne", Consistency{}), "the view lacks the slow write")
	assert.True(t, check("user:anne", Consistency{MinRevision: slowRev}))
	assert.True(t, check("user:anne", Consistency{MinRevision: fast}), "the slow write committed before the query, and its ID is lower")
	assert.True(t, check("user:anne", Consistency{Preference: ConsistencyHigher}))
	objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{
		Type: "folder", Relation: "viewer", User: "user:anne", Consistency: Consistency{MinRevision: slowRev},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"folder:plans"}, objects)
	users, _, err := e.ListUsers(ctx, storeID, &ListUsersRequest{
		Object: "folder:plans", Relation: "viewer", UserFilters: []UserFilter{{Type: "user"}},
		Consistency: Consistency{Preference: ConsistencyHigher},
	})
	require.NoError(t, err)
	assert.Len(t, users, 2)

	drainView(t, e, storeID)
	assert.True(t, check("user:anne", Consistency{}))
	require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID))
	requireViewCounts(t, e.db.handle(), storeID)
	changes, _, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{})
	require.NoError(t, err)
	require.Len(t, changes, 3)
	assert.Equal(t, "user:bob", changes[1].TupleKey.User, "the change log is in apply order")
	assert.Equal(t, "user:anne", changes[2].TupleKey.User)
}

// TestRevisionsNeverStale has writers toggle grants of their own while the
// applier runs, and checks after every write that queries with its revision
// or HIGHER_CONSISTENCY see it, whether the view includes it or they evaluate
// the graph.
func TestRevisionsNeverStale(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, testModel, Options{ViewWait: 5 * time.Millisecond})
	const writers = 6
	for w := range writers {
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk(fmt.Sprintf("group:w%d#member", w), "viewer", fmt.Sprintf("folder:w%d", w))}})
	}
	var g errgroup.Group
	for w := range writers {
		g.Go(func() error {
			user, group, folder := fmt.Sprintf("user:w%d", w), fmt.Sprintf("group:w%d", w), fmt.Sprintf("folder:w%d", w)
			present := false
			for i := range 40 {
				req := &WriteRequest{}
				if present {
					req.Deletes = []TupleKey{tk(user, "member", group)}
				} else {
					req.Writes = []TupleKey{tk(user, "member", group)}
				}
				rev, err := e.Write(ctx, storeID, req)
				if err != nil {
					return err
				}
				present = !present
				c := Consistency{MinRevision: rev}
				if i%2 == 1 {
					c = Consistency{Preference: ConsistencyHigher}
				}
				ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: user, Relation: "viewer", Object: folder}, Consistency: c})
				if err != nil {
					return err
				}
				if ok != present {
					return fmt.Errorf("writer %d step %d with %+v: check says %v after the write made it %v", w, i, c, ok, present)
				}
			}
			return nil
		})
	}
	require.NoError(t, g.Wait())
}

// TestWriteWait checks that a write with Wait returns only once the applier
// applied it, and fails with ErrNotApplied when no applier makes progress.
func TestWriteWait(t *testing.T) {
	ctx := context.Background()
	const applyDelay = 150 * time.Millisecond
	e, storeID := newTestEngineWith(t, testModel, Options{hooks: testHooks{
		beforeApplyCommit: func(context.Context, string) error {
			time.Sleep(applyDelay)
			return nil
		},
	}})
	check := func(user string) bool {
		t.Helper()
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: user, Relation: "member", Object: "group:eng"}})
		require.NoError(t, err)
		return ok
	}

	start := time.Now()
	rev, err := e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}, Wait: true})
	require.NoError(t, err)
	assert.GreaterOrEqual(t, time.Since(start), applyDelay)
	id, err := decodeIntToken(rev)
	require.NoError(t, err)
	pending, err := pendingThrough(ctx, e.db.handle(), storeID, id)
	require.NoError(t, err)
	assert.Zero(t, pending)
	assert.True(t, check("user:anne"), "the view includes the write")

	rev, err = e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("user:bob", "member", "group:eng")}})
	require.NoError(t, err)
	id, err = decodeIntToken(rev)
	require.NoError(t, err)
	pending, err = pendingThrough(ctx, e.db.handle(), storeID, id)
	require.NoError(t, err)
	assert.Equal(t, id, pending, "without Wait the write returns before the applier commits")
	awaitApplied(t, e, storeID)

	t.Run("no applier", func(t *testing.T) {
		idle, idleStore := newTestEngineWith(t, testModel, Options{manualApply: true, WaitTimeout: 100 * time.Millisecond})
		start := time.Now()
		rev, err := idle.Write(ctx, idleStore, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}, Wait: true})
		require.ErrorIs(t, err, ErrNotApplied)
		assert.Contains(t, err.Error(), "no applier made progress")
		assert.NotEmpty(t, rev, "the write committed")
		assert.Less(t, time.Since(start), 5*time.Second)
		tuples, _, err := idle.Read(ctx, idleStore, &ReadRequest{})
		require.NoError(t, err)
		assert.Len(t, tuples, 1, "Read sees committed writes that are not applied")
	})
}

// TestReadChangesWhileWriting pages through the change log while writers
// run, and checks that the pages hold exactly the changes that one read of
// the whole log returns afterwards, in the same order.
func TestReadChangesWhileWriting(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, testModel)
	_, _, _, _, candidates := viewCandidates()

	var writing atomic.Bool
	writing.Store(true)
	var g errgroup.Group
	for w := range 4 {
		rng := rand.New(rand.NewPCG(uint64(w), 9))
		g.Go(func() error {
			for range 50 {
				if _, err := e.Write(ctx, storeID, randomWrite(rng, candidates)); err != nil {
					return err
				}
			}
			return nil
		})
	}
	done := make(chan error, 1)
	go func() {
		done <- g.Wait()
		writing.Store(false)
	}()

	var paged []TupleChange
	token := ""
	for drained := false; ; {
		changes, next, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{PageSize: 7, ContinuationToken: token})
		require.NoError(t, err)
		paged = append(paged, changes...)
		token = next
		if len(changes) > 0 {
			continue
		}
		if drained {
			break
		}
		time.Sleep(time.Millisecond)
		if !writing.Load() {
			awaitApplied(t, e, storeID)
			drained = true
		}
	}
	require.NoError(t, <-done)
	all, _, err := e.ReadChanges(ctx, storeID, &ReadChangesRequest{PageSize: 100_000})
	require.NoError(t, err)
	require.Equal(t, all, paged)
	requireChangeLogMatchesTuples(t, e, storeID)
}

// openSameDB opens another connection pool to the engine's database, as a
// second process would.
func openSameDB(t *testing.T, e *Engine) *sql.DB {
	t.Helper()
	driver, nameQuery := "pgx", "SELECT current_database()"
	opts := conf.DatabaseOpts{
		Type: "postgres", Host: os.ExpandEnv("$PGHOST:$PGPORT"), Schema: "public",
		User: os.Getenv("PGUSER"), Password: os.Getenv("PGPASSWORD"), SSLMode: os.Getenv("PGSSLMODE"),
	}
	if e.db.dialect == DialectMySQL {
		driver, nameQuery = "mysql", "SELECT DATABASE()"
		opts = conf.DatabaseOpts{
			Type: "mysql", Host: os.ExpandEnv("$MYSQL_HOST:$MYSQL_PORT"),
			User: os.Getenv("MYSQL_USER"), Password: os.Getenv("MYSQL_PASSWORD"),
		}
	}
	_, err := e.db.handle().queryRow(context.Background(), nameQuery, nil, &opts.Name)
	require.NoError(t, err)
	dsn, err := dbx.NewDSN(opts)
	require.NoError(t, err)
	other, err := sql.Open(driver, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = other.Close() })
	return other
}

// killLeaderSession ends the session of the engine's leader connection from
// db, as a database restart or a network failure would, which releases the
// engine's applier locks.
func killLeaderSession(t *testing.T, db dbtx, e *Engine) {
	t.Helper()
	ctx := context.Background()
	e.leader.mu.Lock()
	pid := e.leader.pid
	e.leader.mu.Unlock()
	require.NotZero(t, pid, "the engine has a leader connection")
	if db.dialect == DialectMySQL {
		_, err := db.exec(ctx, fmt.Sprintf("KILL %d", pid))
		require.NoError(t, err)
		return
	}
	_, err := db.exec(ctx, "SELECT pg_terminate_backend(?)", pid)
	require.NoError(t, err)
}

// TestApplierFailover runs two engines on one database, as two processes.
// The applier dies in the middle of a batch while writes continue, and the
// other engine takes over without losing or repeating a change. On MySQL the
// applier's session is killed first.
func TestApplierFailover(t *testing.T) {
	skipOnSQLite(t)
	ctx := context.Background()
	db, dialect := newTestDB(t, testSuite(t.Name()))
	var stall atomic.Bool
	stalled := make(chan struct{})
	var stalledOnce sync.Once
	first, err := New(ctx, logx.NewNoopLogger(), db, dialect, Options{ApplyInterval: 50 * time.Millisecond, hooks: testHooks{
		beforeApplyCommit: func(ctx context.Context, _ string) error {
			if !stall.Load() {
				return nil
			}
			// The batch has changed the view and the change log, and dies
			// before it commits.
			stalledOnce.Do(func() { close(stalled) })
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
	model, err := ParseDSL(testModel)
	require.NoError(t, err)
	_, err = first.WriteAuthorizationModel(ctx, st.ID, model)
	require.NoError(t, err)
	awaitApplied(t, first, st.ID)
	graphModel := withGraphModel(t, first, st.ID)

	second, err := New(ctx, logx.NewNoopLogger(), openSameDB(t, first), dialect, Options{ApplyInterval: 50 * time.Millisecond})
	require.NoError(t, err)
	t.Cleanup(second.Close)
	require.True(t, first.lead(ctx, st.ID))
	require.False(t, second.lead(ctx, st.ID), "the first engine is the applier")

	_, _, _, _, candidates := viewCandidates()
	var g errgroup.Group
	for w := range 4 {
		rng := rand.New(rand.NewPCG(uint64(w), 11))
		g.Go(func() error {
			for i := range 60 {
				// Writes go through both engines until the first one dies.
				e := second
				if i%2 == 0 && !stall.Load() {
					e = first
				}
				if _, err := e.Write(ctx, st.ID, randomWrite(rng, candidates)); err != nil {
					return err
				}
				time.Sleep(time.Millisecond)
			}
			return nil
		})
	}
	time.Sleep(50 * time.Millisecond)
	stall.Store(true)
	select {
	case <-stalled:
	case <-time.After(10 * time.Second):
		t.Fatal("the first engine applied no batch")
	}
	applied := second.appliedSignal()
	killed := time.Now()
	if dialect == DialectMySQL {
		// Ending the session releases the GET_LOCK lock while the first
		// engine still runs, so the second one takes over before the first
		// one closes.
		killLeaderSession(t, first.db.handle(), first)
		require.Eventually(t, func() bool { return second.lead(ctx, st.ID) }, 10*time.Second, 10*time.Millisecond)
	}
	first.Close()
	closed = true
	select {
	case <-applied:
		t.Logf("the second engine committed its first batch %s after the first one died", time.Since(killed))
	case <-time.After(5 * time.Second):
		t.Fatal("the second engine did not take over within 5s")
	}
	require.NoError(t, g.Wait())
	requireConsistent(t, second, st.ID, graphModel)
}

// TestModelWritesInterleaved writes models while tuple writes run, and checks
// that the applier applies each tuple write under the model that was latest
// when it reached it, and leaves a view that equals a rebuilt one.
func TestModelWritesInterleaved(t *testing.T) {
	ctx := context.Background()
	extended, err := ParseDSL(testModel + "    define reviewer: [user] or editor\n")
	require.NoError(t, err)

	t.Run("ordered", func(t *testing.T) {
		e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true})
		before, err := e.storeRow(ctx, e.db.handle(), storeID)
		require.NoError(t, err)
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "editor", "doc:1")}})
		modelID, err := e.WriteAuthorizationModel(ctx, storeID, extended)
		require.NoError(t, err)
		mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:bob", "reviewer", "doc:1")}})

		// Until the new model is live, default queries use the old one, and
		// queries that name the new one evaluate the graph.
		bob := CheckTupleKey{User: "user:bob", Relation: "reviewer", Object: "doc:1"}
		_, err = e.Check(ctx, storeID, &CheckRequest{TupleKey: bob})
		require.ErrorIs(t, err, ErrInvalidArgument, "reviewer is not defined in the live model")
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: bob, AuthorizationModelID: modelID})
		require.NoError(t, err)
		assert.True(t, ok)

		// The first batch applies the writes to the live view, starts the
		// build, and runs its first chunk.
		more, err := e.applyBatch(ctx, storeID)
		require.NoError(t, err)
		assert.True(t, more, "the build goes on")
		requireNothingPending(t, e, storeID)
		status, err := e.ViewStatus(ctx, storeID)
		require.NoError(t, err)
		assert.Equal(t, before.ModelID, status.AuthorizationModelID)
		require.NotNil(t, status.Migration)
		assert.Equal(t, before.ModelID, status.Migration.LiveModelID)
		assert.Equal(t, modelID, status.Migration.TargetModelID)
		assert.Equal(t, "building_reach", status.Migration.Phase)
		require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID), "the live view includes the writes")
		ok, err = e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "doc:1"}})
		require.NoError(t, err)
		assert.True(t, ok)

		drainView(t, e, storeID)
		status, err = e.ViewStatus(ctx, storeID)
		require.NoError(t, err)
		assert.Equal(t, modelID, status.AuthorizationModelID)
		assert.Nil(t, status.Migration)
		ok, err = e.Check(ctx, storeID, &CheckRequest{TupleKey: bob})
		require.NoError(t, err)
		assert.True(t, ok)
		requireNothingPending(t, e, storeID)
		require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID))
		requireViewCounts(t, e.db.handle(), storeID)
		requireChangeLogMatchesTuples(t, e, storeID)
	})

	t.Run("concurrent", func(t *testing.T) {
		e, storeID := newTestEngine(t, testModel)
		graphModel := withGraphModel(t, e, storeID)
		base, err := ParseDSL(testModel)
		require.NoError(t, err)
		_, _, _, _, candidates := viewCandidates()
		var g errgroup.Group
		for w := range 4 {
			rng := rand.New(rand.NewPCG(uint64(w), 13))
			g.Go(func() error {
				for range 50 {
					if _, err := e.Write(ctx, storeID, randomWrite(rng, candidates)); err != nil {
						return err
					}
				}
				return nil
			})
		}
		g.Go(func() error {
			for i := range 6 {
				m := base
				if i%2 == 0 {
					m = extended
				}
				if _, err := e.WriteAuthorizationModel(ctx, storeID, m); err != nil {
					return err
				}
				time.Sleep(10 * time.Millisecond)
			}
			return nil
		})
		require.NoError(t, g.Wait())
		requireConsistent(t, e, storeID, graphModel)
	})
}
