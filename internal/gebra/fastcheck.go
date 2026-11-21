package gebra

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgx/v5"
)

// A check of a plain subject whose rewrite compiles to a condition on the view
// runs as one statement that also verifies the serving generation the engine
// assumed and that the store exists, so that it needs no other statement.
//
// With a consistency that asks for writes the view may lack, the statement
// also tells whether a pending change can affect the answer, in which case
// the check evaluates the graph. Otherwise the view answers as the latest
// state would, which includes every write the check asks for. The argument is
// the one in pending.go, which keeps a superset of the changes it calls
// relevant: a change counts when the set its tuple touches, or the parent of
// the tuple to userset edge it adds, is contained in a leaf of the condition
// in the applied view, and it is a userset tuple, a tuple to userset tuple,
// or a member row of the subject or its type's wildcard. Changes of the
// tupleset sets whose member rows the condition reads count whatever they
// are.
//
// A set is contained in a leaf when it is the leaf, has a reach row to it, or
// is contained through computed usersets in such a set; sets in no edge and
// with no member have no reach rows, and computed usersets are the only way
// they are contained in other sets. gebra_model_effect lists, for each
// relation, the tuple relations on the same object whose sets are contained in
// it that way, and the tupleset relations whose edges end in such a set, so
// that the statement finds the relevant pending changes through the index of
// gebra_pending from the leaves and the sets that reach rows put in them.

// modelEffectRow says that pending tuples on an object of SetType with
// relation TupleRel can change who is in the object's SetRel set: as members
// or userset edges of a set contained in it (effectOwn), or as tupleset tuples
// whose edges end in such a set (effectTTU).
type modelEffectRow struct {
	ModelID  string
	SetType  string
	SetRel   string
	TupleRel string
	Kind     string
}

const (
	effectOwn = "own"
	effectTTU = "ttu"
)

// effectRows returns the model's rows of gebra_model_effect.
func (m *compiledModel) effectRows() []modelEffectRow {
	var rows []modelEffectRow
	for _, t := range m.types {
		// containedIn[r] lists the relations whose sets computed usersets
		// put in the object's set of r, r included.
		containedIn := map[string][]string{}
		for x := range t.relations {
			for _, k := range m.computedClosure(setKey(t.name, "_", x)) {
				_, _, r := splitSetKey(k)
				containedIn[r] = append(containedIn[r], x)
			}
		}
		for r := range t.relations {
			for _, x := range containedIn[r] {
				rows = append(rows, modelEffectRow{ModelID: m.id, SetType: t.name, SetRel: r, TupleRel: x, Kind: effectOwn})
			}
			for ts, edges := range m.ttuEdges[t.name] {
				for _, e := range edges {
					if containsString(containedIn[r], e.parent) {
						rows = append(rows, modelEffectRow{ModelID: m.id, SetType: t.name, SetRel: r, TupleRel: ts, Kind: effectTTU})
						break
					}
				}
			}
		}
	}
	return rows
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// ensureEffects stores the model's rows of gebra_model_effect unless this
// engine did already. Rows go in sorted order in one transaction, so that
// engines storing the same model wait for each other instead of deadlocking.
func (e *Engine) ensureEffects(ctx context.Context, m *compiledModel) error {
	if _, ok := e.effects.Load(m.id); ok {
		return nil
	}
	e.effectsMu.Lock()
	defer e.effectsMu.Unlock()
	if _, ok := e.effects.Load(m.id); ok {
		return nil
	}
	rows := m.effectRows()
	slices.SortFunc(rows, func(a, b modelEffectRow) int {
		return strings.Compare(a.SetType+"#"+a.SetRel+"#"+a.TupleRel+"#"+a.Kind, b.SetType+"#"+b.SetRel+"#"+b.TupleRel+"#"+b.Kind)
	})
	if len(rows) > 0 {
		err := e.db.handle().tx(ctx, nil, func(tx dbtx) error {
			var n int64
			if _, err := tx.queryRow(ctx, "SELECT COUNT(*) FROM gebra_model_effect WHERE model_id = ?", []any{m.id}, &n); err != nil || n == int64(len(rows)) {
				return err
			}
			values := make([][]any, len(rows))
			for i, r := range rows {
				values[i] = []any{r.ModelID, r.SetType, r.SetRel, r.TupleRel, r.Kind}
			}
			_, err := tx.insertRows(ctx, "gebra_model_effect", []string{"model_id", "set_type", "set_rel", "tuple_rel", "kind"}, values, onConflict{ignore: true})
			return err
		})
		if err != nil {
			return errors.Wrap(err, "store model effects")
		}
	}
	e.effects.Store(m.id, true)
	return nil
}

// subjectRows returns a condition on a pending row p, joined with a row f of
// gebra_model_effect, that holds unless the change cannot affect whether the
// compiled condition's plain subject U is in the set it touches. Member rows
// of other subjects cannot. A userset edge c → s cannot when U does not reach
// c in the applied view plus every pending row: U is in c or a set that
// reaches c in the applied view, or a pending change touches c, a set on
// c's object, or a set that reaches c. Tupleset tuples always count.
func (c *checkCompiler) subjectRows() (string, []any) {
	var pairs [][2]string
	for _, t := range c.m.types {
		for name, r := range t.relations {
			if r.materialized {
				pairs = append(pairs, [2]string{t.name, name})
			}
		}
	}
	// Sorted, so that the statement's text, which prepared statements are
	// cached by, is the same for every check.
	slices.SortFunc(pairs, func(a, b [2]string) int { return strings.Compare(a[0]+"#"+a[1], b[0]+"#"+b[1]) })
	var mat []string
	var args []any
	for _, pr := range pairs {
		mat = append(mat, "(p.user_type = ? AND p.user_relation = ?)")
		args = append(args, pr[0], pr[1])
	}
	notMaterialized := "1 = 1"
	if len(mat) > 0 {
		notMaterialized = "NOT (" + strings.Join(mat, " OR ") + ")"
	}
	query := `((f.kind = 'ttu' AND p.user_relation = '')
	OR (f.kind = 'own' AND p.user_relation = '' AND p.user_type = ? AND p.user_id IN (?, '*'))
	OR (f.kind = 'own' AND p.user_relation <> '' AND (` + notMaterialized + `
		OR EXISTS (SELECT 1 FROM gebra_view_member m
			WHERE m.store_id = p.store_id AND m.gen = ? AND m.user_type = ? AND m.user_id IN (?, '*')
			AND m.set_type = p.user_type AND m.set_rel = p.user_relation AND m.set_id = p.user_id)
		OR EXISTS (SELECT 1 FROM gebra_view_reach r
			JOIN gebra_view_member m ON m.store_id = r.store_id AND m.gen = r.gen AND m.set_key = r.desc_key
			WHERE r.store_id = p.store_id AND r.gen = ? AND r.anc_type = p.user_type AND r.anc_id = p.user_id AND r.anc_rel = p.user_relation
			AND m.user_type = ? AND m.user_id IN (?, '*'))
		OR EXISTS (SELECT 1 FROM gebra_pending q
			WHERE q.store_id = p.store_id AND q.object_type = p.user_type AND q.object_id = p.user_id)
		OR EXISTS (SELECT 1 FROM gebra_view_reach r
			WHERE r.store_id = p.store_id AND r.gen = ? AND r.anc_type = p.user_type AND r.anc_id = p.user_id AND r.anc_rel = p.user_relation
			AND (SELECT 1 FROM gebra_pending q
				WHERE q.store_id = r.store_id AND q.object_type = r.desc_type AND q.object_id = r.desc_id AND q.relation = r.desc_rel LIMIT 1) IS NOT NULL))))`
	all := []any{c.sub.typ, c.sub.id}
	all = append(all, args...)
	all = append(all, c.gen, c.sub.typ, c.sub.id, c.gen, c.sub.typ, c.sub.id, c.gen)
	return query, all
}

// relevance returns a condition that holds when a pending change of the store
// can change the answer of the compiled condition, whose subject is plain.
//
// Parts that follow reach rows probe gebra_pending for each set with a
// subquery that has a LIMIT, which the planner does not merge into a join, so
// that they cost the same however many changes are pending. A plan that starts
// from the pending changes, which the planner prefers when it last saw few of
// them, reads every pending change of the store on each check.
func (c *checkCompiler) relevance() (string, []any) {
	subjectRows, subjectArgs := c.subjectRows()
	var parts []string
	var args []any
	for i, obj := range c.leaves {
		rel := c.leafRels[i]
		parts = append(parts, `EXISTS (SELECT 1 FROM gebra_model_effect f
JOIN gebra_pending p ON p.store_id = ? AND p.object_type = f.set_type AND p.object_id = ? AND p.relation = f.tuple_rel
WHERE f.model_id = ? AND f.set_type = ? AND f.set_rel = ? AND `+subjectRows+`)`,
			`EXISTS (SELECT 1 FROM gebra_view_reach r
JOIN gebra_model_effect f ON f.model_id = ? AND f.set_type = r.desc_type AND f.set_rel = r.desc_rel
WHERE r.store_id = ? AND r.gen = ? AND r.anc_type = ? AND r.anc_id = ? AND r.anc_rel = ?
AND (SELECT 1 FROM gebra_pending p
	WHERE p.store_id = r.store_id AND p.object_type = r.desc_type AND p.object_id = r.desc_id AND p.relation = f.tuple_rel
	AND `+subjectRows+` LIMIT 1) IS NOT NULL)`)
		args = append(args, c.storeID, obj.id, c.m.id, obj.typ, rel)
		args = append(args, subjectArgs...)
		args = append(args, c.m.id, c.storeID, c.gen, obj.typ, obj.id, rel)
		args = append(args, subjectArgs...)
	}
	seen := map[string]bool{}
	for _, ts := range c.tsReads {
		parents := setKey(ts.obj.typ, ts.obj.id, ts.rel)
		if !seen[parents] {
			seen[parents] = true
			parts = append(parts, `EXISTS (SELECT 1 FROM gebra_pending p
WHERE p.store_id = ? AND p.object_type = ? AND p.object_id = ? AND p.relation = ?)`)
			args = append(args, c.storeID, ts.obj.typ, ts.obj.id, ts.rel)
		}
		parts = append(parts, `EXISTS (SELECT 1 FROM gebra_view_member ts
JOIN gebra_model_effect f ON f.model_id = ? AND f.set_type = ts.user_type AND f.set_rel = ?
JOIN gebra_pending p ON p.store_id = ts.store_id AND p.object_type = ts.user_type AND p.object_id = ts.user_id AND p.relation = f.tuple_rel
WHERE ts.store_id = ? AND ts.gen = ? AND ts.set_key = ? AND ts.user_type = ? AND `+subjectRows+`)`,
			`EXISTS (SELECT 1 FROM gebra_view_member ts
JOIN gebra_view_reach r ON r.store_id = ts.store_id AND r.gen = ts.gen AND r.anc_type = ts.user_type AND r.anc_id = ts.user_id AND r.anc_rel = ?
JOIN gebra_model_effect f ON f.model_id = ? AND f.set_type = r.desc_type AND f.set_rel = r.desc_rel
WHERE ts.store_id = ? AND ts.gen = ? AND ts.set_key = ? AND ts.user_type = ?
AND (SELECT 1 FROM gebra_pending p
	WHERE p.store_id = r.store_id AND p.object_type = r.desc_type AND p.object_id = r.desc_id AND p.relation = f.tuple_rel
	AND `+subjectRows+` LIMIT 1) IS NOT NULL)`)
		args = append(args, c.m.id, ts.target, c.storeID, c.gen, parents, ts.typ)
		args = append(args, subjectArgs...)
		args = append(args, ts.target, c.m.id, c.storeID, c.gen, parents, ts.typ)
		args = append(args, subjectArgs...)
	}
	if len(parts) == 0 {
		return "1 = 0", nil
	}
	return "(" + strings.Join(parts, "\nOR ") + ")", args
}

// liveView is a store's serving view generation and its model as this engine
// last read them.
type liveView struct {
	gen     int64
	modelID string
}

// fastOutcome tells how fastCheck handled a check.
type fastOutcome int

const (
	// fastMiss leaves the check to the evaluator.
	fastMiss fastOutcome = iota
	// fastAnswered answered the check from the view.
	fastAnswered
	// fastRelevant found pending changes that can affect the answer, so the
	// check evaluates the graph.
	fastRelevant
)

// fastCheck answers a check with one statement when it can.
func (e *Engine) fastCheck(ctx context.Context, o *op, storeID string, req *CheckRequest) (bool, fastOutcome, error) {
	if len(req.ContextualTuples) > 0 {
		return false, fastMiss, nil
	}
	revision, err := minRevision(req.Consistency)
	if err != nil {
		return false, fastMiss, err
	}
	lv, ok, err := e.liveView(ctx, storeID)
	if err != nil || !ok || req.AuthorizationModelID != "" && req.AuthorizationModelID != lv.modelID {
		return false, fastMiss, err
	}
	m, err := e.model(ctx, e.db.handle(), storeID, lv.modelID)
	if err != nil {
		return false, fastMiss, nil
	}
	q, err := parseCheck(m, req.TupleKey)
	if err != nil {
		// The engine's live model may be out of date, so the evaluator
		// decides whether the check is valid.
		e.live.Delete(storeID)
		return false, fastMiss, nil
	}
	if q.sub.rel != "" {
		return false, fastMiss, nil
	}
	token := revision > 0 || req.Consistency.Preference == ConsistencyHigher
	if !token && req.Consistency.Preference != ConsistencyMinimizeLatency {
		stale, err := e.viewStale(ctx, e.db.handle(), storeID)
		if err != nil || stale {
			return false, fastMiss, err
		}
	}
	c := newCheckCompiler(m, string(e.db.dialect), storeID, lv.gen, q.sub, true)
	if !c.relation(q.obj, q.rel) {
		return false, fastMiss, nil
	}
	query := "SELECT CASE WHEN " + c.sql.String() + ` THEN 1 ELSE 0 END,
	COALESCE((SELECT gen FROM gebra_view_state WHERE store_id = ?), 0),
	(SELECT COUNT(*) FROM gebra_store WHERE id = ? AND deleted_at IS NULL)`
	args := append(c.args, storeID, storeID)
	var allowed, gen, alive, pending, relevant int64
	dest := []any{&allowed, &gen, &alive}
	if token {
		if err := e.ensureEffects(ctx, m); err != nil {
			return false, fastMiss, err
		}
		cond, condArgs := c.relevance()
		query += `,
	CASE WHEN EXISTS (SELECT 1 FROM gebra_pending WHERE store_id = ?) THEN 1 ELSE 0 END,
	CASE WHEN EXISTS (SELECT 1 FROM gebra_pending WHERE store_id = ?) AND ` + cond + ` THEN 1 ELSE 0 END`
		args = append(args, storeID, storeID)
		args = append(args, condArgs...)
		dest = append(dest, &pending, &relevant)
	}
	if err := e.queryRow(ctx, query, args, dest...); err != nil {
		return false, fastMiss, errors.Wrap(err, "check view")
	}
	if alive == 0 || gen != lv.gen {
		e.live.Delete(storeID)
		return false, fastMiss, nil
	}
	if o != nil {
		o.target(q.obj.typ, q.rel)
	}
	if relevant == 1 {
		return false, fastRelevant, nil
	}
	if o != nil {
		o.setPath("view")
	}
	if pending == 1 {
		e.recordFallback(ctx, "view_irrelevant_pending")
	}
	return allowed == 1, fastAnswered, nil
}

// liveView returns the store's serving view as this engine last saw it, and
// false when the store has no serving view yet.
func (e *Engine) liveView(ctx context.Context, storeID string) (liveView, bool, error) {
	if v, ok := e.live.Load(storeID); ok {
		return v.(liveView), true, nil
	}
	sv, err := storeModels(ctx, e.db.handle(), storeID)
	if errors.Is(err, ErrNotFound) {
		return liveView{}, false, nil
	}
	if err != nil || sv.Gen == 0 {
		return liveView{}, false, err
	}
	lv := liveView{gen: sv.Gen, modelID: sv.LiveModelID}
	e.live.Store(storeID, lv)
	return lv, true, nil
}

// queryRow runs a query that returns one row on a connection that conn
// returns.
func (e *Engine) queryRow(ctx context.Context, query string, args []any, dest ...any) error {
	conn, err := e.conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	found, err := dbtx{database: e.db, q: conn}.queryRow(ctx, query, args, dest...)
	if err == nil && !found {
		return errors.New("query returned no row")
	}
	return err
}

// conn returns a connection of the pool. On Postgres, it makes the
// connection use generic plans for prepared statements. Custom plans
// of the compiled checks cost several milliseconds to plan, more than they
// take to run, and the planner keeps choosing them for checks with a
// consistency token, whose generic plans it estimates higher.
func (e *Engine) conn(ctx context.Context) (*sql.Conn, error) {
	conn, err := e.db.pool.Conn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get connection")
	}
	if e.db.dialect != DialectPostgres {
		return conn, nil
	}
	err = conn.Raw(func(dc any) error {
		pc, ok := dc.(interface{ Conn() *pgx.Conn })
		if !ok {
			return nil
		}
		data := pc.Conn().PgConn().CustomData()
		if data[genericPlansKey] != nil {
			return nil
		}
		if _, err := pc.Conn().Exec(ctx, "SET plan_cache_mode = force_generic_plan"); err != nil {
			return errors.Wrap(err, "set plan cache mode")
		}
		data[genericPlansKey] = true
		return nil
	})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	return conn, nil
}

const genericPlansKey = "gebra.genericPlans"
