package gebra

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// A model write makes the store's applier build a new generation of the view
// next to the serving one, in chunks that each run in a transaction of their
// own:
//
//  1. buildMembers walks the applied tuples in the order of their key and adds
//     the member and edge rows of each. Its cursor is the last tuple it
//     covered.
//  2. buildReach walks the sets that have members or are the child of an
//     edge, in the order of their key, and adds the reach rows of each. Its
//     cursor is the last set it covered.
//
// Writes keep committing meanwhile, and the applier batches that apply them
// change the building generation too, but only the part that the build has
// covered: the member and edge rows of tuples up to the cursor in the first
// phase, and every member and edge row plus the reach rows of sets up to the
// cursor in the second. A chunk reads what lies past its cursor from the
// applied state, which includes every batch before it, so the covered part
// always equals a full build at the applied state, and the generation is
// exact once the second cursor passed the last set. The transaction of the
// last chunk makes it the serving generation.
//
// The applied state of a store does not change while its applier holds the
// view state row, even as writes commit, because a write adds pending rows for
// the keys it changes in the transaction that changes them. So the statements
// of a chunk all read the same applied tuples without a snapshot.
//
// Cursor comparisons run in SQL against the cursor columns of the view state
// row, whose types match the tuple and set key columns, so that they order
// keys by the database's collation as the chunks' scans do.

const (
	buildMembers = "members"
	buildReach   = "reach"
	// defaultBuildChunkSize is how many tuples or sets a build chunk covers.
	// The applier runs a chunk after every batch of writes, so a chunk of
	// about the cost of a full batch keeps the delay it adds to writes
	// within a batch's time.
	defaultBuildChunkSize = applyBatchSize
	// buildReachRowsPerSet bounds the reach rows that a chunk inserts to this
	// many per set of the chunk size. A chunk stops after the set that
	// reaches the bound.
	buildReachRowsPerSet = 10
	// viewGCGrace is how long the rows of a former serving generation stay
	// after a flip, for queries that resolved the generation before it.
	viewGCGrace = time.Minute
)

// buildChunkInfo describes a build chunk to telemetry and test hooks.
type buildChunkInfo struct {
	gen     int64
	modelID string
	phase   string
	// items is how many tuples or sets the chunk covered.
	items     int
	reachRows int
	flip      bool
}

// planBuild makes the store's view build follow the latest model: it cancels
// a build for any other model, and starts one when the serving generation has
// another model and no build runs. It reports whether it changed vs.
func (e *Engine) planBuild(ctx context.Context, tx dbtx, vs *viewStateRow, latest string) (bool, error) {
	need := latest != "" && (vs.Gen == 0 || vs.ModelID != latest)
	now := time.Now().UTC()
	changed := false
	if vs.BuildGen != 0 && (!need || vs.BuildModelID != latest) {
		// No query reads a building generation, so its rows can go at once.
		if err := retireGen(ctx, tx, vs.StoreID, vs.BuildGen, now); err != nil {
			return false, err
		}
		e.logger.InfoContext(ctx, "Cancelled view build", "storeID", vs.StoreID, "gen", vs.BuildGen,
			"modelID", vs.BuildModelID, "latestModelID", latest)
		clearBuild(vs)
		changed = true
	}
	if need && vs.BuildGen == 0 {
		vs.LastGen++
		vs.BuildGen, vs.BuildModelID, vs.BuildPhase = vs.LastGen, latest, buildMembers
		vs.BuildStartedAt = &now
		e.logger.InfoContext(ctx, "Started view build", "storeID", vs.StoreID, "gen", vs.BuildGen,
			"modelID", latest, "liveModelID", vs.ModelID)
		changed = true
	}
	return changed, nil
}

func clearBuild(vs *viewStateRow) {
	vs.BuildGen, vs.BuildModelID, vs.BuildPhase = 0, "", ""
	vs.BuildObjectType, vs.BuildObjectID, vs.BuildRelation = "", "", ""
	vs.BuildUserType, vs.BuildUserID, vs.BuildUserRelation = "", "", ""
	vs.BuildSetKey = ""
	vs.BuildDone, vs.BuildTotal, vs.BuildChunks = 0, 0, 0
	vs.BuildMemberRows, vs.BuildEdgeRows, vs.BuildReachRows = 0, 0, 0
	vs.BuildStartedAt = nil
}

// retireGen records that maintenance deletes the rows of the store's view
// generation once deleteAfter has passed.
func retireGen(ctx context.Context, tx dbtx, storeID string, gen int64, deleteAfter time.Time) error {
	_, err := tx.insertRows(ctx, "gebra_view_gc", []string{"store_id", "gen", "delete_after"}, [][]any{{storeID, gen, deleteAfter}}, onConflict{ignore: true})
	return errors.Wrap(err, "retire view generation")
}

// buildStep runs the next chunk of the store's view build of generation gen,
// which the applier found in phase with an estimated total, and reports
// whether a build still runs afterwards.
func (e *Engine) buildStep(ctx context.Context, storeID string, gen int64, phase string, total int64, b *batchResult) (bool, error) {
	if hook := e.opts.hooks.beforeBuildChunk; hook != nil {
		hook(ctx, storeID, gen)
	}
	if total == 0 {
		var err error
		if total, err = e.estimateBuild(ctx, storeID, gen, phase); err != nil {
			return false, err
		}
	}
	var txOpts *sql.TxOptions
	if e.db.dialect != DialectSQLite {
		txOpts = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	var info buildChunkInfo
	var started time.Time
	var liveRows int64
	running := false
	err := e.writeTx(ctx, func(tx dbtx) error {
		info, running = buildChunkInfo{gen: gen}, false
		vs, latest, err := lockViewState(ctx, tx, storeID)
		if err != nil || vs == nil {
			return err
		}
		// The applier chose the chunk before this transaction, and a newer
		// model may have cancelled the build since. Checking under the lock
		// of the transaction that writes the rows fences a cancelled build
		// off for good.
		if vs.BuildGen != gen || vs.BuildModelID != latest {
			running = vs.BuildGen != 0 || latest != vs.ModelID
			return nil
		}
		if vs.BuildTotal == 0 && vs.BuildPhase == phase {
			vs.BuildTotal = total
		}
		m, err := e.model(ctx, tx, storeID, vs.BuildModelID)
		if err != nil {
			return err
		}
		info.modelID, info.phase = vs.BuildModelID, vs.BuildPhase
		switch vs.BuildPhase {
		case buildMembers:
			err = e.buildMembers(ctx, tx, vs, m, &info)
		case buildReach:
			err = e.buildReach(ctx, tx, vs, m, &info)
		default:
			err = errors.Newf("store %q builds its view in unknown phase %q", storeID, vs.BuildPhase)
		}
		if err != nil {
			return err
		}
		if info.flip {
			if vs.BuildStartedAt != nil {
				started = *vs.BuildStartedAt
			}
			if err := flip(ctx, tx, vs, time.Now().UTC()); err != nil {
				return err
			}
		}
		if err := saveViewState(ctx, tx, vs); err != nil {
			return err
		}
		liveRows = vs.MemberRows + vs.EdgeRows + vs.ReachRows
		if hook := e.opts.hooks.beforeBuildCommit; hook != nil {
			if err := hook(ctx, info); err != nil {
				return err
			}
		}
		running = !info.flip
		return nil
	}, txOpts)
	if err != nil {
		return false, errors.Wrap(err, "build view")
	}
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String("gebra.view.build.phase", info.phase),
		attribute.Int("gebra.view.build.items", info.items),
		attribute.Int("gebra.view.build.reach_rows", info.reachRows),
	)
	if info.flip {
		b.flipped = true
		e.tel.buildDuration.Record(ctx, time.Since(started).Seconds())
		e.logger.InfoContext(ctx, "View build is live", "storeID", storeID, "gen", gen, "duration", time.Since(started))
		e.signalApplied()
	}
	e.maybeAnalyze(int64(info.items+info.reachRows), liveRows, info.flip)
	return running, nil
}

// estimateBuild counts what the phase of the build of generation gen walks:
// the store's tuples, or the sets that have members or are the child of an
// edge. It runs outside of the build's transactions, so the count is an
// estimate that progress reports use.
func (e *Engine) estimateBuild(ctx context.Context, storeID string, gen int64, phase string) (int64, error) {
	db := e.db.handle()
	var n int64
	var err error
	if phase == buildMembers {
		_, err = db.queryRow(ctx, "SELECT COUNT(*) FROM gebra_tuple WHERE store_id = ?", []any{storeID}, &n)
	} else {
		_, err = db.queryRow(ctx, `SELECT COUNT(*) FROM (
			SELECT set_key FROM gebra_view_member WHERE store_id = ? AND gen = ?
			UNION SELECT child_key FROM gebra_view_edge WHERE store_id = ? AND gen = ?) s`,
			[]any{storeID, gen, storeID, gen}, &n)
	}
	return n, errors.Wrap(err, "estimate view build")
}

// tupleKeyColumns are the columns of the unique key of a tuple, in the order
// of its index.
var tupleKeyColumns = []string{"object_type", "object_id", "relation", "user_type", "user_id", "user_relation"}

// buildCursorColumns are the view state columns of the build's tuple cursor,
// in the order of tupleKeyColumns.
const buildCursorColumns = "build_object_type, build_object_id, build_relation, build_user_type, build_user_id, build_user_relation"

func (vs *viewStateRow) tupleCursor() []any {
	return []any{vs.BuildObjectType, vs.BuildObjectID, vs.BuildRelation, vs.BuildUserType, vs.BuildUserID, vs.BuildUserRelation}
}

// keyAfterCursor returns a condition that the tuple key of the table alias
// comes after the build's tuple cursor, and its arguments.
func (e *Engine) keyAfterCursor(alias string, vs *viewStateRow) (string, []any) {
	cursor := vs.tupleCursor()
	cols := make([]string, len(tupleKeyColumns))
	for i, c := range tupleKeyColumns {
		cols[i] = alias + "." + c
	}
	if e.db.dialect != DialectMySQL {
		return "(" + strings.Join(cols, ", ") + ") > (?, ?, ?, ?, ?, ?)", cursor
	}
	// MySQL does not scan an index range for a row comparison by order, so
	// it gets the expanded form, whose first part bounds the range.
	args := []any{cursor[0]}
	var expand func(i int) string
	expand = func(i int) string {
		if i == len(cols)-1 {
			args = append(args, cursor[i])
			return cols[i] + " > ?"
		}
		args = append(args, cursor[i], cursor[i])
		return "(" + cols[i] + " > ? OR " + cols[i] + " = ? AND " + expand(i+1) + ")"
	}
	cond := cols[0] + " >= ? AND " + expand(0)
	return cond, args
}

// buildMembers adds the member and edge rows of the next applied tuples after
// the cursor to the building generation, and moves on to buildReach after the
// last.
func (e *Engine) buildMembers(ctx context.Context, tx dbtx, vs *viewStateRow, m *compiledModel, info *buildChunkInfo) error {
	size := e.opts.buildChunkSize
	keys := strings.Join(tupleKeyColumns, ", ")
	tupleAfter, tupleArgs := e.keyAfterCursor("t", vs)
	pendingAfter, pendingArgs := e.keyAfterCursor("p", vs)
	// The applied tuples are those of gebra_tuple whose keys no pending
	// change touches, and the rows that the earliest pending change of a key
	// deleted, as in tupleQuery.
	q := `SELECT ` + tupleColumns + ` FROM (
	SELECT ` + tupleColumns + ` FROM gebra_tuple t WHERE t.store_id = ? AND ` + tupleAfter + ` AND NOT EXISTS (
		SELECT 1 FROM gebra_pending p WHERE p.store_id = t.store_id AND p.object_type = t.object_type AND p.object_id = t.object_id
			AND p.relation = t.relation AND p.user_type = t.user_type AND p.user_id = t.user_id AND p.user_relation = t.user_relation)
	ORDER BY ` + keys + ` LIMIT ?) a
UNION ALL
SELECT ` + tupleColumns + ` FROM (
	SELECT ` + tupleColumns + ` FROM gebra_pending p WHERE p.store_id = ? AND ` + pendingAfter + ` AND p.deleted = TRUE AND NOT EXISTS (
		SELECT 1 FROM gebra_pending q WHERE q.store_id = p.store_id AND q.object_type = p.object_type AND q.object_id = p.object_id
			AND q.relation = p.relation AND q.user_type = p.user_type AND q.user_id = p.user_id AND q.user_relation = p.user_relation
			AND q.inbox_id < p.inbox_id)
	ORDER BY ` + keys + ` LIMIT ?) b
ORDER BY ` + keys + ` LIMIT ?`
	args := append([]any{vs.StoreID}, tupleArgs...)
	args = append(args, size, vs.StoreID)
	args = append(args, pendingArgs...)
	args = append(args, size, size)
	rows, err := queryStructs[tupleRow](ctx, tx, q, args...)
	if err != nil {
		return errors.Wrap(err, "read tuples to build")
	}
	ts := make([]*tuple, len(rows))
	for i, r := range rows {
		t, err := r.tuple()
		if err != nil {
			return err
		}
		ts[i] = t
	}
	v := newViewMaintainer(ctx, tx, vs.StoreID, vs.BuildGen, m)
	v.noReach, v.strict = true, true
	v.record(nil, ts)
	if err := v.apply(); err != nil {
		return err
	}
	net := v.net()
	vs.BuildMemberRows += net.member
	vs.BuildEdgeRows += net.edge
	vs.BuildDone += int64(len(rows))
	vs.BuildChunks++
	info.items = len(rows)
	if n := len(rows); n > 0 {
		last := rows[n-1]
		vs.BuildObjectType, vs.BuildObjectID, vs.BuildRelation = last.ObjectType, last.ObjectID, last.Relation
		vs.BuildUserType, vs.BuildUserID, vs.BuildUserRelation = last.UserType, last.UserID, last.UserRelation
	}
	if len(rows) < size {
		vs.BuildPhase, vs.BuildSetKey = buildReach, ""
		vs.BuildDone, vs.BuildTotal, vs.BuildChunks = 0, 0, 0
	}
	return nil
}

// buildReach adds the reach rows of the next sets after the cursor that have
// members or are the child of an edge to the building generation, and reports
// a flip after the last.
func (e *Engine) buildReach(ctx context.Context, tx dbtx, vs *viewStateRow, m *compiledModel, info *buildChunkInfo) error {
	size, gen := e.opts.buildChunkSize, vs.BuildGen
	keys, err := queryColumn[string](ctx, tx, `SELECT k FROM (
	SELECT set_key AS k FROM gebra_view_member WHERE store_id = ? AND gen = ? AND set_key > ? GROUP BY set_key ORDER BY set_key LIMIT ?) m
UNION
SELECT k FROM (
	SELECT child_key AS k FROM gebra_view_edge WHERE store_id = ? AND gen = ? AND child_key > ? GROUP BY child_key ORDER BY child_key LIMIT ?) c
ORDER BY k LIMIT ?`, vs.StoreID, gen, vs.BuildSetKey, size, vs.StoreID, gen, vs.BuildSetKey, size, size)
	if err != nil {
		return errors.Wrap(err, "read sets to build")
	}
	v := newViewMaintainer(ctx, tx, vs.StoreID, gen, m)
	parents, err := v.walkUp(keys)
	if err != nil {
		return err
	}
	explicitParents := func(children []string) ([]string, error) {
		var out []string
		for _, c := range children {
			out = append(out, parents[c]...)
		}
		return out, nil
	}
	budget := size * buildReachRowsPerSet
	var rows []viewReachRow
	covered := 0
	for _, d := range keys {
		ups, _ := bfsUp(m, d, explicitParents)
		for a := range ups {
			rows = append(rows, reachRow(vs.StoreID, gen, a, d))
		}
		covered++
		vs.BuildSetKey = d
		if len(rows) >= budget {
			break
		}
	}
	if len(rows) > 0 {
		// The sets after the cursor have no reach rows yet, so a conflict
		// means that the build covered a set twice.
		if _, err := insertReach(ctx, tx, rows, onConflict{}); err != nil {
			return errors.Wrap(err, "insert view reach")
		}
	}
	vs.BuildReachRows += int64(len(rows))
	vs.BuildDone += int64(covered)
	vs.BuildChunks++
	info.items, info.reachRows = covered, len(rows)
	info.flip = len(keys) < size && covered == len(keys)
	return nil
}

// flip makes the completed build the serving generation, and retires the
// former one after viewGCGrace.
func flip(ctx context.Context, tx dbtx, vs *viewStateRow, now time.Time) error {
	if vs.Gen != 0 {
		if err := retireGen(ctx, tx, vs.StoreID, vs.Gen, now.Add(viewGCGrace)); err != nil {
			return err
		}
	}
	vs.Gen, vs.ModelID = vs.BuildGen, vs.BuildModelID
	vs.MemberRows, vs.EdgeRows, vs.ReachRows = vs.BuildMemberRows, vs.BuildEdgeRows, vs.BuildReachRows
	clearBuild(vs)
	return nil
}

// applyToBuild applies the tuples that an applier batch removes and adds to
// the part of the building generation that the build has covered.
func (e *Engine) applyToBuild(ctx context.Context, tx dbtx, vs *viewStateRow, removed, added []*tuple) (*viewMaintainer, error) {
	if vs.BuildPhase == buildMembers && vs.BuildObjectType == "" {
		// The build has not covered a tuple yet, and its cursor in the
		// database can still be the one of a build that this batch
		// cancelled.
		return nil, nil
	}
	m, err := e.model(ctx, tx, vs.StoreID, vs.BuildModelID)
	if err != nil {
		return nil, err
	}
	v := newViewMaintainer(ctx, tx, vs.StoreID, vs.BuildGen, m)
	switch vs.BuildPhase {
	case buildMembers:
		v.noReach = true
		if removed, err = coveredTuples(ctx, tx, vs.StoreID, removed); err != nil {
			return nil, err
		}
		if added, err = coveredTuples(ctx, tx, vs.StoreID, added); err != nil {
			return nil, err
		}
	case buildReach:
		v.covered = func(sets []string) (map[string]bool, error) {
			return coveredSets(ctx, tx, vs.StoreID, vs.BuildSetKey, sets)
		}
	}
	v.record(removed, added)
	if err := v.apply(); err != nil {
		return nil, err
	}
	net := v.net()
	vs.BuildMemberRows += net.member
	vs.BuildEdgeRows += net.edge
	vs.BuildReachRows += net.reach
	return v, nil
}

// coveredTuples returns the tuples whose keys are at most the build's tuple
// cursor in the store's view state row.
func coveredTuples(ctx context.Context, tx dbtx, storeID string, ts []*tuple) ([]*tuple, error) {
	var out []*tuple
	for _, batch := range rowChunks(ts) {
		parts := make([]string, len(batch))
		args := make([]any, 0, 7*len(batch))
		for i, t := range batch {
			parts[i] = "SELECT " + strconv.Itoa(i) + " AS i FROM gebra_view_state WHERE store_id = ? AND (?, ?, ?, ?, ?, ?) <= (" + buildCursorColumns + ")"
			args = append(args, storeID, t.objectType, t.objectID, t.relation, t.userType, t.userID, t.userRelation)
		}
		hits, err := queryColumn[int](ctx, tx, strings.Join(parts, " UNION ALL "), args...)
		if err != nil {
			return nil, errors.Wrap(err, "compare tuples with build cursor")
		}
		for _, i := range hits {
			out = append(out, batch[i])
		}
	}
	return out, nil
}

// coveredSets reports which of the sets are at most the build's set cursor in
// the store's view state row, which is cursor.
func coveredSets(ctx context.Context, tx dbtx, storeID, cursor string, sets []string) (map[string]bool, error) {
	out := make(map[string]bool, len(sets))
	if cursor == "" {
		return out, nil
	}
	for _, batch := range chunks(sets) {
		parts := make([]string, len(batch))
		args := make([]any, 0, 2*len(batch))
		for i, s := range batch {
			parts[i] = "SELECT " + strconv.Itoa(i) + " AS i FROM gebra_view_state WHERE store_id = ? AND ? <= build_set_key"
			args = append(args, storeID, s)
		}
		hits, err := queryColumn[int](ctx, tx, strings.Join(parts, " UNION ALL "), args...)
		if err != nil {
			return nil, errors.Wrap(err, "compare sets with build cursor")
		}
		for _, i := range hits {
			out[batch[i]] = true
		}
	}
	return out, nil
}
