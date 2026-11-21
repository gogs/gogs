package gebra

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

// unionize turns most intersections and exclusions of the model into unions
// and drops most conditions, so that many relations are materialized and the
// view has work to do.
func unionize(m *AuthorizationModel, rng *rand.Rand) {
	var walk func(u *Userset)
	walk = func(u *Userset) {
		switch {
		case u.Intersection != nil && rng.IntN(10) < 7:
			u.Union, u.Intersection = u.Intersection, nil
		case u.Difference != nil && rng.IntN(10) < 7:
			u.Union = &Usersets{Child: []*Userset{u.Difference.Base, u.Difference.Subtract}}
			u.Difference = nil
		}
		for _, us := range []*Usersets{u.Union, u.Intersection} {
			if us != nil {
				for _, c := range us.Child {
					walk(c)
				}
			}
		}
		if u.Difference != nil {
			walk(u.Difference.Base)
			walk(u.Difference.Subtract)
		}
	}
	for _, td := range m.TypeDefinitions {
		for _, u := range td.Relations {
			walk(u)
		}
		if td.Metadata == nil {
			continue
		}
		for _, rm := range td.Metadata.Relations {
			for _, ref := range rm.DirectlyRelatedUserTypes {
				if ref.Condition != "" && rng.IntN(10) < 7 {
					ref.Condition = ""
				}
			}
			// Dropping conditions can leave duplicate restrictions.
			seen := map[string]bool{}
			rm.DirectlyRelatedUserTypes = slices.DeleteFunc(rm.DirectlyRelatedUserTypes, func(r *RelationReference) bool {
				k := fmt.Sprintf("%s#%s*%v@%s", r.Type, r.Relation, r.Wildcard != nil, r.Condition)
				dup := seen[k]
				seen[k] = true
				return dup
			})
		}
	}
}

// modelVariant returns a copy of the model with some unions turned into
// intersections and some intersections into unions, which changes what the
// view materializes and keeps every tuple valid.
func modelVariant(m *AuthorizationModel, rng *rand.Rand) *AuthorizationModel {
	b, _ := json.Marshal(m)
	var c AuthorizationModel
	_ = json.Unmarshal(b, &c)
	var walk func(u *Userset)
	walk = func(u *Userset) {
		switch {
		case u.Union != nil && len(u.Union.Child) > 1 && rng.IntN(3) == 0:
			u.Intersection, u.Union = u.Union, nil
		case u.Intersection != nil && rng.IntN(2) == 0:
			u.Union, u.Intersection = u.Intersection, nil
		}
		for _, us := range []*Usersets{u.Union, u.Intersection} {
			if us != nil {
				for _, c := range us.Child {
					walk(c)
				}
			}
		}
		if u.Difference != nil {
			walk(u.Difference.Base)
			walk(u.Difference.Subtract)
		}
	}
	for _, td := range c.TypeDefinitions {
		for _, u := range td.Relations {
			walk(u)
		}
	}
	return &c
}

// TestViewWriteFuzz generates random models, mostly unions so that the view
// maintains many relations, and toggles random tuples in random batches. It
// also writes variants of the model, whose views the applier builds in chunks
// of one to four tuples or sets while the writes go on. After random applier
// runs, which apply one or several writes and run one build chunk each, it
// checks that the view equals a full build of the live model and compares
// view checks with graph evaluation. GEBRA_WRITE_FUZZ sets the number of
// models.
func TestViewWriteFuzz(t *testing.T) {
	n, _ := strconv.Atoi(os.Getenv("GEBRA_WRITE_FUZZ"))
	if n <= 0 {
		t.Skip("set GEBRA_WRITE_FUZZ to the number of random models to run")
	}
	seed, _ := strconv.ParseUint(os.Getenv("GEBRA_FUZZ_SEED"), 10, 64)
	if seed == 0 {
		seed = uint64(time.Now().UnixNano())
	}
	steps := 60
	if v, _ := strconv.Atoi(os.Getenv("GEBRA_WRITE_FUZZ_STEPS")); v > 0 {
		steps = v
	}
	t.Logf("seed %d", seed)
	ctx := context.Background()
	open := func(name string, opts Options) *Engine {
		db, dialect := newTestDB(t, testSuite(t.Name()+"-"+name))
		e, err := New(ctx, logx.NewNoopLogger(), db, dialect, opts)
		require.NoError(t, err)
		t.Cleanup(e.Close)
		return e
	}
	e := open("manual", Options{manualApply: true})

	var models, writes, toggles, checks, materialized, swaps, flips int
	for i := range n {
		rng := rand.New(rand.NewPCG(seed, uint64(i)))
		g := newModelGen(rng)
		unionize(g.m, rng)
		cm, err := compileModel(g.m, DefaultConditionCostLimit)
		if err != nil {
			continue
		}
		models++
		// The engine runs no applier of its own, so the chunk size can
		// change between stores.
		e.opts.buildChunkSize = 1 + rng.IntN(4)
		for _, td := range cm.types {
			for _, r := range td.relations {
				if r.materialized {
					materialized++
				}
			}
		}
		mj := modelJSON(g.m)
		st, err := e.CreateStore(ctx, "fuzz")
		require.NoError(t, err)
		var m AuthorizationModel
		require.NoError(t, json.Unmarshal([]byte(mj), &m))
		_, err = e.WriteAuthorizationModel(ctx, st.ID, &m)
		require.NoError(t, err)
		drainView(t, e, st.ID)
		pool := g.tuples(80)
		present := map[int]bool{}
		for step := range steps {
			if rng.IntN(20) == 0 {
				variant := &m
				if rng.IntN(3) > 0 {
					variant = modelVariant(&m, rng)
				}
				if _, err := e.WriteAuthorizationModel(ctx, st.ID, variant); err == nil {
					swaps++
				}
			}
			req := &WriteRequest{}
			for _, j := range rng.Perm(len(pool))[:1+rng.IntN(min(6, len(pool)))] {
				if present[j] {
					req.Deletes = append(req.Deletes, pool[j])
				} else {
					req.Writes = append(req.Writes, pool[j])
				}
				present[j] = !present[j]
				toggles++
			}
			_, err := e.Write(ctx, st.ID, req)
			require.NoError(t, err, "model %d step %d: %s", i, step, mj)
			writes++
			msg := fmt.Sprintf("seed %d model %d step %d: %s", seed, i, step, mj)
			if rng.IntN(3) == 0 || step%15 == 14 || step == steps-1 {
				// Builds go on across steps and complete every 30 steps,
				// so that writes land in every phase of a build.
				if step%30 == 29 || step == steps-1 {
					if !modelLive(t, e, st.ID) {
						flips++
					}
					drainView(t, e, st.ID)
				} else {
					applyPending(t, e, st.ID)
				}
				require.Equal(t, rebuiltSnapshot(t, e, st.ID), viewSnapshot(t, e.db.handle(), st.ID), msg)
				requireViewCounts(t, e.db.handle(), st.ID, msg)
			} else {
				for range rng.IntN(4) {
					_, err := e.applyBatch(ctx, st.ID)
					require.NoError(t, err, msg)
				}
			}
			// At other steps writes can be pending, and graph evaluation
			// reads the tuples as of the writes that the view includes.
			if step%15 == 14 || step%15 == 7 {
				checks += compareViewWithGraph(t, e, st.ID, g, msg)
			}
		}
	}
	t.Logf("models %d, materialized relations %d, writes %d, tuple toggles %d, view-vs-graph checks %d, model writes %d, flips %d",
		models, materialized, writes, toggles, checks, swaps, flips)
}

// applyPending runs applier batches until the store has no pending write.
func applyPending(t *testing.T, e *Engine, storeID string) {
	t.Helper()
	for {
		var n int64
		_, err := e.db.handle().queryRow(context.Background(), "SELECT COUNT(*) FROM gebra_inbox WHERE store_id = ?", []any{storeID}, &n)
		require.NoError(t, err)
		if n == 0 {
			return
		}
		_, err = e.applyBatch(context.Background(), storeID)
		require.NoError(t, err)
	}
}

// compareViewWithGraph checks every relation of every object for a set of
// subjects with the view and with graph evaluation, and returns how many
// checks it compared.
func compareViewWithGraph(t *testing.T, e *Engine, storeID string, g *modelGen, msg string) int {
	t.Helper()
	ctx := context.Background()
	viewEv, err := e.newEvaluator(ctx, storeID, "", Consistency{}, nil, map[string]any{"on": true, "x": 1, "limit": 2}, false)
	require.NoError(t, err)
	subjects := []string{"user:u0", "user:u1", "user:u2", "user:u3", "user:*"}
	for _, typ := range g.types {
		subjects = append(subjects, typ+":a", typ+":b#"+g.rels[typ][0])
	}
	n := 0
	for _, typ := range g.types {
		for _, rel := range g.rels[typ] {
			for _, id := range g.objects {
				for _, sub := range subjects {
					q, err := parseCheck(viewEv.m, CheckTupleKey{User: sub, Relation: rel, Object: typ + ":" + id})
					require.NoError(t, err)
					viewEv.reset()
					fromView, errView := viewEv.check(q.obj, q.rel, q.sub)
					graphEv := viewEv.fork()
					graphEv.useView = false
					fromGraph, errGraph := graphEv.check(q.obj, q.rel, q.sub)
					require.Equal(t, errGraph != nil, errView != nil, "%s: check %s %s %s:%s", msg, sub, rel, typ, id)
					require.Equal(t, fromGraph, fromView, "%s: check %s %s %s:%s", msg, sub, rel, typ, id)
					n++
				}
			}
		}
	}
	return n
}

// FuzzParseDSL checks that the DSL parser never panics and that every model
// it accepts and compiles survives a JSON round trip unchanged.
func FuzzParseDSL(f *testing.F) {
	f.Add(testModel)
	f.Add("model\n  schema 1.1\ntype user\ntype doc\n  relations\n    define viewer: [user, user:*, doc#viewer with c] or (a and b) but not c\n")
	f.Add("model\n  schema 1.1\ncondition c(x: map<list<int>>, y: ipaddress) {\n  y.in_cidr(\"10.0.0.0/8\") && x[\"a\"][0] > 1\n}\n")
	f.Add("model\n schema 1.1\ntype a\n  attributes\n    n: int\n  relations\n    define r: [a] or s from r\n")
	f.Fuzz(func(t *testing.T, src string) {
		m, err := ParseDSL(src)
		if err != nil {
			return
		}
		cm, err := compileModel(m, DefaultConditionCostLimit)
		if err != nil {
			return
		}
		b, err := json.Marshal(m)
		require.NoError(t, err)
		var back AuthorizationModel
		require.NoError(t, json.Unmarshal(b, &back))
		cm2, err := compileModel(&back, DefaultConditionCostLimit)
		require.NoError(t, err)
		require.Equal(t, len(cm.types), len(cm2.types))
	})
}

// FuzzConditionEval evaluates a condition over every parameter type with
// arbitrary JSON contexts. Evaluation must not panic, and a context that
// validates must either evaluate or report a missing or invalid parameter.
func FuzzConditionEval(f *testing.F) {
	c := &Condition{
		Name: "c",
		Expression: `i > 1 && u > 1u && d > 0.5 && b && s != "" && dur > duration("1s") && ts > timestamp("2020-01-01T00:00:00Z") &&
			"a" in l && m["k"] > 0 && ip.in_cidr("10.0.0.0/8")`,
		Parameters: map[string]*ConditionParamTypeRef{
			"i": {TypeName: "TYPE_NAME_INT"}, "u": {TypeName: "TYPE_NAME_UINT"}, "d": {TypeName: "TYPE_NAME_DOUBLE"},
			"b": {TypeName: "TYPE_NAME_BOOL"}, "s": {TypeName: "TYPE_NAME_STRING"}, "dur": {TypeName: "TYPE_NAME_DURATION"},
			"ts": {TypeName: "TYPE_NAME_TIMESTAMP"}, "ip": {TypeName: "TYPE_NAME_IPADDRESS"},
			"l": {TypeName: "TYPE_NAME_LIST", GenericTypes: []*ConditionParamTypeRef{{TypeName: "TYPE_NAME_STRING"}}},
			"m": {TypeName: "TYPE_NAME_MAP", GenericTypes: []*ConditionParamTypeRef{{TypeName: "TYPE_NAME_INT"}}},
		},
	}
	cc, err := compileCondition("c", c, DefaultConditionCostLimit)
	require.NoError(f, err)
	f.Add(`{"i":2,"u":2,"d":1.5,"b":true,"s":"x","dur":"2s","ts":"2021-01-01T00:00:00Z","l":["a"],"m":{"k":1},"ip":"10.1.1.1"}`, `{}`)
	f.Add(`{"i":2.5}`, `{"u":-1,"ip":"::1"}`)
	f.Add(`{"l":[1,2],"m":[]}`, `{"ts":"x","dur":"1y"}`)
	f.Fuzz(func(t *testing.T, tupleJSON, requestJSON string) {
		var tupleCtx, requestCtx map[string]any
		if json.Unmarshal([]byte(tupleJSON), &tupleCtx) != nil || json.Unmarshal([]byte(requestJSON), &requestCtx) != nil {
			return
		}
		ok, err := cc.eval(tupleCtx, requestCtx)
		if err != nil {
			require.ErrorIs(t, err, ErrInvalidArgument)
			require.False(t, ok)
		}
		_ = cc.validateContext(tupleCtx)
	})
}
