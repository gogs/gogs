package gebra

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Consistency preferences, as in OpenFGA.
const (
	ConsistencyUnspecified     = "UNSPECIFIED"
	ConsistencyMinimizeLatency = "MINIMIZE_LATENCY"
	ConsistencyHigher          = "HIGHER_CONSISTENCY"
)

// Consistency says how current the materialized view must be for a query.
// The store's applier updates the view shortly after writes commit. A check
// or Expand whose view is not current enough evaluates the graph, which reads
// the latest tuples, and a list query first waits up to Options.ViewWait for
// the applier. Either way the answer is never older than asked for.
//
// With the default consistency, a query uses the view unless the store's
// oldest pending write is older than Options.MaxStaleness. A query that uses
// the view reads every tuple as of the writes that the view includes, so its
// answer is that of one committed state.
type Consistency struct {
	// Preference is empty or ConsistencyUnspecified for the default,
	// ConsistencyMinimizeLatency to use the view however stale it is, or
	// ConsistencyHigher to see every write committed before the query.
	Preference string
	// MinRevision is a revision that Write returned. The query sees that
	// write and every write of the store that committed before it.
	MinRevision string
}

// A revision is the inbox ID of a write. It is satisfied once no write of the
// store with an ID up to it is pending, which holds whatever order the writes
// committed in: a write with a lower ID that commits later is pending until
// the applier reaches it, and a write that has not committed yet cannot be
// one that the query must see.

func encodeRevision(inboxID int64) string {
	if inboxID == 0 {
		return ""
	}
	return encodeToken(strconv.FormatInt(inboxID, 10))
}

// minRevision validates c and returns the inbox ID of its minimum revision,
// or zero.
func minRevision(c Consistency) (int64, error) {
	switch c.Preference {
	case "", ConsistencyUnspecified, ConsistencyMinimizeLatency, ConsistencyHigher:
	default:
		return 0, invalidf("invalid consistency %q, want %q or %q", c.Preference, ConsistencyMinimizeLatency, ConsistencyHigher)
	}
	if c.MinRevision == "" {
		return 0, nil
	}
	need, err := decodeIntToken(c.MinRevision)
	if err != nil {
		return 0, invalidf("invalid min revision %q", c.MinRevision)
	}
	return need, nil
}

// requiredWrite returns the inbox ID up to which a query with c needs every
// committed write of the store applied, or zero when any view will do.
func requiredWrite(ctx context.Context, db dbtx, storeID string, c Consistency, revision int64) (int64, error) {
	if c.Preference != ConsistencyHigher {
		return revision, nil
	}
	// Writes that commit from here on may take lower IDs, which makes the
	// query wait for them too, but never for longer than they take to apply.
	var latest int64
	_, err := db.queryRow(ctx, "SELECT COALESCE(MAX(id), 0) FROM gebra_inbox WHERE store_id = ?", []any{storeID}, &latest)
	if err != nil {
		return 0, errors.Wrap(err, "read latest pending write")
	}
	return max(revision, latest), nil
}

// pendingThrough returns the lowest inbox ID up to id of a committed write of
// the store that is not applied yet, or zero when there is none.
func pendingThrough(ctx context.Context, db dbtx, storeID string, id int64) (int64, error) {
	ids, err := queryColumn[int64](ctx, db, "SELECT id FROM gebra_inbox WHERE store_id = ? AND id <= ? ORDER BY id LIMIT 1", storeID, id)
	if err != nil || len(ids) == 0 {
		return 0, errors.Wrap(err, "read pending writes")
	}
	return ids[0], nil
}

// oldestPending is the time of a store's oldest pending write as an engine
// last read it.
type oldestPending struct {
	read time.Time
	// oldest is zero when no write was pending.
	oldest time.Time
}

// oldestPendingTTL is how long an engine reuses the time of a store's oldest
// pending write, so that queries do not read it every time.
const oldestPendingTTL = 100 * time.Millisecond

// viewStale reports whether the store's oldest pending write is older than
// Options.MaxStaleness.
func (e *Engine) viewStale(ctx context.Context, db dbtx, storeID string) (bool, error) {
	now := time.Now()
	e.oldestMu.Lock()
	c := e.oldest[storeID]
	refresh := c == nil || now.Sub(c.read) > oldestPendingTTL
	if refresh && c != nil {
		// Queries that find the time expired while this one reads it again
		// keep using the previous one, rather than all reading it at once.
		e.oldest[storeID] = &oldestPending{read: now, oldest: c.oldest}
	}
	e.oldestMu.Unlock()
	if refresh {
		rows, err := queryStructs[inboxRow](ctx, db, "SELECT created_at FROM gebra_inbox WHERE store_id = ? ORDER BY id LIMIT 1", storeID)
		if err != nil {
			return false, errors.Wrap(err, "read oldest pending write")
		}
		c = &oldestPending{read: now}
		if len(rows) > 0 {
			c.oldest = rows[0].CreatedAt
		}
		e.oldestMu.Lock()
		e.oldest[storeID] = c
		e.oldestMu.Unlock()
	}
	return !c.oldest.IsZero() && now.Sub(c.oldest) > e.opts.MaxStaleness, nil
}

// forgetOldestPending drops the store's cached oldest pending write when it
// made the view stale, so that queries use the view again as soon as this
// engine's applier caught up. A cache that says the view is fresh stays,
// which spares the queries of a busy store a read after every batch.
func (e *Engine) forgetOldestPending(storeID string) {
	e.oldestMu.Lock()
	defer e.oldestMu.Unlock()
	if c := e.oldest[storeID]; c != nil && !c.oldest.IsZero() && time.Since(c.oldest) > e.opts.MaxStaleness {
		delete(e.oldest, storeID)
	}
}

// appliedSignal returns a channel that is closed when an applier batch of
// this engine commits.
func (e *Engine) appliedSignal() <-chan struct{} {
	e.appliedMu.Lock()
	defer e.appliedMu.Unlock()
	return e.applied
}

func (e *Engine) signalApplied() {
	e.appliedMu.Lock()
	defer e.appliedMu.Unlock()
	close(e.applied)
	e.applied = make(chan struct{})
}

// waitUntilApplied waits until no write of the store with an inbox ID up to
// id is pending, or until the deadline. It returns the lowest pending ID, or
// zero once there is none.
func (e *Engine) waitUntilApplied(ctx context.Context, db dbtx, storeID string, id int64, deadline time.Time) (int64, error) {
	kicked := false
	for delay := time.Millisecond; ; delay = min(2*delay, 10*time.Millisecond) {
		// Taken before reading, so that a batch that commits after the read
		// wakes the wait up.
		applied := e.appliedSignal()
		pending, err := pendingThrough(ctx, db, storeID, id)
		if err != nil || pending == 0 {
			return 0, err
		}
		if !kicked {
			// This engine may become the store's applier, e.g., when the write
			// came from another process.
			e.kick(storeID)
			kicked = true
		}
		if time.Now().Add(delay).After(deadline) {
			return pending, nil
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return pending, ctx.Err()
		case <-applied:
			timer.Stop()
		case <-timer.C:
		}
	}
}

// waitForView reports whether the store's view includes every write with an
// inbox ID up to need, waiting up to Options.ViewWait for the applier.
func (e *Engine) waitForView(ctx context.Context, db dbtx, storeID string, need int64) (bool, error) {
	pending, err := e.waitUntilApplied(ctx, db, storeID, need, time.Now().Add(e.opts.ViewWait))
	return pending == 0, err
}

// waitApplied waits until the write with the inbox ID, and every earlier
// write of the store that committed, is applied, up to Options.WaitTimeout.
func (e *Engine) waitApplied(ctx context.Context, storeID string, id int64) error {
	db := e.db.handle()
	first, err := pendingThrough(ctx, db, storeID, id)
	if err != nil || first == 0 {
		return err
	}
	pending, err := e.waitUntilApplied(ctx, db, storeID, id, time.Now().Add(e.opts.WaitTimeout))
	if err != nil || pending == 0 {
		return err
	}
	if pending == first {
		return errors.Wrapf(ErrNotApplied, "no applier made progress on store %q for %s", storeID, e.opts.WaitTimeout)
	}
	return errors.Wrapf(ErrNotApplied, "the applier of store %q is behind and did not reach the write within %s", storeID, e.opts.WaitTimeout)
}

// applierRun is a running applier goroutine. dirty says that a write
// committed while it ran, so it must look for writes again before it stops.
type applierRun struct {
	dirty bool
}

// applierBackoff delays the next applier batch of a store whose last batch
// failed.
type applierBackoff struct {
	failures int
	next     time.Time
}

const (
	// minApplyBackoff and maxApplyBackoff bound the delay after a failed
	// applier batch, which doubles with every failure in a row.
	minApplyBackoff = 100 * time.Millisecond
	maxApplyBackoff = 30 * time.Second
)

// errApplierRace means that another applier applied writes of the store,
// which happens while two engines both act as its applier.
var errApplierRace = errors.New("writes applied by another applier")

// kick makes sure the store's applier brings its view up to date, when this
// engine is or can become the store's applier.
func (e *Engine) kick(storeID string) {
	if e.opts.manualApply {
		return
	}
	e.applyMu.Lock()
	defer e.applyMu.Unlock()
	if e.closed {
		return
	}
	if run, ok := e.applying[storeID]; ok {
		run.dirty = true
		return
	}
	if b, ok := e.backoff[storeID]; ok && time.Now().Before(b.next) {
		// The sweep starts the applier once the backoff ends.
		return
	}
	run := &applierRun{}
	e.applying[storeID] = run
	e.appliers.Add(1)
	go func() {
		defer e.appliers.Done()
		for {
			leader := e.lead(e.background, storeID)
			var more bool
			var err error
			if leader {
				more, err = e.applyBatch(e.background, storeID)
				if err != nil && e.background.Err() == nil {
					e.applyFailed(storeID, err)
				}
			}
			e.applyMu.Lock()
			if leader && err == nil {
				delete(e.backoff, storeID)
				if more || run.dirty {
					run.dirty = false
					e.applyMu.Unlock()
					continue
				}
			}
			delete(e.applying, storeID)
			e.applyMu.Unlock()
			return
		}
	}()
}

// applyFailed logs a failed applier batch and, unless it lost a race with
// another applier, delays the store's next batch.
func (e *Engine) applyFailed(storeID string, err error) {
	ctx := e.background
	race := writeRace(err) != "" || errors.Is(err, errApplierRace)
	if race {
		e.tel.applyFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("gebra.applier.failure", "race")))
		e.logger.WarnContext(ctx, "Applier batch lost a race with another applier", "storeID", storeID, "error", err)
		return
	}
	e.applyMu.Lock()
	b := e.backoff[storeID]
	if b == nil {
		b = &applierBackoff{}
		e.backoff[storeID] = b
	}
	b.failures++
	delay := maxApplyBackoff
	if b.failures < 20 {
		delay = min(minApplyBackoff<<(b.failures-1), maxApplyBackoff)
	}
	b.next = time.Now().Add(delay)
	failures := b.failures
	e.applyMu.Unlock()
	e.tel.applyFailures.Add(ctx, 1, metric.WithAttributes(attribute.String("gebra.applier.failure", "error")))
	e.logger.ErrorContext(ctx, "Failed to apply writes to view", "storeID", storeID,
		"failures", failures, "retryIn", delay, "error", err)
}

// sweep periodically starts appliers for stores with pending writes, which
// covers writes from other processes, appliers that failed, and stores whose
// applier in another process is gone.
func (e *Engine) sweep() {
	defer e.appliers.Done()
	ticker := time.NewTicker(e.opts.ApplyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-e.background.Done():
			return
		case <-ticker.C:
		}
		e.checkLeadership(e.background)
		// Stores with a view build to run or to start count as pending too,
		// so that a new applier resumes a build after a failover.
		ids, err := queryColumn[string](e.background, e.db.handle(), `SELECT s.id FROM gebra_store s
			WHERE s.deleted_at IS NULL AND (s.id IN (SELECT store_id FROM gebra_inbox)
				OR EXISTS (SELECT 1 FROM gebra_view_state v WHERE v.store_id = s.id AND (v.build_gen <> 0 OR v.model_id <> s.model_id)))`)
		if err != nil {
			if e.background.Err() == nil {
				e.logger.ErrorContext(e.background, "Failed to find stores with pending writes", "error", err)
			}
			continue
		}
		for _, id := range ids {
			e.kick(id)
		}
	}
}

const (
	// applyBatchSize is how many tuple changes the applier applies in one
	// transaction, unless a single write holds more.
	applyBatchSize = 1000
	// applyBatchWrites bounds how many pending writes a batch looks at.
	applyBatchWrites = 1000
	// applyAttempts is how often a batch runs again after a serialization
	// failure, which only happens while two engines both act as applier.
	applyAttempts = 3
)

// applyBatch applies the store's next pending writes to its view, runs the
// next chunk of its view build if one runs, and reports whether more is left.
func (e *Engine) applyBatch(ctx context.Context, storeID string) (more bool, err error) {
	for attempt := 1; ; attempt++ {
		more, err = e.applyBatchOnce(ctx, storeID)
		if err == nil || attempt == applyAttempts || writeRace(err) != "serialization" {
			return more, err
		}
	}
}

// batchResult is what an applier batch applied.
type batchResult struct {
	writes, changes int
	// view and build maintained the serving and the building generation.
	view, build *viewMaintainer
	// flipped is true when the batch's build chunk made its generation the
	// serving one.
	flipped bool
	created []time.Time
}

// applyBatchOnce runs one applier transaction, and then one chunk of the
// store's view build when one runs. The transaction cancels or starts the
// build as the store's latest model asks, applies the pending writes in inbox
// ID order to the serving generation and to the part of the building one that
// the build covered, records their changes in the change log, deletes them
// from the inbox, and updates the view state, so each write is applied
// exactly once.
func (e *Engine) applyBatchOnce(ctx context.Context, storeID string) (more bool, err error) {
	// Applier batches run in the background, so each starts a trace of its own.
	ctx, span := e.tel.tracer.Start(ctx, "gebra.view.apply", trace.WithNewRoot(),
		trace.WithAttributes(attribute.String("gebra.store.id", storeID)))
	start := time.Now()
	var b batchResult
	defer func() {
		if err == nil && b.writes > 0 {
			now := time.Now()
			e.tel.applyChanges.Add(ctx, int64(b.changes))
			e.tel.applyBatchWrites.Record(ctx, int64(b.writes))
			for _, c := range b.created {
				e.tel.applyLag.Record(ctx, max(0, now.Sub(c).Seconds()))
			}
			e.tel.recordViewChange(ctx, b.view, "applier")
			e.tel.recordViewChange(ctx, b.build, "build")
		}
		var attrs []attribute.KeyValue
		(&op{span: span}).finish(err, &attrs, []attribute.KeyValue{
			attribute.Int("gebra.view.changes", b.changes),
			attribute.Int("gebra.applier.writes", b.writes),
			attribute.Bool("gebra.view.flip", b.flipped),
		})
		e.tel.applyDuration.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attrs...))
	}()

	// A batch only reads the inbox and the view, which no one else writes,
	// so it runs under read committed.
	var txOpts *sql.TxOptions
	if e.db.dialect != DialectSQLite {
		txOpts = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	var build viewStateRow
	var liveRows int64
	err = e.writeTx(ctx, func(tx dbtx) error {
		b, more, build = batchResult{}, false, viewStateRow{}
		// Only appliers lock the view state row. It keeps a former applier
		// that has not noticed its lost leadership from applying the same
		// writes again.
		vs, latest, err := lockViewState(ctx, tx, storeID)
		if err != nil || vs == nil {
			return err
		}
		planned, err := e.planBuild(ctx, tx, vs, latest)
		if err != nil {
			return err
		}
		if more, err = e.applyIncremental(ctx, tx, vs, &b); err != nil {
			return err
		}
		if b.writes > 0 || planned {
			if err := saveViewState(ctx, tx, vs); err != nil {
				return err
			}
		}
		build, liveRows = *vs, vs.MemberRows+vs.EdgeRows+vs.ReachRows
		if hook := e.opts.hooks.beforeApplyCommit; hook != nil && b.writes > 0 {
			return hook(ctx, storeID)
		}
		return nil
	}, txOpts)
	if err != nil {
		return false, err
	}
	if b.writes > 0 {
		e.forgetOldestPending(storeID)
		e.signalApplied()
		changed := int64(b.changes)
		for _, v := range []*viewMaintainer{b.view, b.build} {
			if v != nil {
				changed += v.added.member + v.removed.member + v.added.edge + v.removed.edge + v.added.reach + v.removed.reach
			}
		}
		e.maybeAnalyze(changed, liveRows, false)
	}
	if build.BuildGen != 0 {
		running, err := e.buildStep(ctx, storeID, build.BuildGen, build.BuildPhase, build.BuildTotal, &b)
		if err != nil {
			return false, err
		}
		more = more || running
	}
	return more, nil
}

// lockViewState locks the store's view state row and returns it with the
// store's latest model, or nil when the store is deleted.
func lockViewState(ctx context.Context, tx dbtx, storeID string) (*viewStateRow, string, error) {
	q := `SELECT v.*, s.model_id AS latest_model_id
		FROM gebra_view_state v
		JOIN gebra_store s ON s.id = v.store_id AND s.deleted_at IS NULL
		WHERE v.store_id = ?`
	if tx.dialect != DialectSQLite {
		q += " FOR UPDATE OF v"
	}
	rows, err := queryStructs[viewStateRow](ctx, tx, q, storeID)
	if err != nil {
		return nil, "", errors.Wrap(err, "lock view state")
	}
	if len(rows) == 0 {
		var n int64
		if _, err := tx.queryRow(ctx, "SELECT COUNT(*) FROM gebra_store WHERE id = ? AND deleted_at IS NULL", []any{storeID}, &n); err != nil {
			return nil, "", errors.Wrap(err, "get store")
		}
		if n > 0 {
			return nil, "", errors.Newf("store %q has no view state", storeID)
		}
		return nil, "", nil
	}
	return &rows[0], rows[0].LatestModelID, nil
}

// applyIncremental applies the oldest pending writes, up to applyBatchSize
// tuple changes, and reports whether more are left. Model writes change no
// tuples, and the applier only consumes them.
func (e *Engine) applyIncremental(ctx context.Context, tx dbtx, vs *viewStateRow, b *batchResult) (more bool, _ error) {
	// The payloads of writes past the batch's budget stay unread. A write
	// always fits when it comes first, however many changes it holds.
	heads, err := queryStructs[inboxRow](ctx, tx, `SELECT h.id, h.store_id, h.kind, h.tuples, h.created_at,
			CASE WHEN SUM(h.tuples) OVER (ORDER BY h.id ROWS UNBOUNDED PRECEDING) <= ? OR ROW_NUMBER() OVER (ORDER BY h.id) = 1
				THEN h.changes ELSE '' END AS changes
		FROM (SELECT * FROM gebra_inbox WHERE store_id = ? ORDER BY id LIMIT ?) h
		ORDER BY h.id`, applyBatchSize, vs.StoreID, applyBatchWrites)
	if err != nil {
		return false, errors.Wrap(err, "read pending writes")
	}
	more = len(heads) == applyBatchWrites
	var rows []inboxRow
	changes := 0
	for _, h := range heads {
		if len(rows) > 0 && changes+h.Tuples > applyBatchSize {
			more = true
			break
		}
		rows = append(rows, h)
		changes += h.Tuples
	}
	if len(rows) == 0 {
		return more, nil
	}

	log, err := inboxChanges(rows)
	if err != nil {
		return false, err
	}
	removed, added, err := netChanges(log)
	if err != nil {
		return false, err
	}
	if len(removed)+len(added) > 0 && vs.Gen != 0 {
		m, err := e.model(ctx, tx, vs.StoreID, vs.ModelID)
		if err != nil {
			return false, err
		}
		view := newViewMaintainer(ctx, tx, vs.StoreID, vs.Gen, m)
		view.record(removed, added)
		if err := view.apply(); err != nil {
			return false, err
		}
		b.view = view
		net := view.net()
		vs.MemberRows += net.member
		vs.EdgeRows += net.edge
		vs.ReachRows += net.reach
	}
	if len(removed)+len(added) > 0 && vs.BuildGen != 0 {
		if b.build, err = e.applyToBuild(ctx, tx, vs, removed, added); err != nil {
			return false, err
		}
	}
	if err := consumeWrites(ctx, tx, vs.StoreID, rows, log, b); err != nil {
		return false, err
	}
	for _, r := range rows {
		vs.Applied = max(vs.Applied, r.ID)
	}
	return more, nil
}

// consumeWrites records the writes' tuple changes, which inboxChanges
// returned, in the change log and deletes the writes and their pending
// changes from the inbox.
func consumeWrites(ctx context.Context, tx dbtx, storeID string, rows []inboxRow, log []changeRow, b *batchResult) error {
	if len(log) > 0 {
		values := make([][]any, len(log))
		for i, c := range log {
			values[i] = []any{c.StoreID, c.Operation, c.ObjectType, c.ObjectID, c.Relation, c.UserType, c.UserID, c.UserRelation, c.ConditionName, c.ConditionContext, c.CreatedAt}
		}
		if _, err := tx.insertRows(ctx, "gebra_change", changeColumns, values, onConflict{}); err != nil {
			return errors.Wrap(err, "record changes")
		}
	}
	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
		b.created = append(b.created, r.CreatedAt)
	}
	for _, batch := range chunks(ids) {
		n, err := tx.exec(ctx, "DELETE FROM gebra_inbox WHERE store_id = ? AND id IN ?", storeID, batch)
		if err != nil {
			return errors.Wrap(err, "delete applied writes")
		}
		if n != int64(len(batch)) {
			return errors.Wrapf(errApplierRace, "%d of %d writes of store %q", int64(len(batch))-n, len(batch), storeID)
		}
		if len(log) > 0 {
			if _, err := tx.exec(ctx, "DELETE FROM gebra_pending WHERE inbox_id IN ?", batch); err != nil {
				return errors.Wrap(err, "delete pending changes")
			}
		}
	}
	b.writes += len(rows)
	b.changes += len(log)
	return nil
}

// saveViewState writes vs to the store's view state row.
func saveViewState(ctx context.Context, tx dbtx, vs *viewStateRow) error {
	vs.UpdatedAt = time.Now().UTC()
	_, err := tx.exec(ctx, `UPDATE gebra_view_state SET applied = ?, gen = ?, model_id = ?, member_rows = ?, edge_rows = ?, reach_rows = ?, last_gen = ?,
	build_gen = ?, build_model_id = ?, build_phase = ?, build_object_type = ?, build_object_id = ?, build_relation = ?,
	build_user_type = ?, build_user_id = ?, build_user_relation = ?, build_set_key = ?, build_done = ?, build_total = ?, build_chunks = ?,
	build_member_rows = ?, build_edge_rows = ?, build_reach_rows = ?, build_started_at = ?, updated_at = ?
WHERE store_id = ?`,
		vs.Applied, vs.Gen, vs.ModelID, vs.MemberRows, vs.EdgeRows, vs.ReachRows, vs.LastGen,
		vs.BuildGen, vs.BuildModelID, vs.BuildPhase, vs.BuildObjectType, vs.BuildObjectID, vs.BuildRelation,
		vs.BuildUserType, vs.BuildUserID, vs.BuildUserRelation, vs.BuildSetKey, vs.BuildDone, vs.BuildTotal, vs.BuildChunks,
		vs.BuildMemberRows, vs.BuildEdgeRows, vs.BuildReachRows, vs.BuildStartedAt, vs.UpdatedAt, vs.StoreID)
	return errors.Wrap(err, "update view state")
}

// resetViewState records that the store's view is empty and has no model.
func resetViewState(ctx context.Context, tx dbtx, storeID string) error {
	_, err := tx.exec(ctx, "INSERT INTO gebra_view_state (store_id, applied, updated_at) VALUES (?, 0, ?)", storeID, time.Now().UTC())
	return errors.Wrap(err, "create view state")
}

// inboxChange is a tuple change as an inbox row holds it.
type inboxChange struct {
	Operation        string `json:"op"`
	ObjectType       string `json:"ot"`
	ObjectID         string `json:"oi"`
	Relation         string `json:"r"`
	UserType         string `json:"ut"`
	UserID           string `json:"ui"`
	UserRelation     string `json:"ur,omitempty"`
	ConditionName    string `json:"cn,omitempty"`
	ConditionContext string `json:"cc,omitempty"`
}

// inboxChanges returns the tuple changes of the inbox rows as change log
// rows, in order.
func inboxChanges(rows []inboxRow) ([]changeRow, error) {
	var out []changeRow
	for _, r := range rows {
		if r.Kind != inboxTuples {
			continue
		}
		var cs []inboxChange
		if err := json.Unmarshal([]byte(r.Changes), &cs); err != nil {
			return nil, errors.Wrapf(err, "decode changes of write %d", r.ID)
		}
		for _, c := range cs {
			out = append(out, changeRow{
				StoreID: r.StoreID, Operation: c.Operation,
				ObjectType: c.ObjectType, ObjectID: c.ObjectID, Relation: c.Relation,
				UserType: c.UserType, UserID: c.UserID, UserRelation: c.UserRelation,
				ConditionName: c.ConditionName, ConditionContext: c.ConditionContext, CreatedAt: r.CreatedAt,
			})
		}
	}
	return out, nil
}

// netChanges returns the tuples that a run of changes removes and adds,
// comparing the state before the first change of each tuple with the state
// after its last change. A tuple deleted and written again with another
// condition is both removed and added.
func netChanges(changes []changeRow) (removed, added []*tuple, _ error) {
	type span struct{ first, last *changeRow }
	spans := map[tupleIdent]*span{}
	var order []tupleIdent
	for i := range changes {
		c := &changes[i]
		id := tupleIdent{c.ObjectType, c.ObjectID, c.Relation, c.UserType, c.UserID, c.UserRelation}
		if s, ok := spans[id]; ok {
			s.last = c
			continue
		}
		spans[id] = &span{first: c, last: c}
		order = append(order, id)
	}
	for _, id := range order {
		s := spans[id]
		// Writes only add missing tuples and deletes only remove present ones,
		// and the changes of one tuple are in commit order, so the first
		// change says whether the tuple existed before.
		if s.first.Operation == OperationDelete {
			t, err := s.first.tuple()
			if err != nil {
				return nil, nil, err
			}
			removed = append(removed, t)
		}
		if s.last.Operation == OperationWrite {
			t, err := s.last.tuple()
			if err != nil {
				return nil, nil, err
			}
			added = append(added, t)
		}
	}
	return removed, added, nil
}

func (r *changeRow) tuple() (*tuple, error) {
	return (&tupleRow{
		ObjectType: r.ObjectType, ObjectID: r.ObjectID, Relation: r.Relation,
		UserType: r.UserType, UserID: r.UserID, UserRelation: r.UserRelation,
		ConditionName: r.ConditionName, ConditionContext: r.ConditionContext,
	}).tuple()
}

// storeView is the latest model of a store, and the generation and model of
// its serving view.
type storeView struct {
	LatestModelID string
	// LiveModelID is the model of the serving generation, which is zero
	// until the first build of the store completed.
	LiveModelID string
	Gen         int64
}

func storeModels(ctx context.Context, db dbtx, storeID string) (storeView, error) {
	rows, err := queryStructs[storeView](ctx, db, `SELECT s.model_id AS latest_model_id, COALESCE(v.model_id, '') AS live_model_id, COALESCE(v.gen, 0) AS gen
		FROM gebra_store s LEFT JOIN gebra_view_state v ON v.store_id = s.id
		WHERE s.id = ? AND s.deleted_at IS NULL`, storeID)
	if err != nil {
		return storeView{}, errors.Wrap(err, "get store")
	}
	if len(rows) == 0 {
		return storeView{}, errors.Wrapf(ErrNotFound, "store %q", storeID)
	}
	return rows[0], nil
}

// ViewStatus reports how far a store's materialized view lags behind its
// writes, and the progress of a view build for a new model.
type ViewStatus struct {
	// AppliedRevision is the revision of the last write the applier applied.
	// Writes with lower revisions can still be pending when they committed
	// late.
	AppliedRevision string `json:"applied_revision"`
	// LatestRevision is the highest revision of the store's committed writes.
	LatestRevision string `json:"latest_revision"`
	// PendingWrites is how many committed writes, including model writes,
	// the view does not include.
	PendingWrites int64 `json:"pending_writes"`
	// PendingChanges is how many tuple changes those writes hold.
	PendingChanges int64 `json:"pending_changes"`
	// AuthorizationModelID is the live model: the model of the serving view,
	// which queries use when they name no model. It is empty until the
	// store's first view build completed, and queries evaluate the graph
	// with the latest model until then.
	AuthorizationModelID string `json:"authorization_model_id"`
	// FreshAsOf is a time before which the view includes every write: the
	// time of the oldest pending write, or now when nothing is pending.
	FreshAsOf time.Time `json:"fresh_as_of"`
	// Migration is set while the live model is not the store's latest one.
	Migration *ViewMigration `json:"migration,omitempty"`
}

// ViewMigration is the progress of the view build that makes the store's
// latest model live.
type ViewMigration struct {
	// LiveModelID is the model that queries use until the build completes,
	// or empty for a store's first model.
	LiveModelID string `json:"live_model"`
	// TargetModelID is the latest model, which the build is for.
	TargetModelID string `json:"target_model"`
	// Phase is "pending" until the applier starts the build,
	// "building_members" while it adds the member and edge rows of every
	// tuple, and "building_reach" while it adds the reach rows of every set.
	// The applier applies writes to the build all along, so no phase catches
	// up on writes.
	Phase string `json:"phase"`
	// DoneChunks is how many chunks of the current phase the build ran, and
	// TotalChunks an estimate of how many the phase takes, so that
	// DoneChunks/TotalChunks is the phase's progress.
	DoneChunks  int64 `json:"done_chunks"`
	TotalChunks int64 `json:"total_chunks"`
	// DoneItems is how many tuples or sets the phase covered, of an
	// estimated TotalItems.
	DoneItems  int64      `json:"done_items"`
	TotalItems int64      `json:"total_items"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
}

// ViewStatus returns how far the store's view lags behind its writes.
func (e *Engine) ViewStatus(ctx context.Context, storeID string) (*ViewStatus, error) {
	db := e.db.handle()
	st, err := e.storeRow(ctx, db, storeID)
	if err != nil {
		return nil, err
	}
	states, err := queryStructs[viewStateRow](ctx, db, "SELECT * FROM gebra_view_state WHERE store_id = ?", storeID)
	if err != nil {
		return nil, errors.Wrap(err, "read view state")
	}
	if len(states) == 0 {
		return nil, errors.Newf("store %q has no view state", storeID)
	}
	vs := states[0]
	var pending struct {
		Writes, Changes, Oldest, Latest int64
	}
	_, err = db.queryRow(ctx, `SELECT COUNT(*), COALESCE(SUM(tuples), 0), COALESCE(MIN(id), 0), COALESCE(MAX(id), 0)
		FROM gebra_inbox WHERE store_id = ?`, []any{storeID}, &pending.Writes, &pending.Changes, &pending.Oldest, &pending.Latest)
	if err != nil {
		return nil, errors.Wrap(err, "count pending writes")
	}
	status := &ViewStatus{
		AppliedRevision:      encodeRevision(vs.Applied),
		LatestRevision:       encodeRevision(max(vs.Applied, pending.Latest)),
		PendingWrites:        pending.Writes,
		PendingChanges:       pending.Changes,
		AuthorizationModelID: vs.ModelID,
		FreshAsOf:            time.Now().UTC(),
		Migration:            e.migration(&vs, st.ModelID),
	}
	if pending.Oldest == 0 {
		return status, nil
	}
	oldest, err := queryStructs[inboxRow](ctx, db, "SELECT created_at FROM gebra_inbox WHERE id = ?", pending.Oldest)
	if err != nil {
		return nil, errors.Wrap(err, "read oldest pending write")
	}
	if len(oldest) > 0 {
		status.FreshAsOf = oldest[0].CreatedAt.UTC()
	}
	return status, nil
}

// migration returns the progress of the build that makes the latest model
// live, or nil when it is live.
func (e *Engine) migration(vs *viewStateRow, latest string) *ViewMigration {
	if latest == "" || vs.Gen != 0 && vs.ModelID == latest {
		return nil
	}
	m := &ViewMigration{LiveModelID: vs.ModelID, TargetModelID: latest, Phase: "pending"}
	if vs.BuildGen == 0 || vs.BuildModelID != latest {
		return m
	}
	m.Phase = "building_" + vs.BuildPhase
	m.DoneChunks, m.DoneItems = vs.BuildChunks, vs.BuildDone
	m.TotalItems = max(vs.BuildTotal, vs.BuildDone)
	size := int64(e.opts.buildChunkSize)
	m.TotalChunks = vs.BuildChunks + (m.TotalItems-vs.BuildDone+size-1)/size
	if vs.BuildStartedAt != nil {
		started := vs.BuildStartedAt.UTC()
		m.StartedAt = &started
	}
	return m
}
