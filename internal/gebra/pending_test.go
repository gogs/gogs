package gebra

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"unknwon.dev/x/logx"
)

// TestTokenQueriesUseView holds writes pending and checks that a query whose
// revision the view lacks gets the answer of the latest state, from the view
// when no pending change can affect it and from the graph otherwise.
func TestTokenQueriesUseView(t *testing.T) {
	ctx := context.Background()
	const (
		behind     = "view_behind"
		irrelevant = "view_irrelevant_pending"
		partial    = "view_behind_partial"
	)
	check := func(user, rel, obj string) *diffQuery {
		return &diffQuery{Kind: kindCheck, User: user, Relation: rel, Object: obj}
	}
	listObjects := func(typ, rel, user string) *diffQuery {
		return &diffQuery{Kind: kindListObjects, Type: typ, Relation: rel, User: user}
	}
	listUsers := func(obj, rel string) *diffQuery {
		return &diffQuery{Kind: kindListUsers, Object: obj, Relation: rel, Filter: UserFilter{Type: "user"}}
	}
	many := make([]TupleKey, maxPendingChanges)
	for i := range many {
		many[i] = tk(fmt.Sprintf("user:u%04d", i), "member", "group:many")
	}
	for _, tc := range []struct {
		name string
		// model is testModel when empty.
		model   string
		applied []TupleKey
		// pending holds the writes and deletes of pending writes, one write
		// per element. A key whose object is prefixed with "-" is deleted.
		pending [][]TupleKey
		query   *diffQuery
		higher  bool
		want    string
		reason  string
	}{
		{
			name:    "member added to a set that reaches the target",
			applied: []TupleKey{tk("group:eng#member", "viewer", "folder:x")},
			pending: [][]TupleKey{{tk("user:anne", "member", "group:eng")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "member added for another user",
			applied: []TupleKey{tk("group:eng#member", "viewer", "folder:x"), tk("user:anne", "member", "group:eng")},
			pending: [][]TupleKey{{tk("user:bob", "member", "group:eng")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: irrelevant,
		},
		{
			name:    "member added to a set that does not reach the target",
			applied: []TupleKey{tk("group:eng#member", "viewer", "folder:x")},
			pending: [][]TupleKey{{tk("user:anne", "member", "group:ops")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "false", reason: irrelevant,
		},
		{
			name:    "member removed",
			applied: []TupleKey{tk("group:eng#member", "viewer", "folder:x"), tk("user:anne", "member", "group:eng")},
			pending: [][]TupleKey{{tk("user:anne", "member", "-group:eng")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "false", reason: behind,
		},
		{
			name:    "wildcard added",
			pending: [][]TupleKey{{tk("user:*", "viewer", "doc:1")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "true", reason: behind,
		},
		{
			name:    "edge creates a path",
			applied: []TupleKey{tk("user:anne", "member", "group:eng")},
			pending: [][]TupleKey{{tk("group:eng#member", "viewer", "folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "edge from a set that the user is not in",
			applied: []TupleKey{tk("user:bob", "member", "group:eng")},
			pending: [][]TupleKey{{tk("group:eng#member", "viewer", "folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "false", reason: irrelevant,
		},
		{
			name:    "edge from a set that the user is in through another",
			applied: []TupleKey{tk("user:anne", "member", "group:a"), tk("group:a#member", "member", "group:b")},
			pending: [][]TupleKey{{tk("group:b#member", "viewer", "folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "two edges form a path",
			applied: []TupleKey{tk("user:anne", "member", "group:a")},
			pending: [][]TupleKey{{tk("group:a#member", "member", "group:b")}, {tk("group:b#member", "viewer", "folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "a member and an edge form a path",
			pending: [][]TupleKey{{tk("user:anne", "member", "group:a")}, {tk("group:a#member", "viewer", "folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "edge removed",
			applied: []TupleKey{tk("user:anne", "member", "group:a"), tk("group:a#member", "viewer", "folder:x")},
			pending: [][]TupleKey{{tk("group:a#member", "viewer", "-folder:x")}},
			query:   check("user:anne", "viewer", "folder:x"), want: "false", reason: behind,
		},
		{
			name:    "tupleset tuple added",
			applied: []TupleKey{tk("user:anne", "viewer", "folder:f")},
			pending: [][]TupleKey{{tk("folder:f", "parent", "doc:1")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "true", reason: behind,
		},
		{
			name:    "tupleset tuple of another object",
			applied: []TupleKey{tk("user:anne", "viewer", "folder:f")},
			pending: [][]TupleKey{{tk("folder:f", "parent", "doc:2")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "false", reason: irrelevant,
		},
		{
			name:    "excluded set changed",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:anne", "blocked", "doc:1")}},
			query:   check("user:anne", "can_view", "doc:1"), want: "false", reason: behind,
		},
		{
			name:    "excluded set of another object",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:anne", "blocked", "doc:2")}},
			query:   check("user:anne", "can_view", "doc:1"), want: "true", reason: irrelevant,
		},
		{
			// Evaluation reads this relation's tuples, which a pending change
			// of another user touches.
			name:    "direct tuples of a relation that the view does not hold",
			model:   blockedModel,
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:bob", "viewer", "doc:1")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "true", reason: behind,
		},
		{
			name:    "intersection operand changed",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:anne", "editor", "doc:1")}},
			query:   check("user:anne", "can_edit", "doc:1"), want: "true", reason: behind,
		},
		{
			name:    "userset subject gains an edge",
			pending: [][]TupleKey{{tk("group:eng#member", "viewer", "folder:x")}},
			query:   check("group:eng#member", "viewer", "folder:x"), want: "true", reason: behind,
		},
		{
			name:    "userset subject and a member change",
			applied: []TupleKey{tk("group:eng#member", "viewer", "folder:x")},
			pending: [][]TupleKey{{tk("user:anne", "member", "group:eng")}},
			query:   check("group:eng#member", "viewer", "folder:x"), want: "true", reason: irrelevant,
		},
		{
			name:    "higher consistency",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:bob", "viewer", "doc:1")}},
			query:   check("user:anne", "viewer", "doc:1"), higher: true, want: "true", reason: irrelevant,
		},
		{
			name:    "list objects of another user",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:bob", "viewer", "doc:2")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1", reason: irrelevant,
		},
		{
			name:    "list objects gains an object",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:anne", "viewer", "doc:2")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1,doc:2", reason: partial,
		},
		{
			name:    "list objects loses an object",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1"), tk("user:anne", "viewer", "doc:2")},
			pending: [][]TupleKey{{tk("user:anne", "viewer", "-doc:2")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1", reason: partial,
		},
		{
			name: "list objects keeps an object granted another way",
			applied: []TupleKey{
				tk("user:anne", "viewer", "doc:1"), tk("user:anne", "member", "group:g"), tk("group:g#member", "editor", "doc:1"),
			},
			pending: [][]TupleKey{{tk("user:anne", "viewer", "-doc:1")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1", reason: partial,
		},
		{
			name: "list objects gains the objects below a pending member",
			applied: []TupleKey{
				tk("user:anne", "viewer", "doc:1"), tk("group:g#member", "viewer", "folder:f"),
				tk("folder:f", "parent", "doc:3"), tk("folder:f", "parent", "doc:4"),
			},
			pending: [][]TupleKey{{tk("user:anne", "member", "group:g")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1,doc:3,doc:4", reason: partial,
		},
		{
			name:    "list objects gains through two pending edges",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1"), tk("user:anne", "member", "group:g")},
			pending: [][]TupleKey{{tk("group:g#member", "viewer", "folder:f")}, {tk("folder:f", "parent", "doc:3")}},
			query:   listObjects("doc", "viewer", "user:anne"), want: "doc:1,doc:3", reason: partial,
		},
		{
			name:    "list objects through an exclusion",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1"), tk("user:anne", "viewer", "doc:2")},
			pending: [][]TupleKey{{tk("user:anne", "blocked", "doc:2")}},
			query:   listObjects("doc", "can_view", "user:anne"), want: "doc:1", reason: behind,
		},
		{
			name:    "list users of another object",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1")},
			pending: [][]TupleKey{{tk("user:bob", "viewer", "doc:2")}},
			query:   listUsers("doc:1", "viewer"), want: "user:anne", reason: irrelevant,
		},
		{
			name:    "list users gains a user through a group",
			applied: []TupleKey{tk("user:anne", "viewer", "doc:1"), tk("group:eng#member", "editor", "doc:1")},
			pending: [][]TupleKey{{tk("user:bob", "member", "group:eng")}},
			query:   listUsers("doc:1", "viewer"), want: "user:anne,user:bob", reason: behind,
		},
		{
			// Checks find relevant changes in SQL, so the number of pending
			// changes does not matter to them.
			name:    "many pending changes",
			pending: [][]TupleKey{many, {tk("user:bob", "member", "group:other")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "false", reason: irrelevant,
		},
		{
			name:    "too many pending changes and a relevant one",
			pending: [][]TupleKey{many, {tk("user:anne", "viewer", "doc:1")}},
			query:   check("user:anne", "viewer", "doc:1"), want: "true", reason: behind,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			model := tc.model
			if model == "" {
				model = testModel
			}
			e, storeID := newTestEngineWith(t, model, Options{
				manualApply: true, ViewWait: time.Millisecond,
				MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
			})
			if len(tc.applied) > 0 {
				mustWrite(t, e, storeID, &WriteRequest{Writes: tc.applied})
				drainView(t, e, storeID)
			}
			var revision string
			for _, keys := range tc.pending {
				req := &WriteRequest{}
				for _, k := range keys {
					if k.Object[0] == '-' {
						k.Object = k.Object[1:]
						req.Deletes = append(req.Deletes, k)
					} else {
						req.Writes = append(req.Writes, k)
					}
				}
				revision = mustWrite(t, e, storeID, req)
			}
			c := Consistency{MinRevision: revision}
			if tc.higher {
				c = Consistency{Preference: ConsistencyHigher}
			}
			before := fallbacks(t, reader)
			got := runGebraQuery(ctx, e, storeID, "", c, tc.query)
			after := fallbacks(t, reader)
			assert.Equal(t, tc.want, got.String(), tc.query.String())
			assert.Equal(t, int64(1), after[tc.reason]-before[tc.reason], "fallbacks %v", after)
		})
	}
}

// TestPendingRelevanceFuzz generates random models and tuples, holds random
// writes pending, and checks that queries with the revision of the last write
// or HIGHER_CONSISTENCY answer as graph evaluation of the latest tuples does,
// whether the view answers them or not. GEBRA_PENDING_FUZZ sets the number of
// models.
func TestPendingRelevanceFuzz(t *testing.T) {
	n := 20
	if v, _ := strconv.Atoi(os.Getenv("GEBRA_PENDING_FUZZ")); v > 0 {
		n = v
	}
	seed, _ := strconv.ParseUint(os.Getenv("GEBRA_FUZZ_SEED"), 10, 64)
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	t.Logf("seed %d", seed)
	ctx := context.Background()
	reader := sdkmetric.NewManualReader()
	db, dialect := newTestDB(t, testSuite(t.Name()))
	e, err := New(ctx, logx.NewNoopLogger(), db, dialect, Options{
		manualApply: true, ViewWait: time.Millisecond,
		MeterProvider: sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)),
	})
	require.NoError(t, err)
	t.Cleanup(e.Close)

	var models, queries int
	for i := range n {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		g := newModelGen(rng)
		if rng.IntN(4) > 0 {
			unionize(g.m, rng)
		}
		if _, err := compileModel(g.m, DefaultConditionCostLimit); err != nil {
			continue
		}
		models++
		mj := modelJSON(g.m)
		st, err := e.CreateStore(ctx, "fuzz")
		require.NoError(t, err)
		// Queries that name the older of two copies of the model evaluate
		// the graph with the latest tuples.
		var graphModelID string
		for range 2 {
			var m AuthorizationModel
			require.NoError(t, json.Unmarshal([]byte(mj), &m))
			id, err := e.WriteAuthorizationModel(ctx, st.ID, &m)
			require.NoError(t, err)
			if graphModelID == "" {
				graphModelID = id
			}
		}
		drainView(t, e, st.ID)

		pool := g.tuples(50)
		present := map[int]bool{}
		req := &WriteRequest{}
		for j, k := range pool {
			if rng.IntN(2) == 0 {
				req.Writes = append(req.Writes, k)
				present[j] = true
			}
		}
		if len(req.Writes) > 0 {
			mustWrite(t, e, st.ID, req)
		}
		drainView(t, e, st.ID)
		var revision string
		for range 1 + rng.IntN(3) {
			req := &WriteRequest{}
			for _, j := range rng.Perm(len(pool))[:1+rng.IntN(min(3, len(pool)))] {
				if present[j] {
					req.Deletes = append(req.Deletes, pool[j])
				} else {
					req.Writes = append(req.Writes, pool[j])
				}
				present[j] = !present[j]
			}
			revision = mustWrite(t, e, st.ID, req)
		}

		for k, q := range g.queries(40) {
			if q.Kind == kindExpand {
				continue
			}
			q.Contextual = nil
			c := Consistency{MinRevision: revision}
			if k%3 == 0 {
				c = Consistency{Preference: ConsistencyHigher}
			}
			got := runGebraQuery(ctx, e, st.ID, "", c, q)
			want := runGebraQuery(ctx, e, st.ID, graphModelID, Consistency{}, q)
			require.True(t, got.same(want), "seed %d model %d: %s: got %s, want %s\nmodel: %s", seed, i, q, got, want, mj)
			queries++
		}
	}
	reasons := fallbacks(t, reader)
	t.Logf("models %d, queries %d, view used %d, graph %d, partly graph %d", models, queries,
		reasons["view_irrelevant_pending"], reasons["view_behind"], reasons["view_behind_partial"])
	require.Positive(t, reasons["view_irrelevant_pending"])
	require.Positive(t, reasons["view_behind"])
	require.Positive(t, reasons["view_behind_partial"])
}
