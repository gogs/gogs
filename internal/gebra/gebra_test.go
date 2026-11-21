package gebra

import (
	"context"
	"database/sql"
	"fmt"
	"hash/fnv"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/dbtest"
)

const testModel = `
model
  schema 1.1

type user

type group
  relations
    define member: [user, user:*, group#member]

type folder
  relations
    define parent: [folder]
    define owner: [user, group#member]
    define viewer: [user, group#member] or owner or viewer from parent

type doc
  attributes
    title: string
    size: int
  relations
    define parent: [folder]
    define editor: [user, group#member]
    define viewer: [user, user:*] or editor or viewer from parent
    define blocked: [user]
    define can_view: viewer but not blocked
    define can_edit: editor and viewer
`

func newTestEngine(t *testing.T, dsl string) (*Engine, string) {
	t.Helper()
	return newTestEngineWith(t, dsl, Options{})
}

func newTestEngineWith(t *testing.T, dsl string, opts Options) (*Engine, string) {
	t.Helper()
	ctx := context.Background()
	db, dialect := newTestDB(t, testSuite(t.Name()))
	e, err := New(ctx, logx.NewNoopLogger(), db, dialect, opts)
	require.NoError(t, err)
	t.Cleanup(e.Close)
	st, err := e.CreateStore(ctx, "test")
	require.NoError(t, err)
	model, err := ParseDSL(dsl)
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, st.ID, model)
	require.NoError(t, err)
	awaitApplied(t, e, st.ID)
	return e, st.ID
}

// newTestDB creates a test database for the dbtest suite and returns its pool
// and dialect.
func newTestDB(t *testing.T, suite string) (*sql.DB, Dialect) {
	t.Helper()
	gdb := dbtest.NewDB(t, suite)
	db, err := gdb.DB()
	require.NoError(t, err)
	return db, Dialect(gdb.Name())
}

// testSuite returns the dbtest suite name of a test, which keeps database
// names within MySQL's 64-character limit: dbtest adds 16 characters, and
// names longer than 48 characters keep a readable prefix and end with a hash
// of the whole name.
func testSuite(name string) string {
	name = "gebra-" + strings.ReplaceAll(name, "/", "-")
	const maxLen = 48
	if len(name) <= maxLen {
		return name
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	hash := fmt.Sprintf("%016x", h.Sum64())
	return name[:maxLen-len(hash)-1] + "-" + hash
}

func TestTestSuite(t *testing.T) {
	long := "TestApplierStatements/group_membership_add_with_many_reach_rows"
	a, b := testSuite(long+"-a"), testSuite(long+"-b")
	assert.LessOrEqual(t, len(a), 48)
	assert.LessOrEqual(t, len(b), 48)
	assert.NotEqual(t, a, b)
	assert.True(t, strings.HasPrefix(a, "gebra-TestApplierStatements-"), a)
	assert.Equal(t, "gebra-TestCheck", testSuite("TestCheck"))
}

// awaitApplied waits until the store's view includes every committed write
// and its latest model is live, applying the writes itself when the engine
// starts no appliers. It fails after two minutes.
func awaitApplied(t testing.TB, e *Engine, storeID string) {
	t.Helper()
	if e.opts.manualApply {
		for {
			more, err := e.applyBatch(context.Background(), storeID)
			require.NoError(t, err)
			if !more {
				return
			}
		}
	}
	awaitAppliedWithin(t, e, storeID, 2*time.Minute)
}

func awaitAppliedWithin(t testing.TB, e *Engine, storeID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	pending, err := e.waitUntilApplied(context.Background(), e.db.handle(), storeID, math.MaxInt64, deadline)
	require.NoError(t, err)
	require.Zero(t, pending, "the applier did not apply every write within %s", timeout)
	for {
		applied := e.appliedSignal()
		if modelLive(t, e, storeID) {
			return
		}
		require.True(t, time.Now().Before(deadline), "the latest model did not go live within %s", timeout)
		e.kick(storeID)
		select {
		case <-applied:
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// modelLive reports whether the store's latest model is live and no view
// build runs.
func modelLive(t testing.TB, e *Engine, storeID string) bool {
	t.Helper()
	st, err := e.storeRow(context.Background(), e.db.handle(), storeID)
	require.NoError(t, err)
	vs := servingState(t, e.db.handle(), storeID)
	return vs.Gen != 0 && vs.ModelID == st.ModelID && vs.BuildGen == 0
}

// mustWrite writes and returns the write's revision. It waits for the
// applier unless the test applies writes itself.
func mustWrite(t *testing.T, e *Engine, storeID string, req *WriteRequest, msgAndArgs ...any) string {
	t.Helper()
	req.Wait = !e.opts.manualApply
	revision, err := e.Write(context.Background(), storeID, req)
	require.NoError(t, err, msgAndArgs...)
	return revision
}

func tk(user, relation, object string) TupleKey {
	return TupleKey{User: user, Relation: relation, Object: object}
}

// servingState returns the store's view state row.
func servingState(t testing.TB, db dbtx, storeID string) viewStateRow {
	t.Helper()
	rows, err := queryStructs[viewStateRow](context.Background(), db, "SELECT * FROM gebra_view_state WHERE store_id = ?", storeID)
	require.NoError(t, err)
	require.Len(t, rows, 1, "view state of store %q", storeID)
	return rows[0]
}

// viewSnapshot returns the serving generation of the store's view as sorted
// strings.
func viewSnapshot(t *testing.T, db dbtx, storeID string) []string {
	t.Helper()
	return genSnapshot(t, db, storeID, servingState(t, db, storeID).Gen)
}

// genSnapshot returns a generation of the store's view as sorted strings.
func genSnapshot(t testing.TB, db dbtx, storeID string, gen int64) []string {
	t.Helper()
	ctx := context.Background()
	out := []string{}
	members, err := queryStructs[viewMemberRow](ctx, db, "SELECT * FROM gebra_view_member WHERE store_id = ? AND gen = ?", storeID, gen)
	require.NoError(t, err)
	for _, m := range members {
		out = append(out, "member "+m.SetKey+" "+m.UserType+":"+m.UserID)
	}
	edges, err := queryStructs[viewEdgeRow](ctx, db, "SELECT * FROM gebra_view_edge WHERE store_id = ? AND gen = ?", storeID, gen)
	require.NoError(t, err)
	for _, e := range edges {
		out = append(out, fmt.Sprintf("edge %s %s %d", e.ChildKey, e.ParentKey, e.Refs))
	}
	reach, err := queryStructs[viewReachRow](ctx, db, "SELECT * FROM gebra_view_reach WHERE store_id = ? AND gen = ?", storeID, gen)
	require.NoError(t, err)
	for _, r := range reach {
		out = append(out, "reach "+r.DescKey+" "+setKey(r.AncType, r.AncID, r.AncRel))
	}
	slices.Sort(out)
	return out
}

// requireViewCounts checks that the row counts in the store's view state match
// the view tables.
func requireViewCounts(t *testing.T, db dbtx, storeID string, msgAndArgs ...any) {
	t.Helper()
	state := servingState(t, db, storeID)
	var want viewCounts
	for table, n := range map[string]*int64{"gebra_view_member": &want.member, "gebra_view_edge": &want.edge, "gebra_view_reach": &want.reach} {
		_, err := db.queryRow(context.Background(), "SELECT COUNT(*) FROM "+table+" WHERE store_id = ? AND gen = ?", []any{storeID, state.Gen}, n)
		require.NoError(t, err)
	}
	require.Equal(t, want, viewCounts{member: state.MemberRows, edge: state.EdgeRows, reach: state.ReachRows}, msgAndArgs...)
}

// rebuiltSnapshot returns what a full build of the view for the store's live
// model at its applied state holds, without changing any table. It computes
// the view in memory, independently of the build in the applier.
func rebuiltSnapshot(t *testing.T, e *Engine, storeID string) []string {
	t.Helper()
	vs := servingState(t, e.db.handle(), storeID)
	require.NotZero(t, vs.Gen, "the store has a live view")
	return rebuiltSnapshotFor(t, e, storeID, vs.ModelID)
}

// rebuiltSnapshotFor returns what a full build of the view for the model at
// the store's applied state holds.
func rebuiltSnapshotFor(t testing.TB, e *Engine, storeID, modelID string) []string {
	t.Helper()
	m, err := e.model(context.Background(), e.db.handle(), storeID, modelID)
	require.NoError(t, err)
	rows, err := queryStructs[tupleRow](context.Background(), e.db.handle(), `SELECT `+tupleColumns+` FROM gebra_tuple t WHERE t.store_id = ? AND NOT EXISTS (
	SELECT 1 FROM gebra_pending p WHERE p.store_id = t.store_id AND p.object_type = t.object_type AND p.object_id = t.object_id
		AND p.relation = t.relation AND p.user_type = t.user_type AND p.user_id = t.user_id AND p.user_relation = t.user_relation)
UNION ALL SELECT `+tupleColumns+` FROM gebra_pending p WHERE p.store_id = ? AND p.deleted = TRUE AND NOT EXISTS (
	SELECT 1 FROM gebra_pending q WHERE q.store_id = p.store_id AND q.object_type = p.object_type AND q.object_id = p.object_id
		AND q.relation = p.relation AND q.user_type = p.user_type AND q.user_id = p.user_id AND q.user_relation = p.user_relation
		AND q.inbox_id < p.inbox_id)`, storeID, storeID)
	require.NoError(t, err)

	out := []string{}
	edgeCount := map[viewEdge]int64{}
	parents := map[string][]string{}
	anchored := map[string]struct{}{}
	for _, r := range rows {
		tp, err := r.tuple()
		require.NoError(t, err)
		member, edges := m.viewEffects(tp)
		if member != nil {
			out = append(out, "member "+member.SetKey+" "+member.UserType+":"+member.UserID)
			anchored[member.SetKey] = struct{}{}
		}
		for _, e := range edges {
			if edgeCount[e] == 0 {
				parents[e.child] = append(parents[e.child], e.parent)
			}
			edgeCount[e]++
			anchored[e.child] = struct{}{}
		}
	}
	for e, n := range edgeCount {
		out = append(out, fmt.Sprintf("edge %s %s %d", e.child, e.parent, n))
	}
	explicitParents := func(children []string) ([]string, error) {
		var ps []string
		for _, c := range children {
			ps = append(ps, parents[c]...)
		}
		return ps, nil
	}
	for d := range anchored {
		ups, _ := bfsUp(m, d, explicitParents)
		for a := range ups {
			out = append(out, "reach "+d+" "+a)
		}
	}
	slices.Sort(out)
	return out
}

// viewCandidates returns the objects of a small graph with nested and cyclic
// groups and folders, and the tuples that the view tests toggle in it.
func viewCandidates() (users, groups, folders, docs []string, candidates []TupleKey) {
	users = []string{"user:u0", "user:u1", "user:u2", "user:u3"}
	groups = []string{"group:g0", "group:g1", "group:g2", "group:g3"}
	folders = []string{"folder:f0", "folder:f1", "folder:f2", "folder:f3"}
	docs = []string{"doc:d0", "doc:d1", "doc:d2"}

	for _, g := range groups {
		for _, u := range users {
			candidates = append(candidates, tk(u, "member", g))
		}
		for _, g2 := range groups {
			// A set inside itself is implicit, which writes reject.
			if g2 != g {
				candidates = append(candidates, tk(g2+"#member", "member", g))
			}
		}
	}
	candidates = append(candidates, tk("user:*", "member", "group:g3"))
	for _, f := range folders {
		for _, f2 := range folders {
			if f != f2 {
				candidates = append(candidates, tk(f2, "parent", f))
			}
		}
		candidates = append(candidates, tk(users[0], "owner", f), tk(groups[1]+"#member", "owner", f), tk(groups[2]+"#member", "viewer", f), tk(users[1], "viewer", f))
	}
	for _, d := range docs {
		for _, f := range folders {
			candidates = append(candidates, tk(f, "parent", d))
		}
		candidates = append(candidates,
			tk(groups[0]+"#member", "editor", d), tk(users[2], "editor", d), tk(users[3], "viewer", d),
			tk("user:*", "viewer", d), tk(users[1], "blocked", d))
	}

	return users, groups, folders, docs, candidates
}

// TestViewMatchesEvaluation writes and deletes random tuples, including cyclic
// group nesting, and checks after every step that the incrementally maintained
// view equals a view rebuilt from scratch, and periodically that every check
// answered from the view matches graph evaluation that ignores the view.
func TestViewMatchesEvaluation(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, testModel)

	users, groups, folders, docs, candidates := viewCandidates()

	subjects := append(slices.Clone(users), "user:*")
	for _, g := range groups {
		subjects = append(subjects, g+"#member")
	}
	type target struct{ object, relation string }
	var targets []target
	for _, g := range groups {
		targets = append(targets, target{g, "member"})
	}
	for _, f := range folders {
		for _, rel := range []string{"owner", "viewer"} {
			targets = append(targets, target{f, rel})
		}
	}
	for _, d := range docs {
		for _, rel := range []string{"editor", "viewer", "can_view", "can_edit"} {
			targets = append(targets, target{d, rel})
		}
	}

	compareChecks := func(step int) {
		viewEv, err := e.newEvaluator(ctx, storeID, "", Consistency{}, nil, nil, false)
		require.NoError(t, err)
		require.True(t, viewEv.useView)
		for _, tg := range targets {
			for _, sub := range subjects {
				q, err := parseCheck(viewEv.m, CheckTupleKey{User: sub, Relation: tg.relation, Object: tg.object})
				require.NoError(t, err)

				viewEv.reset()
				fromView, err := viewEv.check(q.obj, q.rel, q.sub)
				require.NoError(t, err)

				evalEv := viewEv.fork()
				evalEv.useView = false
				fromEval, err := evalEv.check(q.obj, q.rel, q.sub)
				require.NoError(t, err)

				require.Equal(t, fromEval, fromView, "step %d: check %s %s %s", step, sub, tg.relation, tg.object)
			}
		}
	}

	rng := rand.New(rand.NewPCG(1, 2))
	present := map[TupleKey]bool{}
	for step := 0; step < 400; step++ {
		// Toggle one tuple most of the time and up to eight in one request
		// otherwise, so that one batch adds and removes edges on shared paths.
		n := 1
		if step%3 == 0 {
			n = 2 + rng.IntN(7)
		}
		req := &WriteRequest{}
		var toggled []TupleKey
		for _, i := range rng.Perm(len(candidates))[:n] {
			k := candidates[i]
			if present[k] {
				req.Deletes = append(req.Deletes, k)
			} else {
				req.Writes = append(req.Writes, k)
			}
			toggled = append(toggled, k)
		}
		mustWrite(t, e, storeID, req, "step %d", step)
		for _, k := range toggled {
			present[k] = !present[k]
		}

		require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID), "step %d after toggling %v", step, toggled)
		requireViewCounts(t, e.db.handle(), storeID, "step %d", step)
		if step%40 == 39 {
			compareChecks(step)
		}
	}
}

func TestCheck(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, testModel)
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{
		tk("user:anne", "member", "group:eng"),
		tk("group:eng#member", "member", "group:all"),
		tk("group:all#member", "viewer", "folder:root"),
		tk("folder:root", "parent", "folder:sub"),
		tk("folder:sub", "parent", "doc:plan"),
		tk("user:anne", "blocked", "doc:secret"),
		tk("folder:root", "parent", "doc:secret"),
		tk("user:bob", "editor", "doc:plan"),
	}})

	check := func(user, relation, object string) bool {
		t.Helper()
		ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: user, Relation: relation, Object: object}})
		require.NoError(t, err)
		return ok
	}
	assert.True(t, check("user:anne", "viewer", "doc:plan"), "nested groups through two folder levels")
	assert.True(t, check("user:anne", "can_view", "doc:plan"))
	assert.True(t, check("user:anne", "viewer", "doc:secret"))
	assert.False(t, check("user:anne", "can_view", "doc:secret"), "blocked")
	assert.False(t, check("user:bob", "viewer", "folder:root"))
	assert.True(t, check("user:bob", "can_edit", "doc:plan"), "editors are viewers")
	assert.False(t, check("user:anne", "can_edit", "doc:plan"))
	assert.True(t, check("group:eng#member", "viewer", "doc:plan"), "userset subject")

	mustWrite(t, e, storeID, &WriteRequest{Deletes: []TupleKey{tk("group:eng#member", "member", "group:all")}})
	assert.False(t, check("user:anne", "viewer", "doc:plan"), "a delete is visible to the next check")

	ok, err := e.Check(ctx, storeID, &CheckRequest{
		TupleKey:         CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "doc:plan"},
		ContextualTuples: []TupleKey{tk("group:eng#member", "member", "group:all")},
	})
	require.NoError(t, err)
	assert.True(t, ok, "contextual tuple restores the path")

	_, err = e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}})
	assert.ErrorIs(t, err, ErrInvalidArgument, "duplicate write")
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "member", "group:eng")}, OnDuplicate: "ignore"})
	_, err = e.Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{tk("group:eng", "viewer", "doc:plan")}})
	assert.ErrorIs(t, err, ErrInvalidArgument, "type restriction")
}

func TestConditions(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, `
model
  schema 1.1
type user
type doc
  relations
    define viewer: [user with in_window, user]
    define reader: viewer
condition in_window(now: timestamp, grant: timestamp, ttl: duration) {
  now < grant + ttl
}
`)
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{{
		User: "user:anne", Relation: "viewer", Object: "doc:1",
		Condition: &RelationshipCondition{Name: "in_window", Context: map[string]any{"grant": "2026-01-01T00:00:00Z", "ttl": "1h"}},
	}}})

	check := func(now string) (bool, error) {
		req := &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "reader", Object: "doc:1"}}
		if now != "" {
			req.Context = map[string]any{"now": now}
		}
		return e.Check(ctx, storeID, req)
	}
	ok, err := check("2026-01-01T00:30:00Z")
	require.NoError(t, err)
	assert.True(t, ok)
	ok, err = check("2026-01-01T02:00:00Z")
	require.NoError(t, err)
	assert.False(t, ok)
	_, err = check("")
	assert.ErrorIs(t, err, ErrMissingContext)

	objects, _, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{
		Type: "doc", Relation: "reader", User: "user:anne", Context: map[string]any{"now": "2026-01-01T00:30:00Z"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"doc:1"}, objects)
}

// TestListPagination lists with every sort order and a small page size, and
// compares the concatenated pages with checking every object one by one.
func TestListPagination(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngine(t, testModel)

	var writes []TupleKey
	var attrs []ObjectAttributes
	var allDocs []string
	for i := range 23 {
		d := fmt.Sprintf("doc:d%02d", i)
		allDocs = append(allDocs, d)
		switch i % 4 {
		case 0:
			writes = append(writes, tk("folder:f0", "parent", d))
		case 1:
			writes = append(writes, tk("group:eng#member", "editor", d))
		case 2:
			writes = append(writes, tk("user:anne", "viewer", d))
		}
		if i%5 == 0 {
			writes = append(writes, tk("user:anne", "blocked", d))
		}
		// Titles repeat and some are missing, so ties are broken by ID.
		a := map[string]any{"size": (i * 7 % 11) - 5}
		if i%6 != 0 {
			a["title"] = fmt.Sprintf("t%d", i%3)
		}
		attrs = append(attrs, ObjectAttributes{Object: d, Attributes: a})
	}
	writes = append(writes,
		tk("user:anne", "member", "group:eng"),
		tk("folder:f1", "parent", "folder:f0"),
		tk("user:anne", "viewer", "folder:f1"),
		tk("user:bob", "viewer", "folder:f1"),
	)
	mustWrite(t, e, storeID, &WriteRequest{Writes: writes})
	require.NoError(t, e.WriteAttributes(ctx, storeID, attrs))

	for _, rel := range []string{"viewer", "can_view", "can_edit"} {
		var want []string
		for _, d := range allDocs {
			ok, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: rel, Object: d}})
			require.NoError(t, err)
			if ok {
				want = append(want, d)
			}
		}
		require.NotEmpty(t, want)

		for _, sort := range []Sort{{}, {Descending: true}, {Attribute: "title"}, {Attribute: "title", Descending: true}, {Attribute: "size"}} {
			t.Run(fmt.Sprintf("%s/%s/desc=%v", rel, sort.Attribute, sort.Descending), func(t *testing.T) {
				var got []string
				token := ""
				for range 100 {
					page, next, err := e.ListObjects(ctx, storeID, &ListObjectsRequest{
						Type: "doc", Relation: rel, User: "user:anne", Sort: sort, PageSize: 4, ContinuationToken: token,
					})
					require.NoError(t, err)
					require.LessOrEqual(t, len(page), 4)
					got = append(got, page...)
					if next == "" {
						break
					}
					token = next
				}
				assert.ElementsMatch(t, want, got)
				assert.Equal(t, sortedByAttribute(t, e, storeID, got, sort), got)
			})
		}
	}

	users, _, err := e.ListUsers(ctx, storeID, &ListUsersRequest{
		Object: "doc:d00", Relation: "viewer", UserFilters: []UserFilter{{Type: "user"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []User{{Object: &UserObject{Type: "user", ID: "anne"}}, {Object: &UserObject{Type: "user", ID: "bob"}}}, users)

	users, _, err = e.ListUsers(ctx, storeID, &ListUsersRequest{
		Object: "doc:d00", Relation: "can_view", UserFilters: []UserFilter{{Type: "user"}},
	})
	require.NoError(t, err)
	assert.Equal(t, []User{{Object: &UserObject{Type: "user", ID: "bob"}}}, users, "anne is blocked")
}

// sortedByAttribute sorts IDs the way list queries must: by the decoded
// attribute value, missing values first, then by ID.
func sortedByAttribute(t *testing.T, e *Engine, storeID string, objects []string, s Sort) []string {
	t.Helper()
	values := map[string]string{}
	if s.Attribute != "" {
		rows, err := queryStructs[attributeRow](context.Background(), e.db.handle(), "SELECT * FROM gebra_object_attribute WHERE store_id = ? AND name = ?", storeID, s.Attribute)
		require.NoError(t, err)
		for _, r := range rows {
			values["doc:"+r.ObjectID] = r.Value
		}
	}
	out := slices.Clone(objects)
	slices.SortFunc(out, func(a, b string) int {
		c := strings.Compare(values[a], values[b])
		if c == 0 {
			c = strings.Compare(a, b)
		}
		if s.Descending {
			c = -c
		}
		return c
	})
	return out
}

func TestEncodeIntOrder(t *testing.T) {
	nums := []int64{-1 << 62, -300, -1, 0, 1, 255, 256, 1 << 40}
	for i := 1; i < len(nums); i++ {
		assert.Less(t, encodeInt(nums[i-1]), encodeInt(nums[i]))
	}
}

func TestParseDSL(t *testing.T) {
	m, err := ParseDSL(testModel)
	require.NoError(t, err)
	_, err = compileModel(m, DefaultConditionCostLimit)
	require.NoError(t, err)

	doc := m.TypeDefinitions[3]
	viewer := doc.Relations["viewer"]
	require.NotNil(t, viewer.Union)
	require.Len(t, viewer.Union.Child, 3)
	assert.NotNil(t, viewer.Union.Child[0].This)
	assert.Equal(t, "editor", viewer.Union.Child[1].ComputedUserset.Relation)
	assert.Equal(t, "parent", viewer.Union.Child[2].TupleToUserset.Tupleset.Relation)
	refs := doc.Metadata.Relations["viewer"].DirectlyRelatedUserTypes
	assert.Equal(t, "user", refs[1].Type)
	assert.NotNil(t, refs[1].Wildcard)
	assert.Equal(t, AttributeInt, doc.Metadata.Attributes["size"])

	_, err = ParseDSL("model\n  schema 1.1\ntype user\n  relations\n    define a: [user] or b and c\n")
	assert.ErrorIs(t, err, ErrInvalidArgument, "mixed operators need parentheses")

	m, err = ParseDSL("model\n  schema 1.1\ntype user\ntype doc\n  relations\n    define a: a or b\n    define b: [user]\n")
	require.NoError(t, err)
	_, err = compileModel(m, DefaultConditionCostLimit)
	assert.ErrorIs(t, err, ErrInvalidArgument, "a relation defined in terms of itself")
}

// drainView applies every pending write to the store's view.
func drainView(t *testing.T, e *Engine, storeID string) {
	t.Helper()
	for {
		more, err := e.applyBatch(context.Background(), storeID)
		require.NoError(t, err)
		if !more {
			return
		}
	}
}

// TestViewConsistency checks that queries see a write that the view does not
// include yet exactly when they ask for it.
func TestViewConsistency(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, testModel, Options{ViewWait: 20 * time.Millisecond, manualApply: true})
	drainView(t, e, storeID)
	check := func(c Consistency) bool {
		t.Helper()
		allowed, err := e.Check(ctx, storeID, &CheckRequest{
			TupleKey:    CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "folder:plans"},
			Consistency: c,
		})
		require.NoError(t, err)
		return allowed
	}

	revision := mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{
		tk("user:anne", "member", "group:eng"),
		tk("group:eng#member", "viewer", "folder:plans"),
	}})
	require.NotEmpty(t, revision)

	assert.False(t, check(Consistency{}), "the view does not include the write yet")
	assert.True(t, check(Consistency{MinRevision: revision}))
	assert.True(t, check(Consistency{Preference: ConsistencyHigher}))
	status, err := e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	assert.Equal(t, int64(1), status.PendingWrites)
	assert.Equal(t, int64(2), status.PendingChanges)
	assert.Equal(t, revision, status.LatestRevision)

	drainView(t, e, storeID)
	assert.True(t, check(Consistency{}))
	status, err = e.ViewStatus(ctx, storeID)
	require.NoError(t, err)
	assert.Equal(t, int64(0), status.PendingWrites)
	assert.Equal(t, int64(0), status.PendingChanges)
	assert.Equal(t, revision, status.AppliedRevision)

	_, err = e.Check(ctx, storeID, &CheckRequest{
		TupleKey:    CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "folder:plans"},
		Consistency: Consistency{MinRevision: "not a revision"},
	})
	assert.ErrorIs(t, err, ErrInvalidArgument)
}

// TestBatchesMatchRebuild toggles random tuples and applies them to the view
// in batches of varying size, including batches that add and remove the same
// tuple, and checks that the view then equals a rebuilt one.
func TestBatchesMatchRebuild(t *testing.T) {
	e, storeID := newTestEngineWith(t, testModel, Options{manualApply: true})
	_, _, _, _, candidates := viewCandidates()

	rng := rand.New(rand.NewPCG(5, 6))
	present := map[TupleKey]bool{}
	var last TupleKey
	for step := range 300 {
		k := candidates[rng.IntN(len(candidates))]
		if step%5 == 4 {
			k = last
		}
		req := &WriteRequest{}
		if present[k] {
			req.Deletes = []TupleKey{k}
		} else {
			req.Writes = []TupleKey{k}
		}
		mustWrite(t, e, storeID, req, "step %d", step)
		present[k] = !present[k]
		last = k

		if rng.IntN(4) == 0 {
			drainView(t, e, storeID)
			require.Equal(t, rebuiltSnapshot(t, e, storeID), viewSnapshot(t, e.db.handle(), storeID), "step %d", step)
			requireViewCounts(t, e.db.handle(), storeID, "step %d", step)
		}
	}
}

// TestViewEdgeRefs checks a view edge that two tuples put in place, as its
// reference count goes from 1 to 2 and back to 0.
func TestViewEdgeRefs(t *testing.T) {
	ctx := context.Background()
	e, storeID := newTestEngineWith(t, `
model
  schema 1.1

type user

type folder
  relations
    define parent: [folder]
    define owner: [folder]
    define reader: [user]
    define viewer: reader or viewer from parent or viewer from owner
`, Options{manualApply: true})
	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{tk("user:anne", "reader", "folder:root")}})
	drainView(t, e, storeID)

	edge := "edge " + setKey("folder", "root", "viewer") + " " + setKey("folder", "plans", "viewer") + " "
	for _, step := range []struct {
		req  *WriteRequest
		refs string
	}{
		{&WriteRequest{Writes: []TupleKey{tk("folder:root", "parent", "folder:plans")}}, "1"},
		{&WriteRequest{Writes: []TupleKey{tk("folder:root", "owner", "folder:plans")}}, "2"},
		{&WriteRequest{Deletes: []TupleKey{tk("folder:root", "parent", "folder:plans")}}, "1"},
		{&WriteRequest{Deletes: []TupleKey{tk("folder:root", "owner", "folder:plans")}}, ""},
	} {
		mustWrite(t, e, storeID, step.req)
		drainView(t, e, storeID)
		snapshot := viewSnapshot(t, e.db.handle(), storeID)
		var edges []string
		for _, s := range snapshot {
			if strings.HasPrefix(s, "edge ") {
				edges = append(edges, s)
			}
		}
		if step.refs == "" {
			assert.Empty(t, edges, "refs %s", step.refs)
		} else {
			assert.Equal(t, []string{edge + step.refs}, edges)
		}
		require.Equal(t, rebuiltSnapshot(t, e, storeID), snapshot, "refs %s", step.refs)
		requireViewCounts(t, e.db.handle(), storeID, "refs %s", step.refs)

		allowed, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "viewer", Object: "folder:plans"}})
		require.NoError(t, err)
		assert.Equal(t, step.refs != "", allowed, "refs %s", step.refs)
	}
}

// TestTelemetry checks the spans and metrics that queries and writes record.
func TestTelemetry(t *testing.T) {
	ctx := context.Background()
	spans := tracetest.NewSpanRecorder()
	reader := sdkmetric.NewManualReader()
	e, storeID := newTestEngineWith(t, testModel, Options{
		TracerProvider: sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(spans)),
		MeterProvider:  sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
	})

	mustWrite(t, e, storeID, &WriteRequest{Writes: []TupleKey{
		tk("user:anne", "member", "group:eng"),
		tk("group:eng#member", "viewer", "folder:plans"),
	}})
	for _, rel := range []string{"viewer", "owner"} {
		_, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: rel, Object: "folder:plans"}})
		require.NoError(t, err)
	}
	_, err := e.Check(ctx, storeID, &CheckRequest{TupleKey: CheckTupleKey{User: "user:anne", Relation: "nope", Object: "folder:plans"}})
	require.ErrorIs(t, err, ErrInvalidArgument)

	attrs := func(s sdktrace.ReadOnlySpan) map[string]string {
		out := map[string]string{}
		for _, kv := range s.Attributes() {
			out[string(kv.Key)] = kv.Value.Emit()
		}
		return out
	}
	var checks []map[string]string
	var write, apply map[string]string
	for _, s := range spans.Ended() {
		switch s.Name() {
		case "gebra.check":
			checks = append(checks, attrs(s))
		case "gebra.write":
			write = attrs(s)
		case "gebra.view.apply":
			if a := attrs(s); a["gebra.view.changes"] == "2" {
				apply = a
			}
		}
	}
	require.Len(t, checks, 3)
	assert.Equal(t, "true", checks[0]["gebra.allowed"])
	assert.Equal(t, "view", checks[0]["gebra.path"])
	assert.Equal(t, "folder", checks[0]["gebra.object_type"])
	assert.Equal(t, "viewer", checks[0]["gebra.relation"])
	assert.Equal(t, "false", checks[1]["gebra.allowed"])
	assert.Equal(t, "invalid_argument", checks[2]["error.type"])
	assert.NotContains(t, checks[2], "gebra.relation", "an undefined relation must not become an attribute")
	require.NotNil(t, write)
	assert.Equal(t, "2", write["gebra.write.tuples_written"])
	assert.Equal(t, "true", write["gebra.write.wait"])
	require.NotNil(t, apply, "the applier batch that applied the write")

	// Statements are client spans under the operation that ran them, with the
	// statement's text but never its arguments.
	byID := map[trace.SpanID]sdktrace.ReadOnlySpan{}
	for _, s := range spans.Ended() {
		byID[s.SpanContext().SpanID()] = s
	}
	statements := map[string][]map[string]string{}
	for _, s := range spans.Ended() {
		if s.SpanKind() != trace.SpanKindClient {
			continue
		}
		parent, ok := byID[s.Parent().SpanID()]
		require.True(t, ok, "statement %q has a parent span", s.Name())
		a := attrs(s)
		a["name"] = s.Name()
		statements[parent.Name()] = append(statements[parent.Name()], a)
		assert.NotContains(t, a["db.query.text"], "anne")
		assert.NotContains(t, a["db.query.text"], "plans")
	}
	require.NotEmpty(t, statements["gebra.check"])
	check := statements["gebra.check"][0]
	wantSystem := map[Dialect]string{DialectPostgres: "postgresql", DialectMySQL: "mysql", DialectSQLite: "sqlite"}[e.db.dialect]
	assert.Equal(t, wantSystem, check["db.system.name"])
	assert.Equal(t, "SELECT", check["db.operation.name"])
	assert.NotEmpty(t, check["db.query.text"])
	assert.Contains(t, check, "db.response.returned_rows")
	if e.db.dialect != DialectSQLite {
		assert.NotEmpty(t, check["db.namespace"])
	}
	var writeStatements []string
	for _, a := range statements["gebra.write"] {
		writeStatements = append(writeStatements, a["name"])
	}
	assert.Contains(t, writeStatements, "BEGIN")
	assert.Contains(t, writeStatements, "INSERT gebra_tuple")
	assert.Contains(t, writeStatements, "INSERT gebra_inbox")
	assert.Contains(t, writeStatements, "COMMIT")
	assert.Equal(t, "1", apply["gebra.applier.writes"])
	assert.Equal(t, "false", apply["gebra.view.flip"])
	// anne joins eng's member set, and eng's member set reaches viewer.
	assert.Equal(t, "1", apply["gebra.view.member_rows_inserted"])
	assert.Equal(t, "1", apply["gebra.view.reach_rows_inserted"])

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))
	got := map[string]metricdata.Aggregation{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			got[m.Name] = m.Data
		}
	}
	queries, ok := got["gebra.query.duration"].(metricdata.Histogram[float64])
	require.True(t, ok)
	var total uint64
	for _, dp := range queries.DataPoints {
		total += dp.Count
	}
	assert.Equal(t, uint64(3), total)
	rows, ok := got["gebra.view.rows"].(metricdata.Gauge[int64])
	require.True(t, ok)
	byTable := map[string]int64{}
	for _, dp := range rows.DataPoints {
		table, _ := dp.Attributes.Value("gebra.view.table")
		byTable[table.AsString()] = dp.Value
	}
	assert.Equal(t, map[string]int64{"member": 1, "edge": 1, "reach": 1}, byTable)
	inbox, ok := got["gebra.inbox.pending_writes"].(metricdata.Gauge[int64])
	require.True(t, ok)
	require.Len(t, inbox.DataPoints, 1)
	assert.Equal(t, int64(0), inbox.DataPoints[0].Value)
	lag, ok := got["gebra.applier.lag"].(metricdata.Histogram[float64])
	require.True(t, ok)
	require.Len(t, lag.DataPoints, 1)
	assert.Equal(t, uint64(2), lag.DataPoints[0].Count, "the model write and the tuple write")
	leadership, ok := got["gebra.applier.leadership.changes"].(metricdata.Sum[int64])
	if e.db.dialect == DialectPostgres {
		require.True(t, ok)
		assert.Equal(t, int64(1), leadership.DataPoints[0].Value, "acquired the store")
	}
}
