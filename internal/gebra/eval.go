package gebra

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// evaluator answers checks for one request. It is not safe for concurrent use;
// fork it to evaluate in parallel.
type evaluator struct {
	ctx        context.Context
	e          *Engine
	db         dbtx
	storeID    string
	m          *compiledModel
	requestCtx map[string]any

	// consistency and revision are the request's consistency and the inbox
	// ID of its minimum revision.
	consistency Consistency
	revision    int64

	// useView is true when the materialized view describes the data the
	// request sees: the request uses the live model and has no contextual
	// tuples.
	useView bool
	// gen is the serving generation of the view, which the view fragments
	// of the evaluator's statements read.
	gen int64
	// behind is true while the view lacks writes that the request asks for,
	// until settle decides how the request runs.
	behind bool
	// verified is true when the evaluator read the model of the view in its
	// snapshot.
	verified bool
	// pending holds the store's pending changes that prepare read for an
	// evaluator that is behind, and pendingOverflow tells that there were
	// more than maxPendingChanges, which it did not keep.
	pending         []pendingChange
	pendingOverflow bool
	// partialSources holds what relevantSources returned for a query that
	// settle let evaluate only what pending changes affect.
	partialSources []string
	// applied makes tuple reads return the tuples as of the writes that the
	// view includes rather than the latest ones, so that a query that uses
	// the view reads one point in time. It is true when useView is, and stays
	// true for evaluators that skip the view to evaluate rewrites.
	applied bool
	// snap is the snapshot that the request's statements run in, or nil for
	// a request of a single statement.
	snap *snapshot
	// explicitOnly makes check report whether the subject is named by a
	// tuple on a path that can grant the relation: wildcards do not match
	// other subjects, intersections need any child, exclusions only their
	// base, and conditions hold. ListUsers lists a user only when this is
	// true too, so that users that a wildcard covers are not listed one by
	// one. useView is false for such an evaluator.
	explicitOnly bool
	// explicit is the explicitOnly evaluator of a ListUsers worker, created
	// on first use.
	explicit *evaluator
	// contextual holds the request's contextual tuples by set key.
	contextual map[string][]*tuple
	// overridden holds the keys of contextual tuples, which replace stored
	// tuples with the same key.
	overridden map[string]struct{}

	reads map[string][]*tuple
	memo  map[string]checkResult
	// cycles counts how often evaluation reached a check that was still in
	// progress. Results computed across such a cut are not memoized because
	// they assumed a value for the check in progress.
	cycles int
	// negations counts the exclusions whose subtracted side is being
	// evaluated. A cut assumes the value that denies: false for the check in
	// progress, or true when an odd number of exclusions lie between it and
	// the cut, so that a relation that excludes itself never grants.
	negations int
}

type checkResult struct {
	allowed bool
	err     error
	done    bool
	// negations is the evaluator's negations when the check started.
	negations int
}

// newEvaluator resolves the store and model of a request and parses its
// contextual tuples. An empty modelID selects the store's live model, the one
// of its serving view, or the latest model before the store's first view is
// complete. The evaluator uses the view only for the live model. When the
// view lacks writes that c asks for, the evaluator is behind, and settle
// decides whether the view can still answer the query.
//
// With snapshot, the evaluator starts its snapshot first and reads the
// store's models in it, so that the models it uses are those of the view in
// the snapshot. Otherwise it runs its statements outside of a snapshot until
// begin starts one.
func (e *Engine) newEvaluator(ctx context.Context, storeID, modelID string, c Consistency, contextual []TupleKey, requestCtx map[string]any, snapshot bool) (_ *evaluator, err error) {
	revision, err := minRevision(c)
	if err != nil {
		return nil, err
	}
	ev := &evaluator{ctx: ctx, e: e, db: e.db.handle(), storeID: storeID, requestCtx: requestCtx, consistency: c, revision: revision}
	if snapshot {
		if ev.snap, err = e.beginSnapshot(ctx); err != nil {
			return nil, err
		}
		ev.verified = true
		defer func() {
			if err != nil {
				ev.end()
			}
		}()
	}
	var sv storeView
	err = ev.run(func(db dbtx) error {
		sv, err = storeModels(ctx, db, storeID)
		if err != nil {
			return err
		}
		if modelID == "" {
			modelID = sv.LiveModelID
			if sv.Gen == 0 {
				modelID = sv.LatestModelID
			}
		}
		ev.m, err = e.model(ctx, db, storeID, modelID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if hook := e.opts.hooks.afterStoreModels; hook != nil {
		hook(ctx)
	}
	ev.useView, ev.gen = sv.Gen != 0 && modelID == sv.LiveModelID, sv.Gen
	var fallback string
	switch {
	case ev.useView:
	case sv.Gen == 0:
		fallback = "no_live_view"
	case modelID > sv.LiveModelID:
		// Model IDs sort by creation time.
		fallback = "model_not_live"
	default:
		fallback = "older_model"
	}
	if err := ev.setContextual(contextual); err != nil {
		return nil, err
	}
	if fallback == "" && !ev.useView {
		fallback = "contextual_tuples"
	}
	if ev.useView {
		// A view that lacks any pending write fails HIGHER_CONSISTENCY,
		// whichever ID the write has.
		through := revision
		if c.Preference == ConsistencyHigher {
			through = math.MaxInt64
		}
		if through > 0 {
			var pending int64
			err := ev.run(func(db dbtx) (err error) {
				pending, err = pendingThrough(ctx, db, storeID, through)
				return err
			})
			if err != nil {
				return nil, err
			}
			ev.behind = pending > 0
		} else if c.Preference == "" || c.Preference == ConsistencyUnspecified {
			var stale bool
			err := ev.run(func(db dbtx) (err error) {
				stale, err = e.viewStale(ctx, db, storeID)
				return err
			})
			if err != nil {
				return nil, err
			}
			if stale {
				ev.useView, fallback = false, "view_stale"
			}
		}
	}
	ev.applied = ev.useView
	ev.setPath()
	if fallback != "" {
		e.recordFallback(ctx, fallback)
	}
	if ev.snap != nil {
		if err := ev.prepare(); err != nil {
			return nil, err
		}
	}
	ev.reset()
	return ev, nil
}

// setPath records on the operation whether the view answers it.
func (ev *evaluator) setPath() {
	if o := opFromContext(ev.ctx); o != nil {
		path := "graph"
		if ev.useView {
			path = "view"
		}
		o.setPath(path)
	}
}

func (e *Engine) recordFallback(ctx context.Context, reason string) {
	if o := opFromContext(ctx); o != nil {
		o.span.SetAttributes(attribute.String("gebra.view.fallback", reason))
	}
	e.tel.viewFallbacks.Add(ctx, 1, metric.WithAttributes(attribute.String("gebra.view.fallback", reason)))
}

// begin starts the snapshot that the evaluator and its forks run their
// statements in until end, and prepares it.
func (ev *evaluator) begin() error {
	snap, err := ev.e.beginSnapshot(ev.ctx)
	if err != nil {
		return err
	}
	ev.snap = snap
	ev.verified = false
	return ev.prepare()
}

// prepare reads, in the snapshot, the serving generation of the view and its
// model unless the evaluator read the store's models in it, and the pending
// changes when the evaluator is behind. When another generation with another
// model became the serving one since newEvaluator, the applier no longer
// maintains the evaluator's, and the evaluator evaluates the graph instead.
// The serving generation of the snapshot includes every write applied in it,
// so the evaluator reads that one, which also holds the writes that a flip
// since newEvaluator applied to the new generation only.
func (ev *evaluator) prepare() error {
	ev.pending, ev.pendingOverflow = nil, false
	if !ev.useView || ev.verified && !ev.behind {
		return nil
	}
	type row struct {
		Gen     int64
		ModelID string
		pendingChange
	}
	var rows []row
	err := ev.run(func(db dbtx) (err error) {
		if !ev.behind {
			rows, err = queryStructs[row](ev.ctx, db, "SELECT gen, model_id FROM gebra_view_state WHERE store_id = ?", ev.storeID)
			return err
		}
		// One more row than the limit tells that the limit is exceeded.
		rows, err = queryStructs[row](ev.ctx, db, `SELECT v.gen, v.model_id, COALESCE(p.object_type, '') AS object_type, COALESCE(p.object_id, '') AS object_id,
	COALESCE(p.relation, '') AS relation, COALESCE(p.user_type, '') AS user_type, COALESCE(p.user_id, '') AS user_id,
	COALESCE(p.user_relation, '') AS user_relation, COALESCE(p.condition_name, '') AS condition_name, COALESCE(p.deleted, FALSE) AS deleted
FROM gebra_view_state v LEFT JOIN gebra_pending p ON p.store_id = v.store_id
WHERE v.store_id = ? LIMIT ?`, ev.storeID, maxPendingChanges+1)
		return err
	})
	if err != nil {
		return errors.Wrap(err, "read view state")
	}
	if len(rows) == 0 || rows[0].Gen == 0 || rows[0].ModelID != ev.m.id {
		ev.useView, ev.applied, ev.behind = false, false, false
		ev.setPath()
		ev.e.recordFallback(ev.ctx, "view_flipped")
		return nil
	}
	ev.gen, ev.verified = rows[0].Gen, true
	if !ev.behind {
		return nil
	}
	ev.pendingOverflow = len(rows) > maxPendingChanges
	for _, r := range rows {
		if r.ObjectType != "" && !ev.pendingOverflow {
			ev.pending = append(ev.pending, r.pendingChange)
		}
	}
	return nil
}

// settle decides how a query of an evaluator that is behind runs. When no
// pending change can affect its answer, the view answers it, and the
// evaluator reads the latest tuples wherever it does not read the view: the
// answer is then that of the latest state, which includes every write that
// the query asks for. Otherwise the query evaluates the graph, after waiting
// up to Options.ViewWait for the view with wait. With partial, when only
// members and edges are relevant, the evaluator keeps them in
// partialSources, and the caller reads the view and evaluates the graph only
// for what they affect. A nil scope makes every pending change relevant.
func (ev *evaluator) settle(scope *pendingScope, wait, partial bool) error {
	if !ev.behind {
		return nil
	}
	if !ev.useView {
		ev.behind = false
		return nil
	}
	relevant, sources, err := ev.relevantSources(scope)
	if err != nil {
		return err
	}
	if relevant && wait {
		ev.end()
		need, err := requiredWrite(ev.ctx, ev.db, ev.storeID, ev.consistency, ev.revision)
		if err != nil {
			return err
		}
		caughtUp, err := ev.e.waitForView(ev.ctx, ev.db, ev.storeID, need)
		if err != nil {
			return err
		}
		ev.behind = !caughtUp
		if err := ev.begin(); err != nil {
			return err
		}
		if !ev.behind || !ev.useView {
			ev.behind = false
			return nil
		}
		if relevant, sources, err = ev.relevantSources(scope); err != nil {
			return err
		}
	}
	ev.behind = false
	switch {
	case relevant && partial && sources != nil:
		ev.useView, ev.applied, ev.partialSources = false, false, sources
		ev.e.recordFallback(ev.ctx, "view_behind_partial")
	case relevant:
		ev.useView, ev.applied = false, false
		ev.e.recordFallback(ev.ctx, "view_behind")
	default:
		ev.applied = false
		ev.e.recordFallback(ev.ctx, "view_irrelevant_pending")
	}
	ev.setPath()
	return nil
}

// useGraph makes the evaluator evaluate the graph, for a query that pending
// changes can affect.
func (ev *evaluator) useGraph() {
	if !ev.useView {
		return
	}
	ev.useView, ev.applied, ev.behind = false, false, false
	ev.setPath()
	ev.e.recordFallback(ev.ctx, "view_behind")
}

// end ends the evaluator's snapshot, if any.
func (ev *evaluator) end() {
	if ev.snap != nil {
		ev.snap.end()
		ev.snap = nil
	}
}

// run runs fn with the database handle of the evaluator's statements.
func (ev *evaluator) run(fn func(db dbtx) error) error {
	if ev.snap == nil {
		return fn(ev.db)
	}
	return ev.snap.run(fn)
}

func (ev *evaluator) setContextual(keys []TupleKey) error {
	ts, err := parseContextualTuples(ev.m, keys)
	if err != nil {
		return err
	}
	ev.contextual = map[string][]*tuple{}
	ev.overridden = map[string]struct{}{}
	for _, t := range ts {
		k := setKey(t.objectType, t.objectID, t.relation)
		ev.contextual[k] = append(ev.contextual[k], t)
		ev.overridden[t.keyString()] = struct{}{}
	}
	if len(ts) > 0 {
		ev.useView, ev.applied = false, false
	}
	return nil
}

func (ev *evaluator) reset() {
	ev.reads = map[string][]*tuple{}
	ev.memo = map[string]checkResult{}
}

// fork returns an evaluator for the same request with its own caches. It
// shares the snapshot, which may run statements of evaluators in parallel.
func (ev *evaluator) fork() *evaluator {
	f := *ev
	f.reset()
	f.cycles = 0
	f.negations = 0
	f.explicit = nil
	return &f
}

func parseContextualTuples(m *compiledModel, keys []TupleKey) ([]*tuple, error) {
	seen := make(map[string]struct{}, len(keys))
	out := make([]*tuple, 0, len(keys))
	for _, k := range keys {
		t, err := parseTupleKey(k)
		if err != nil {
			return nil, err
		}
		if err := m.validateTuple(t); err != nil {
			return nil, err
		}
		if _, ok := seen[t.keyString()]; ok {
			return nil, invalidf("duplicate contextual tuple %s", t.keyString())
		}
		seen[t.keyString()] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

type checkQuery struct {
	obj object
	rel string
	sub subject
}

func parseCheck(m *compiledModel, k CheckTupleKey) (*checkQuery, error) {
	obj, err := parseObject(k.Object)
	if err != nil {
		return nil, err
	}
	sub, err := parseSubject(k.User)
	if err != nil {
		return nil, err
	}
	if m.relation(obj.typ, k.Relation) == nil {
		return nil, invalidf("relation %s#%s is not defined", obj.typ, k.Relation)
	}
	if err := m.validateSubject(sub); err != nil {
		return nil, err
	}
	return &checkQuery{obj: obj, rel: k.Relation, sub: sub}, nil
}

// readTuples returns the tuples of a set that the model allows, with
// contextual tuples in place of stored tuples that have the same key.
func (ev *evaluator) readTuples(typ, id, rel string) ([]*tuple, error) {
	k := setKey(typ, id, rel)
	if ts, ok := ev.reads[k]; ok {
		return ts, nil
	}
	var q tupleQuery
	q.add(ev.applied, "", ev.storeID, typ, id, rel)
	return ev.read(k, k, q)
}

// tupleColumns are the columns of a stored tuple that evaluation reads.
const tupleColumns = "object_type, object_id, relation, user_type, user_id, user_relation, condition_name, condition_context"

// tupleQuery is a union of reads of stored tuples, each of one set.
type tupleQuery struct {
	sql  strings.Builder
	args []any
}

// add adds a read of the tuples of the set that match the filter on user
// columns. The arguments are the store ID, the set's object type, object ID,
// and relation, and the filter's arguments.
//
// With applied, it reads the tuples as of the writes that the view includes:
// gebra_tuple without the keys that pending changes touch, plus the rows that
// the earliest pending change of a key deleted. A key's pending changes
// alternate between writes and deletes in inbox ID order, so the earliest
// says whether the key existed before them and with which row.
func (q *tupleQuery) add(applied bool, filter string, args ...any) {
	const set = " WHERE store_id = ? AND object_type = ? AND object_id = ? AND relation = ?"
	if q.sql.Len() > 0 {
		q.sql.WriteString(" UNION ALL ")
	}
	if !applied {
		q.sql.WriteString("SELECT " + tupleColumns + " FROM gebra_tuple" + set + filter)
		q.args = append(q.args, args...)
		return
	}
	q.sql.WriteString("SELECT " + tupleColumns + " FROM gebra_tuple t" + set + filter + ` AND NOT EXISTS (
	SELECT 1 FROM gebra_pending p WHERE p.store_id = t.store_id AND p.object_type = t.object_type AND p.object_id = t.object_id
		AND p.relation = t.relation AND p.user_type = t.user_type AND p.user_id = t.user_id AND p.user_relation = t.user_relation)
UNION ALL SELECT ` + tupleColumns + " FROM gebra_pending p" + set + filter + ` AND p.deleted = TRUE AND NOT EXISTS (
	SELECT 1 FROM gebra_pending q WHERE q.store_id = p.store_id AND q.object_type = p.object_type AND q.object_id = p.object_id
		AND q.relation = p.relation AND q.user_type = p.user_type AND q.user_id = p.user_id AND q.user_relation = p.user_relation
		AND q.inbox_id < p.inbox_id)`)
	q.args = append(q.args, args...)
	q.args = append(q.args, args...)
}

// readThis returns the tuples of a set that can grant sub directly: the ones
// that name sub or its type's wildcard, and the usersets. Unlike readTuples,
// it does not read every member of a large set.
func (ev *evaluator) readThis(obj object, rel string, sub subject) ([]*tuple, error) {
	k := setKey(obj.typ, obj.id, rel)
	if ts, ok := ev.reads[k]; ok {
		return ts, nil
	}
	cacheKey := k + "\x00" + sub.typ + ":" + sub.id
	if ts, ok := ev.reads[cacheKey]; ok {
		return ts, nil
	}
	// Each part compares every key column with a single value and the parts
	// are joined by a union rather than an OR, so that the index bounds them
	// even in the generic plan of a prepared statement. That plan turns an OR,
	// or an IN list of parameters, into a filter over every tuple of the set.
	// Userset subjects only match userset tuples, which the last part reads.
	var q tupleQuery
	q.add(ev.applied, " AND user_type = ? AND user_id = ? AND user_relation = ''", ev.storeID, obj.typ, obj.id, rel, sub.typ, sub.id)
	if sub.id != "*" {
		q.add(ev.applied, " AND user_type = ? AND user_id = '*' AND user_relation = ''", ev.storeID, obj.typ, obj.id, rel, sub.typ)
	}
	usersets := map[string]bool{}
	for _, ref := range ev.m.relation(obj.typ, rel).direct {
		if ref.Relation == "" || usersets[ref.Type+"#"+ref.Relation] {
			continue
		}
		usersets[ref.Type+"#"+ref.Relation] = true
		q.add(ev.applied, " AND user_type = ? AND user_relation = ?", ev.storeID, obj.typ, obj.id, rel, ref.Type, ref.Relation)
	}
	return ev.read(cacheKey, k, q)
}

// read returns the stored tuples that the query selects from the set k and
// the model allows, with contextual tuples in place of stored tuples that have
// the same key, and caches them under cacheKey.
func (ev *evaluator) read(cacheKey, k string, q tupleQuery) ([]*tuple, error) {
	var rows []tupleRow
	err := ev.run(func(db dbtx) (err error) {
		rows, err = queryStructs[tupleRow](ev.ctx, db, q.sql.String(), q.args...)
		return err
	})
	if err != nil {
		return nil, errors.Wrap(err, "read tuples")
	}
	if hook := ev.e.opts.hooks.afterTupleRead; hook != nil {
		hook(ev.ctx)
	}
	ts := make([]*tuple, 0, len(rows)+len(ev.contextual[k]))
	for _, r := range rows {
		t, err := r.tuple()
		if err != nil {
			return nil, err
		}
		if _, ok := ev.overridden[t.keyString()]; ok || !ev.m.allows(t) {
			continue
		}
		ts = append(ts, t)
	}
	ts = append(ts, ev.contextual[k]...)
	ev.reads[cacheKey] = ts
	return ts, nil
}

func (ev *evaluator) conditionHolds(t *tuple) (bool, error) {
	if t.conditionName == "" {
		return true, nil
	}
	return ev.m.conditions[t.conditionName].eval(t.conditionContext, ev.requestCtx)
}

// check reports whether sub has relation rel with obj.
func (ev *evaluator) check(obj object, rel string, sub subject) (bool, error) {
	if err := ev.ctx.Err(); err != nil {
		return false, err
	}
	r := ev.m.relation(obj.typ, rel)
	if r == nil {
		return false, nil
	}
	if sub.rel == rel && sub.typ == obj.typ && sub.id == obj.id {
		return true, nil
	}

	key := setKey(obj.typ, obj.id, rel) + "@" + sub.String()
	if res, ok := ev.memo[key]; ok {
		if !res.done {
			ev.cycles++
			return (ev.negations-res.negations)%2 == 1, nil
		}
		return res.allowed, res.err
	}
	ev.memo[key] = checkResult{negations: ev.negations}
	cycles := ev.cycles

	var allowed bool
	var err error
	if c := ev.compiler(sub); c != nil && c.relation(obj, rel) {
		allowed, err = ev.runCompiled(c)
	} else {
		allowed, err = ev.eval(obj, rel, r.rewrite, sub)
	}

	if ev.cycles == cycles {
		ev.memo[key] = checkResult{allowed: allowed, err: err, done: true}
	} else {
		delete(ev.memo, key)
	}
	return allowed, err
}

// compiler returns a compiler of checks of sub from the view, or nil when the
// evaluator does not use the view.
func (ev *evaluator) compiler(sub subject) *checkCompiler {
	if !ev.useView {
		return nil
	}
	return newCheckCompiler(ev.m, string(ev.e.db.dialect), ev.storeID, ev.gen, sub, ev.applied)
}

func (ev *evaluator) runCompiled(c *checkCompiler) (bool, error) {
	var allowed []int64
	err := ev.run(func(db dbtx) (err error) {
		allowed, err = queryColumn[int64](ev.ctx, db, "SELECT CASE WHEN "+c.sql.String()+" THEN 1 ELSE 0 END", c.args...)
		return err
	})
	if err != nil {
		return false, errors.Wrap(err, "check view")
	}
	return len(allowed) > 0 && allowed[0] == 1, nil
}

// runCompiledAlone runs a compiled check outside of a snapshot. The statement
// also reads the serving generation of the view, and current is false when it
// is no longer the evaluator's, in which case the answer is void: the applier
// no longer maintains the evaluator's generation, and a write that the query
// must see may be in the new generation only.
func (ev *evaluator) runCompiledAlone(c *checkCompiler) (allowed, current bool, err error) {
	var allowedN, gen int64
	args := append(c.args, ev.storeID)
	found, err := ev.db.queryRow(ev.ctx, "SELECT CASE WHEN "+c.sql.String()+` THEN 1 ELSE 0 END AS allowed,
	COALESCE((SELECT gen FROM gebra_view_state WHERE store_id = ?), 0) AS gen`, args, &allowedN, &gen)
	if err != nil {
		return false, false, errors.Wrap(err, "check view")
	}
	if !found || gen != ev.gen {
		return false, false, nil
	}
	return allowedN == 1, true, nil
}

// eval evaluates a node of the rewrite of obj#rel. Errors, e.g., missing
// condition parameters, only fail the check when the other operands do not
// decide it.
func (ev *evaluator) eval(obj object, rel string, u *Userset, sub subject) (bool, error) {
	switch {
	case u.This != nil:
		return ev.evalThis(obj, rel, sub)

	case u.ComputedUserset != nil:
		return ev.check(obj, u.ComputedUserset.Relation, sub)

	case u.TupleToUserset != nil:
		return ev.evalTupleToUserset(obj, u.TupleToUserset, sub)

	case u.Union != nil:
		var firstErr error
		for _, c := range u.Union.Child {
			ok, err := ev.eval(obj, rel, c, sub)
			if ok {
				return true, nil
			}
			if firstErr == nil {
				firstErr = err
			}
		}
		return false, firstErr

	case u.Intersection != nil && ev.explicitOnly:
		return ev.eval(obj, rel, &Userset{Union: u.Intersection}, sub)

	case u.Difference != nil && ev.explicitOnly:
		return ev.eval(obj, rel, u.Difference.Base, sub)

	case u.Intersection != nil:
		var firstErr error
		for _, c := range u.Intersection.Child {
			ok, err := ev.eval(obj, rel, c, sub)
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			if !ok {
				return false, nil
			}
		}
		return firstErr == nil, firstErr

	case u.Difference != nil:
		base, baseErr := ev.eval(obj, rel, u.Difference.Base, sub)
		if baseErr == nil && !base {
			return false, nil
		}
		ev.negations++
		subtract, subtractErr := ev.eval(obj, rel, u.Difference.Subtract, sub)
		ev.negations--
		if subtractErr == nil && subtract {
			return false, nil
		}
		if baseErr != nil {
			return false, baseErr
		}
		return subtractErr == nil, subtractErr
	}
	return false, nil
}

func (ev *evaluator) evalThis(obj object, rel string, sub subject) (bool, error) {
	ts, err := ev.readThis(obj, rel, sub)
	if err != nil {
		return false, err
	}
	var firstErr error
	record := func(err error) {
		if firstErr == nil {
			firstErr = err
		}
	}

	// Direct matches first, they need no further reads.
	for _, t := range ts {
		var match bool
		if t.userRelation == "" {
			match = sub.rel == "" && t.userType == sub.typ && (t.userID == sub.id || t.userID == "*" && !ev.explicitOnly)
		} else {
			match = t.userType == sub.typ && t.userID == sub.id && t.userRelation == sub.rel
		}
		if !match {
			continue
		}
		ok, err := ev.conditionHolds(t)
		if ok {
			return true, nil
		}
		record(err)
	}

	for _, t := range ts {
		if t.userRelation == "" {
			continue
		}
		ok, err := ev.followConditional(t, func() (bool, error) {
			return ev.check(object{typ: t.userType, id: t.userID}, t.userRelation, sub)
		})
		if ok {
			return true, nil
		}
		record(err)
	}
	return false, firstErr
}

// followConditional reports whether the tuple's condition holds and next
// returns true. A condition that cannot be evaluated, e.g., for a missing
// parameter, only fails the result when next returns true, because a tuple
// that leads nowhere grants nothing whatever its condition says.
func (ev *evaluator) followConditional(t *tuple, next func() (bool, error)) (bool, error) {
	holds, condErr := ev.conditionHolds(t)
	if condErr == nil && !holds {
		return false, nil
	}
	ok, err := next()
	if err != nil {
		return false, err
	}
	if !ok {
		return false, nil
	}
	return condErr == nil, condErr
}

func (ev *evaluator) evalTupleToUserset(obj object, ttu *TupleToUserset, sub subject) (bool, error) {
	ts, err := ev.readTuples(obj.typ, obj.id, ttu.Tupleset.Relation)
	if err != nil {
		return false, err
	}
	var firstErr error
	for _, t := range ts {
		target := ttu.ComputedUserset.Relation
		if t.userRelation != "" || t.userID == "*" || ev.m.relation(t.userType, target) == nil {
			continue
		}
		ok, err := ev.followConditional(t, func() (bool, error) {
			return ev.check(object{typ: t.userType, id: t.userID}, target, sub)
		})
		if ok {
			return true, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return false, firstErr
}

// CheckRequest asks whether a user has a relation with an object.
type CheckRequest struct {
	TupleKey             CheckTupleKey
	ContextualTuples     []TupleKey
	Context              map[string]any
	AuthorizationModelID string
	Consistency          Consistency
}

// Check reports whether the user has the relation with the object.
func (e *Engine) Check(ctx context.Context, storeID string, req *CheckRequest) (allowed bool, err error) {
	ctx, o := e.startOp(ctx, "check", storeID)
	defer func() { o.end(err, attribute.Bool("gebra.allowed", allowed)) }()
	allowed, outcome, err := e.fastCheck(ctx, o, storeID, req)
	if err != nil || outcome == fastAnswered {
		return allowed, err
	}
	ev, err := e.newEvaluator(ctx, storeID, req.AuthorizationModelID, req.Consistency, req.ContextualTuples, req.Context, false)
	if err != nil {
		return false, err
	}
	if outcome == fastRelevant {
		ev.useGraph()
	}
	q, err := parseCheck(ev.m, req.TupleKey)
	if err != nil {
		return false, err
	}
	o.target(q.obj.typ, q.rel)
	allowed, compiled, err := ev.checkOnce(q)
	if !compiled {
		o.setPath("graph")
	}
	return allowed, err
}

// checkOnce answers one check. A check that compiles to a single statement
// on the view runs without a snapshot, and the statement checks that the view
// still serves the evaluator's model. Other checks run in a snapshot.
//
// It also reports whether the view answered the check with one statement.
func (ev *evaluator) checkOnce(q *checkQuery) (allowed, compiled bool, err error) {
	if c := ev.compiler(q.sub); c != nil && !ev.behind && c.relation(q.obj, q.rel) {
		allowed, current, err := ev.runCompiledAlone(c)
		if err != nil || current {
			return allowed, true, err
		}
	}
	if err := ev.begin(); err != nil {
		return false, false, err
	}
	defer ev.end()
	if ev.behind {
		scope := &pendingScope{sub: &q.sub, sets: newScopeSets()}
		ev.m.addScope(scope.sets, q.obj, q.rel, map[string]bool{})
		if err := ev.settle(scope, false, false); err != nil {
			return false, false, err
		}
	}
	c := ev.compiler(q.sub)
	compiled = c != nil && c.relation(q.obj, q.rel)
	allowed, err = ev.check(q.obj, q.rel, q.sub)
	return allowed, compiled, err
}

// BatchCheckItem is one check of a batch.
type BatchCheckItem struct {
	TupleKey         CheckTupleKey
	ContextualTuples []TupleKey
	Context          map[string]any
	CorrelationID    string
}

// BatchCheckResult is the outcome of one check of a batch. Err is set when
// the check could not be decided.
type BatchCheckResult struct {
	Allowed bool
	Err     error
}

// BatchCheck runs any number of checks with one model and returns their
// results by correlation ID. Each check reads one point in time, as a Check
// would, and checks of one batch may read different ones.
func (e *Engine) BatchCheck(ctx context.Context, storeID, modelID string, c Consistency, items []BatchCheckItem) (_ map[string]BatchCheckResult, err error) {
	ctx, o := e.startOp(ctx, "batch_check", storeID)
	defer func() { o.end(err, attribute.Int("gebra.batch.size", len(items))) }()
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if it.CorrelationID == "" {
			return nil, invalidf("every check needs a correlation ID")
		}
		if _, ok := seen[it.CorrelationID]; ok {
			return nil, invalidf("duplicate correlation ID %q", it.CorrelationID)
		}
		seen[it.CorrelationID] = struct{}{}
	}
	base, err := e.newEvaluator(ctx, storeID, modelID, c, nil, nil, false)
	if err != nil {
		return nil, err
	}

	results := make(map[string]BatchCheckResult, len(items))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, e.opts.BatchCheckConcurrency)
	for _, it := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			allowed, err := base.batchItem(it)
			mu.Lock()
			results[it.CorrelationID] = BatchCheckResult{Allowed: allowed, Err: err}
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results, nil
}

func (base *evaluator) batchItem(it BatchCheckItem) (bool, error) {
	allowed, outcome, err := base.e.fastCheck(base.ctx, nil, base.storeID, &CheckRequest{
		TupleKey: it.TupleKey, ContextualTuples: it.ContextualTuples, Context: it.Context,
		AuthorizationModelID: base.m.id, Consistency: base.consistency,
	})
	if err != nil || outcome == fastAnswered {
		return allowed, err
	}
	ev := base.fork()
	if outcome == fastRelevant {
		ev.useGraph()
	}
	ev.requestCtx = it.Context
	if err := ev.setContextual(it.ContextualTuples); err != nil {
		return false, err
	}
	q, err := parseCheck(ev.m, it.TupleKey)
	if err != nil {
		return false, err
	}
	allowed, _, err = ev.checkOnce(q)
	return allowed, err
}

// ExpandNode is a node of a userset tree, in the shape OpenFGA returns.
type ExpandNode struct {
	Name         string            `json:"name"`
	Leaf         *ExpandLeaf       `json:"leaf,omitempty"`
	Union        *ExpandNodes      `json:"union,omitempty"`
	Intersection *ExpandNodes      `json:"intersection,omitempty"`
	Difference   *ExpandDifference `json:"difference,omitempty"`
}

type ExpandNodes struct {
	Nodes []*ExpandNode `json:"nodes"`
}

type ExpandDifference struct {
	Base     *ExpandNode `json:"base"`
	Subtract *ExpandNode `json:"subtract"`
}

type ExpandLeaf struct {
	Users          *ExpandUsers          `json:"users,omitempty"`
	Computed       *ExpandComputed       `json:"computed,omitempty"`
	TupleToUserset *ExpandTupleToUserset `json:"tupleToUserset,omitempty"`
}

type ExpandUsers struct {
	Users []string `json:"users"`
}

type ExpandComputed struct {
	Userset string `json:"userset"`
}

type ExpandTupleToUserset struct {
	Tupleset string           `json:"tupleset"`
	Computed []ExpandComputed `json:"computed"`
}

// ExpandRequest asks for the userset tree of a relation of an object.
type ExpandRequest struct {
	Object               string
	Relation             string
	ContextualTuples     []TupleKey
	AuthorizationModelID string
	Consistency          Consistency
}

// Expand returns one level of the userset tree of the relation: the stored
// users, and the usersets that the rewrite refers to. It reads the stored
// tuples that a check with the same consistency would see.
func (e *Engine) Expand(ctx context.Context, storeID string, req *ExpandRequest) (_ *ExpandNode, err error) {
	ctx, o := e.startOp(ctx, "expand", storeID)
	defer func() { o.end(err) }()
	ev, err := e.newEvaluator(ctx, storeID, req.AuthorizationModelID, req.Consistency, req.ContextualTuples, nil, true)
	if err != nil {
		return nil, err
	}
	defer ev.end()
	if err := ev.settle(nil, false, false); err != nil {
		return nil, err
	}
	// Expand reads stored tuples, never the view.
	o.setPath("graph")
	obj, err := parseObject(req.Object)
	if err != nil {
		return nil, err
	}
	r := ev.m.relation(obj.typ, req.Relation)
	if r == nil {
		return nil, invalidf("relation %s#%s is not defined", obj.typ, req.Relation)
	}
	return ev.expand(obj, req.Relation, r.rewrite)
}

func (ev *evaluator) expand(obj object, rel string, u *Userset) (*ExpandNode, error) {
	node := &ExpandNode{Name: setKey(obj.typ, obj.id, rel)}
	switch {
	case u.This != nil:
		ts, err := ev.readTuples(obj.typ, obj.id, rel)
		if err != nil {
			return nil, err
		}
		users := make([]string, 0, len(ts))
		for _, t := range ts {
			users = append(users, t.userString())
		}
		slices.Sort(users)
		node.Leaf = &ExpandLeaf{Users: &ExpandUsers{Users: slices.Compact(users)}}

	case u.ComputedUserset != nil:
		node.Leaf = &ExpandLeaf{Computed: &ExpandComputed{Userset: setKey(obj.typ, obj.id, u.ComputedUserset.Relation)}}

	case u.TupleToUserset != nil:
		ttu := u.TupleToUserset
		ts, err := ev.readTuples(obj.typ, obj.id, ttu.Tupleset.Relation)
		if err != nil {
			return nil, err
		}
		computed := []ExpandComputed{}
		for _, t := range ts {
			if t.userRelation == "" && ev.m.relation(t.userType, ttu.ComputedUserset.Relation) != nil {
				computed = append(computed, ExpandComputed{Userset: setKey(t.userType, t.userID, ttu.ComputedUserset.Relation)})
			}
		}
		node.Leaf = &ExpandLeaf{TupleToUserset: &ExpandTupleToUserset{
			Tupleset: setKey(obj.typ, obj.id, ttu.Tupleset.Relation),
			Computed: computed,
		}}

	case u.Union != nil, u.Intersection != nil:
		children := u.Union
		if children == nil {
			children = u.Intersection
		}
		nodes := &ExpandNodes{}
		for _, c := range children.Child {
			n, err := ev.expand(obj, rel, c)
			if err != nil {
				return nil, err
			}
			nodes.Nodes = append(nodes.Nodes, n)
		}
		if u.Union != nil {
			node.Union = nodes
		} else {
			node.Intersection = nodes
		}

	case u.Difference != nil:
		base, err := ev.expand(obj, rel, u.Difference.Base)
		if err != nil {
			return nil, err
		}
		subtract, err := ev.expand(obj, rel, u.Difference.Subtract)
		if err != nil {
			return nil, err
		}
		node.Difference = &ExpandDifference{Base: base, Subtract: subtract}
	}
	return node, nil
}
