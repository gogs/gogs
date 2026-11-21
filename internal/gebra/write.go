package gebra

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// WriteRequest adds and removes tuples in one transaction.
type WriteRequest struct {
	Writes  []TupleKey
	Deletes []TupleKey
	// OnDuplicate is "error" (the default) or "ignore". With "ignore", writing
	// a tuple that exists with the same condition does nothing.
	OnDuplicate string
	// OnMissing is "error" (the default) or "ignore". With "ignore", deleting
	// a tuple that does not exist does nothing.
	OnMissing            string
	AuthorizationModelID string
	// Wait makes Write return only after the store's applier applied the
	// write, so that queries with any consistency see it. Without Wait, Write
	// returns once the write commits, and queries see it once the applier
	// has caught up, or when they pass its revision as
	// Consistency.MinRevision.
	Wait bool
}

const (
	OperationWrite  = "TUPLE_OPERATION_WRITE"
	OperationDelete = "TUPLE_OPERATION_DELETE"
)

// maxWriteAttempts bounds how often Write starts over after losing a race
// with a concurrent writer of the same tuples.
const maxWriteAttempts = 10

// errWriteRace means a concurrent writer changed a tuple that the write read,
// and the write must start over to see the change.
var errWriteRace = errors.New("tuple changed by a concurrent write")

// Write applies the request's deletes and writes atomically and returns the
// revision of the write, which is empty when the write changed nothing.
//
// A write validates the tuples, changes gebra_tuple, and records itself in
// the store's inbox. It locks nothing but the tuples it changes, so writes of
// different tuples run in parallel. The store's applier updates the view and
// the change log after the write commits. With req.Wait, Write waits for that,
// and fails with ErrNotApplied, next to the revision, when it times out.
func (e *Engine) Write(ctx context.Context, storeID string, req *WriteRequest) (revision string, err error) {
	ctx, o := e.startOp(ctx, "write", storeID)
	var res writeResult
	attempts := 0
	defer func() {
		wait := attribute.Bool("gebra.write.wait", req.Wait)
		attrs := []attribute.KeyValue{wait}
		o.finish(err, &attrs, []attribute.KeyValue{
			wait,
			attribute.Int("gebra.write.attempts", attempts),
			attribute.Int("gebra.write.tuples_written", res.written),
			attribute.Int("gebra.write.tuples_deleted", res.deleted),
		})
		e.tel.writeDuration.Record(ctx, time.Since(o.start).Seconds(), metric.WithAttributes(attrs...))
		if revision != "" {
			e.tel.writeTuples.Add(ctx, int64(res.written), metric.WithAttributes(attribute.String("gebra.write.operation", "write")))
			e.tel.writeTuples.Add(ctx, int64(res.deleted), metric.WithAttributes(attribute.String("gebra.write.operation", "delete")))
		}
	}()
	for _, v := range []string{req.OnDuplicate, req.OnMissing} {
		if v != "" && v != "error" && v != "ignore" {
			return "", invalidf(`invalid write option %q, want "error" or "ignore"`, v)
		}
	}
	if len(req.Writes) == 0 && len(req.Deletes) == 0 {
		return "", invalidf("write needs at least one tuple to write or delete")
	}
	parse := func(keys []TupleKey, seen map[string]struct{}) ([]*tuple, error) {
		out := make([]*tuple, len(keys))
		for i, k := range keys {
			t, err := parseTupleKey(k)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[t.keyString()]; ok {
				return nil, invalidf("tuple %s appears more than once in the request", t.keyString())
			}
			seen[t.keyString()] = struct{}{}
			out[i] = t
		}
		return out, nil
	}
	seen := map[string]struct{}{}
	writes, err := parse(req.Writes, seen)
	if err != nil {
		return "", err
	}
	deletes, err := parse(req.Deletes, seen)
	if err != nil {
		return "", err
	}

	// A retry must see the tuples that the writer it raced with committed,
	// which MySQL's default isolation level hides.
	var txOpts *sql.TxOptions
	if e.db.dialect != DialectSQLite {
		txOpts = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	for attempt := 1; ; attempt++ {
		attempts = attempt
		err := e.writeTx(ctx, func(tx dbtx) (err error) {
			res, err = e.write(ctx, tx, storeID, req, writes, deletes)
			return err
		}, txOpts)
		if err == nil {
			break
		}
		race := writeRace(err)
		if race == "" || attempt == maxWriteAttempts || ctx.Err() != nil {
			res = writeResult{}
			return "", err
		}
		e.tel.writeRetries.Add(ctx, 1, metric.WithAttributes(attribute.String("gebra.write.race", race)))
	}
	if res.revision == 0 {
		return "", nil
	}
	e.kick(storeID)
	revision = encodeRevision(res.revision)
	if req.Wait {
		if err := e.waitApplied(ctx, storeID, res.revision); err != nil {
			return revision, err
		}
	}
	return revision, nil
}

func (e *Engine) write(ctx context.Context, tx dbtx, storeID string, req *WriteRequest, writes, deletes []*tuple) (writeResult, error) {
	st, err := e.storeRow(ctx, tx, storeID)
	if err != nil {
		return writeResult{}, err
	}
	modelID := st.ModelID
	if req.AuthorizationModelID != "" {
		modelID = req.AuthorizationModelID
	}
	m, err := e.model(ctx, tx, storeID, modelID)
	if err != nil {
		return writeResult{}, err
	}

	if hook := e.opts.hooks.beforeTuples; hook != nil {
		hook(ctx, req)
	}
	existing, err := findTuples(ctx, tx, storeID, slices.Concat(deletes, writes))
	if err != nil {
		return writeResult{}, err
	}
	var removedRows, addedRows []*tupleRow
	for _, t := range deletes {
		row := existing[t.ident()]
		if row == nil {
			if req.OnMissing == "ignore" {
				continue
			}
			return writeResult{}, invalidf("cannot delete tuple %s, it does not exist", t.keyString())
		}
		removedRows = append(removedRows, row)
	}
	now := time.Now().UTC()
	for _, t := range writes {
		if err := m.validateTuple(t); err != nil {
			return writeResult{}, err
		}
		cctx, err := t.contextJSON()
		if err != nil {
			return writeResult{}, err
		}
		if row := existing[t.ident()]; row != nil {
			if req.OnDuplicate == "ignore" && row.ConditionName == t.conditionName && sameJSON(row.ConditionContext, cctx) {
				continue
			}
			return writeResult{}, invalidf("cannot write tuple %s, it already exists", t.keyString())
		}
		addedRows = append(addedRows, &tupleRow{
			StoreID: storeID, ObjectType: t.objectType, ObjectID: t.objectID, Relation: t.relation,
			UserType: t.userType, UserID: t.userID, UserRelation: t.userRelation,
			ConditionName: t.conditionName, ConditionContext: cctx, CreatedAt: now,
		})
	}
	if len(removedRows) == 0 && len(addedRows) == 0 {
		return writeResult{}, nil
	}

	// Writers that change the same tuples change them in the same order, so
	// that they wait for each other rather than deadlock.
	slices.SortFunc(removedRows, func(a, b *tupleRow) int { return cmp.Compare(a.ID, b.ID) })
	slices.SortFunc(addedRows, func(a, b *tupleRow) int {
		return cmp.Or(
			strings.Compare(a.ObjectType, b.ObjectType), strings.Compare(a.ObjectID, b.ObjectID),
			strings.Compare(a.Relation, b.Relation), strings.Compare(a.UserType, b.UserType),
			strings.Compare(a.UserID, b.UserID), strings.Compare(a.UserRelation, b.UserRelation),
		)
	})
	for _, batch := range chunks(removedRows) {
		ids := make([]int64, len(batch))
		for i, r := range batch {
			ids[i] = r.ID
		}
		n, err := tx.exec(ctx, "DELETE FROM gebra_tuple WHERE id IN ?", ids)
		if err != nil {
			return writeResult{}, errors.Wrap(err, "delete tuples")
		}
		if n != int64(len(ids)) {
			return writeResult{}, errWriteRace
		}
	}
	if len(addedRows) > 0 {
		values := make([][]any, len(addedRows))
		for i, r := range addedRows {
			values[i] = []any{r.StoreID, r.ObjectType, r.ObjectID, r.Relation, r.UserType, r.UserID, r.UserRelation, r.ConditionName, r.ConditionContext, r.CreatedAt}
		}
		cols := []string{"store_id", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "condition_name", "condition_context", "created_at"}
		if _, err := tx.insertRows(ctx, "gebra_tuple", cols, values, onConflict{}); err != nil {
			return writeResult{}, errors.Wrap(err, "insert tuples")
		}
	}

	// The inbox row comes last, so a write takes its inbox ID only after its
	// tuple changes succeeded. Another write of the same tuples has committed
	// by then, or this write waited on the tuples' rows or unique index
	// entries until it did, so of two writes of a tuple the later one has the
	// higher inbox ID.
	changes := make([]inboxChange, 0, len(removedRows)+len(addedRows))
	for _, r := range removedRows {
		changes = append(changes, newInboxChange(OperationDelete, r))
	}
	for _, r := range addedRows {
		changes = append(changes, newInboxChange(OperationWrite, r))
	}
	payload, err := json.Marshal(changes)
	if err != nil {
		return writeResult{}, errors.Wrap(err, "encode changes")
	}
	inboxID, err := tx.insertID(ctx, "INSERT INTO gebra_inbox (store_id, kind, changes, tuples, created_at) VALUES (?, ?, ?, ?, ?)",
		storeID, inboxTuples, string(payload), len(changes), now)
	if err != nil {
		return writeResult{}, errors.Wrap(err, "record write")
	}
	pending := make([][]any, 0, len(changes))
	for _, r := range removedRows {
		pending = append(pending, []any{inboxID, len(pending), storeID, r.ObjectType, r.ObjectID, r.Relation,
			r.UserType, r.UserID, r.UserRelation, true, r.ConditionName, r.ConditionContext, r.CreatedAt})
	}
	for _, r := range addedRows {
		pending = append(pending, []any{inboxID, len(pending), storeID, r.ObjectType, r.ObjectID, r.Relation,
			r.UserType, r.UserID, r.UserRelation, false, "", "", now})
	}
	cols := []string{"inbox_id", "seq", "store_id", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "deleted", "condition_name", "condition_context", "created_at"}
	if _, err := tx.insertRows(ctx, "gebra_pending", cols, pending, onConflict{}); err != nil {
		return writeResult{}, errors.Wrap(err, "record pending changes")
	}
	if hook := e.opts.hooks.beforeWriteCommit; hook != nil {
		hook(ctx, req)
	}
	return writeResult{revision: inboxID, written: len(addedRows), deleted: len(removedRows)}, nil
}

// writeResult is what a write transaction changed.
type writeResult struct {
	// revision is the write's inbox ID, or zero when it changed nothing.
	revision         int64
	written, deleted int
}

// writeRace returns which race a failed transaction lost, which a retry
// resolves, or "" when the error is not a race: a tuple that a concurrent
// writer added or deleted first, a deadlock between writers of the same
// tuples, or a serialization failure.
func writeRace(err error) string {
	if errors.Is(err, errWriteRace) {
		return "tuple_changed"
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return "tuple_added" // unique_violation
		case "40001":
			return "serialization" // serialization_failure
		case "40P01":
			return "deadlock" // deadlock_detected
		}
		return ""
	}
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		switch mysqlErr.Number {
		case 1062:
			return "tuple_added" // ER_DUP_ENTRY
		case 1213:
			return "deadlock" // ER_LOCK_DEADLOCK
		}
	}
	return ""
}

func sameJSON(a, b string) bool {
	if a == b {
		return true
	}
	var x, y any
	if json.Unmarshal([]byte(a), &x) != nil || json.Unmarshal([]byte(b), &y) != nil {
		return false
	}
	bx, _ := json.Marshal(x)
	by, _ := json.Marshal(y)
	return string(bx) == string(by)
}

// tupleIdent is the unique key of a stored tuple.
type tupleIdent struct {
	objectType, objectID, relation, userType, userID, userRelation string
}

func (t *tuple) ident() tupleIdent {
	return tupleIdent{t.objectType, t.objectID, t.relation, t.userType, t.userID, t.userRelation}
}

// findTuples returns the stored tuples among the given ones by their key.
func findTuples(ctx context.Context, tx dbtx, storeID string, ts []*tuple) (map[tupleIdent]*tupleRow, error) {
	out := make(map[tupleIdent]*tupleRow, len(ts))
	for _, batch := range rowChunks(ts) {
		keys := make([][]any, len(batch))
		for i, t := range batch {
			keys[i] = []any{t.objectType, t.objectID, t.relation, t.userType, t.userID, t.userRelation}
		}
		rows, err := queryStructs[tupleRow](ctx, tx, "SELECT * FROM gebra_tuple WHERE store_id = ? AND (object_type, object_id, relation, user_type, user_id, user_relation) IN ?", storeID, keys)
		if err != nil {
			return nil, errors.Wrap(err, "find tuples")
		}
		for i := range rows {
			r := &rows[i]
			out[tupleIdent{r.ObjectType, r.ObjectID, r.Relation, r.UserType, r.UserID, r.UserRelation}] = r
		}
	}
	return out, nil
}

func newInboxChange(op string, r *tupleRow) inboxChange {
	return inboxChange{
		Operation:  op,
		ObjectType: r.ObjectType, ObjectID: r.ObjectID, Relation: r.Relation,
		UserType: r.UserType, UserID: r.UserID, UserRelation: r.UserRelation,
		ConditionName: r.ConditionName, ConditionContext: r.ConditionContext,
	}
}

// ReadRequest filters stored tuples. Object is "type:id", "type:" for every
// object of a type, or empty. User is "type:id", "type:id#relation",
// "type:*", "type:" for every user of a type, or empty.
type ReadRequest struct {
	Object   string
	Relation string
	User     string
	// PageSize zero means 50.
	PageSize          int
	ContinuationToken string
}

// Read returns stored tuples that match the filter, in write order. It does
// not evaluate rewrites. It reads gebra_tuple, so it sees every committed
// write, including ones the applier has not applied yet.
func (e *Engine) Read(ctx context.Context, storeID string, req *ReadRequest) ([]Tuple, string, error) {
	db := e.db.handle()
	if _, err := e.storeRow(ctx, db, storeID); err != nil {
		return nil, "", err
	}
	q := "SELECT * FROM gebra_tuple WHERE store_id = ?"
	args := []any{storeID}
	if req.Object != "" {
		typ, id, ok := strings.Cut(req.Object, ":")
		if !ok || !validName(typ) {
			return nil, "", invalidf("invalid object filter %q, want type: or type:id", req.Object)
		}
		q += " AND object_type = ?"
		args = append(args, typ)
		if id != "" {
			q += " AND object_id = ?"
			args = append(args, id)
		}
	}
	if req.Relation != "" {
		q += " AND relation = ?"
		args = append(args, req.Relation)
	}
	if req.User != "" {
		if strings.HasSuffix(req.User, ":") {
			q += " AND user_type = ?"
			args = append(args, strings.TrimSuffix(req.User, ":"))
		} else {
			sub, err := parseSubject(req.User)
			if err != nil {
				return nil, "", err
			}
			q += " AND user_type = ? AND user_id = ? AND user_relation = ?"
			args = append(args, sub.typ, sub.id, sub.rel)
		}
	}
	if req.ContinuationToken != "" {
		last, err := decodeIntToken(req.ContinuationToken)
		if err != nil {
			return nil, "", err
		}
		q += " AND id > ?"
		args = append(args, last)
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	rows, err := queryStructs[tupleRow](ctx, db, q+" ORDER BY id LIMIT ?", append(args, pageSize+1)...)
	if err != nil {
		return nil, "", errors.Wrap(err, "read tuples")
	}
	var next string
	if len(rows) > pageSize {
		rows = rows[:pageSize]
		next = encodeToken(strconv.FormatInt(rows[pageSize-1].ID, 10))
	}
	out := make([]Tuple, len(rows))
	for i, r := range rows {
		t, err := r.tuple()
		if err != nil {
			return nil, "", err
		}
		out[i] = Tuple{Key: t.key(), Timestamp: r.CreatedAt}
	}
	return out, next, nil
}

// TupleChange is a tuple write or delete.
type TupleChange struct {
	TupleKey  TupleKey  `json:"tuple_key"`
	Operation string    `json:"operation"`
	Timestamp time.Time `json:"timestamp"`
}

// ReadChangesRequest pages through the change log. Type filters by object
// type when not empty.
type ReadChangesRequest struct {
	Type      string
	StartTime time.Time
	// PageSize zero means 50.
	PageSize          int
	ContinuationToken string
}

// ReadChanges returns changes in the order the store's applier applied them,
// so a write's changes appear once it is applied. The returned token is never
// empty, so a client can poll it for later changes.
func (e *Engine) ReadChanges(ctx context.Context, storeID string, req *ReadChangesRequest) ([]TupleChange, string, error) {
	db := e.db.handle()
	if _, err := e.storeRow(ctx, db, storeID); err != nil {
		return nil, "", err
	}
	q := "SELECT * FROM gebra_change WHERE store_id = ?"
	args := []any{storeID}
	if req.Type != "" {
		q += " AND object_type = ?"
		args = append(args, req.Type)
	}
	var last int64
	if req.ContinuationToken != "" {
		var err error
		if last, err = decodeIntToken(req.ContinuationToken); err != nil {
			return nil, "", err
		}
		q += " AND id > ?"
		args = append(args, last)
	} else if !req.StartTime.IsZero() {
		q += " AND created_at >= ?"
		args = append(args, req.StartTime.UTC())
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	rows, err := queryStructs[changeRow](ctx, db, q+" ORDER BY id LIMIT ?", append(args, pageSize)...)
	if err != nil {
		return nil, "", errors.Wrap(err, "read changes")
	}
	if req.ContinuationToken != "" {
		// Retention records its cutoff before it deletes changes, so reading
		// the cutoff after the changes catches a read that missed some.
		if err := checkChangesKept(ctx, db, storeID, last); err != nil {
			return nil, "", err
		}
	}
	out := make([]TupleChange, len(rows))
	for i, r := range rows {
		t, err := (&tupleRow{
			ObjectType: r.ObjectType, ObjectID: r.ObjectID, Relation: r.Relation,
			UserType: r.UserType, UserID: r.UserID, UserRelation: r.UserRelation,
			ConditionName: r.ConditionName, ConditionContext: r.ConditionContext,
		}).tuple()
		if err != nil {
			return nil, "", err
		}
		out[i] = TupleChange{TupleKey: t.key(), Operation: r.Operation, Timestamp: r.CreatedAt}
		last = r.ID
	}
	return out, encodeToken(strconv.FormatInt(last, 10)), nil
}

// ObjectAttributes sets attributes of an object, which list queries sort by.
// A nil value removes the attribute.
type ObjectAttributes struct {
	Object     string         `json:"object"`
	Attributes map[string]any `json:"attributes"`
}

// WriteAttributes sets object attributes. Attributes are not part of the
// view, so changing them never rewrites permission data.
func (e *Engine) WriteAttributes(ctx context.Context, storeID string, items []ObjectAttributes) error {
	return e.writeTx(ctx, func(tx dbtx) error {
		st, err := e.storeRow(ctx, tx, storeID)
		if err != nil {
			return err
		}
		m, err := e.model(ctx, tx, storeID, st.ModelID)
		if err != nil {
			return err
		}
		for _, it := range items {
			obj, err := parseObject(it.Object)
			if err != nil {
				return err
			}
			t := m.types[obj.typ]
			if t == nil {
				return invalidf("type %q is not defined", obj.typ)
			}
			for name, v := range it.Attributes {
				at, ok := t.attributes[name]
				if !ok {
					return invalidf("type %q has no attribute %q", obj.typ, name)
				}
				if v == nil {
					_, err := tx.exec(ctx, "DELETE FROM gebra_object_attribute WHERE store_id = ? AND object_type = ? AND name = ? AND object_id = ?", storeID, obj.typ, name, obj.id)
					if err != nil {
						return errors.Wrap(err, "delete attribute")
					}
					continue
				}
				enc, err := encodeAttribute(at, v)
				if err != nil {
					return invalidf("attribute %s.%s of %s: %v", obj.typ, name, it.Object, err)
				}
				_, err = tx.insertRows(ctx, "gebra_object_attribute", []string{"store_id", "object_type", "name", "object_id", "value"},
					[][]any{{storeID, obj.typ, name, obj.id, enc}},
					onConflict{key: []string{"store_id", "object_type", "name", "object_id"}, update: []string{"value"}})
				if err != nil {
					return errors.Wrap(err, "upsert attribute")
				}
			}
		}
		return nil
	})
}

// encodeAttribute encodes a value so that byte order matches value order.
func encodeAttribute(at AttributeType, v any) (string, error) {
	switch at {
	case AttributeString:
		s, ok := v.(string)
		if !ok {
			return "", errors.Newf("%v is not a string", v)
		}
		if len(s) > 191 {
			return "", errors.New("strings are limited to 191 bytes")
		}
		return s, nil
	case AttributeInt:
		f, ok := number(v)
		if !ok || f != math.Trunc(f) || f < math.MinInt64 || f > math.MaxInt64 {
			return "", errors.Newf("%v is not an integer", v)
		}
		return encodeInt(int64(f)), nil
	case AttributeTime:
		s, ok := v.(string)
		if !ok {
			return "", errors.Newf("%v is not an RFC 3339 timestamp", v)
		}
		ts, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return "", errors.Newf("%v is not an RFC 3339 timestamp", v)
		}
		return encodeInt(ts.UnixNano()), nil
	}
	return "", errors.Newf("unknown attribute type %q", at)
}

// encodeInt flips the sign bit so that negative numbers sort first, and uses
// fixed-width lowercase hex so that string order is numeric order.
func encodeInt(n int64) string {
	return fmt.Sprintf("%016x", uint64(n)^(1<<63))
}
