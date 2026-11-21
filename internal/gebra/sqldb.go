package gebra

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"
)

// Dialect is the kind of database an engine stores its data in.
type Dialect string

const (
	DialectPostgres Dialect = "postgres"
	DialectMySQL    Dialect = "mysql"
	DialectSQLite   Dialect = "sqlite"
)

// database runs the engine's statements on a database/sql pool.
//
// Statements use ? placeholders, which become $1, $2, and so on for
// Postgres. An argument that is a slice, other than []byte, expands into a
// parenthesized list of placeholders, so that "x IN ?" takes a slice, and an
// empty slice into (NULL). A slice of slices expands into a list of row
// values. The statements have no ? in literals.
//
// A statement that runs under another span, e.g., of a query or an applier
// batch, gets a client span with the OpenTelemetry database attributes, named
// after its verb and first table, e.g., "SELECT gebra_tuple". Its text has
// placeholders, never argument values. Failed statements are logged at debug
// level and slow ones at warn level.
type database struct {
	pool    *sql.DB
	dialect Dialect
	logger  *logx.Logger
	tracer  trace.Tracer
	// namespace is the name of the database, which spans report.
	namespace string
	// onStatement, when set, runs before every statement.
	onStatement func(ctx context.Context)

	// numbered caches the Postgres form of statements without slice
	// arguments, and names how statements are named in spans, both by their
	// text.
	numbered sync.Map
	names    sync.Map
}

// slowStatement is how long a statement may take before it is logged as slow.
const slowStatement = 200 * time.Millisecond

// querier is what statements run on: the pool, a transaction or a connection.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// dbtx runs statements on the pool, a transaction or a connection.
type dbtx struct {
	*database
	q querier
}

// handle returns a dbtx that runs statements on the pool.
func (d *database) handle() dbtx { return dbtx{database: d, q: d.pool} }

// rebind expands slice arguments and numbers the placeholders for Postgres.
func (d *database) rebind(query string, args []any) (string, []any) {
	hasSlice := false
	for _, a := range args {
		if isListArg(a) {
			hasSlice = true
			break
		}
	}
	if !hasSlice {
		if d.dialect != DialectPostgres {
			return query, args
		}
		if q, ok := d.numbered.Load(query); ok {
			return q.(string), args
		}
		q := numberPlaceholders(query)
		d.numbered.Store(query, q)
		return q, args
	}
	var b strings.Builder
	out := make([]any, 0, len(args))
	i := 0
	for _, r := range query {
		if r != '?' {
			b.WriteRune(r)
			continue
		}
		if i >= len(args) {
			b.WriteRune(r)
			continue
		}
		a := args[i]
		i++
		if !isListArg(a) {
			b.WriteRune('?')
			out = append(out, a)
			continue
		}
		v := reflect.ValueOf(a)
		if v.Len() == 0 {
			b.WriteString("(NULL)")
			continue
		}
		// A list of lists, e.g., of keys for "(a, b) IN ?", expands into a
		// list of row values.
		b.WriteByte('(')
		for j := range v.Len() {
			if j > 0 {
				b.WriteString(", ")
			}
			elem := v.Index(j).Interface()
			if !isListArg(elem) {
				b.WriteByte('?')
				out = append(out, elem)
				continue
			}
			row := reflect.ValueOf(elem)
			b.WriteString(placeholders(row.Len()))
			for k := range row.Len() {
				out = append(out, row.Index(k).Interface())
			}
		}
		b.WriteByte(')')
	}
	q := b.String()
	if d.dialect == DialectPostgres {
		q = numberPlaceholders(q)
	}
	return q, out
}

func isListArg(a any) bool {
	switch a.(type) {
	case nil, string, []byte, int, int64, bool, time.Time:
		return false
	case []string, []int64, []any:
		return true
	}
	t := reflect.TypeOf(a)
	return t.Kind() == reflect.Slice && t.Elem().Kind() != reflect.Uint8
}

// numberPlaceholders turns ? placeholders into $1, $2, and so on.
func numberPlaceholders(query string) string {
	var b strings.Builder
	n := 0
	for _, r := range query {
		if r == '?' {
			n++
			b.WriteString("$" + strconv.Itoa(n))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// statement is one statement in progress, for logging and tracing.
type statement struct {
	d     *database
	ctx   context.Context
	query string
	start time.Time
	span  trace.Span
}

func (d *database) begin(ctx context.Context, query string) *statement {
	if d.onStatement != nil {
		d.onStatement(ctx)
	}
	s := &statement{d: d, ctx: ctx, query: query, start: time.Now()}
	// Statements outside of an operation's span, e.g., of maintenance, would
	// each become a trace of their own.
	if d.tracer != nil && trace.SpanContextFromContext(ctx).IsValid() {
		n := d.statementName(query)
		attrs := []attribute.KeyValue{
			attribute.String("db.system.name", d.systemName()),
			attribute.String("db.operation.name", n.operation),
			attribute.String("db.query.text", query),
		}
		if d.namespace != "" {
			attrs = append(attrs, attribute.String("db.namespace", d.namespace))
		}
		if n.collection != "" {
			attrs = append(attrs, attribute.String("db.collection.name", n.collection))
		}
		_, s.span = d.tracer.Start(ctx, n.span, trace.WithSpanKind(trace.SpanKindClient), trace.WithAttributes(attrs...))
	}
	return s
}

// end ends the statement's span, with the rows it returned or affected, and
// logs the statement when it failed or was slow.
func (s *statement) end(rows int64, err error) {
	took := time.Since(s.start)
	if s.span != nil {
		s.span.SetAttributes(attribute.Int64("db.response.returned_rows", rows))
		if err != nil {
			kind := statementErrorType(err)
			s.span.SetAttributes(attribute.String("error.type", kind))
			if code := sqlState(err); code != "" {
				s.span.SetAttributes(attribute.String("db.response.status_code", code))
			}
			s.span.RecordError(err)
			s.span.SetStatus(codes.Error, err.Error())
		}
		s.span.End()
	}
	switch {
	case err != nil && !errors.Is(err, context.Canceled):
		s.d.logger.DebugContext(s.ctx, "Statement failed", "query", s.query, "duration", took, "error", err)
	case took >= slowStatement:
		s.d.logger.WarnContext(s.ctx, "Slow statement", "query", s.query, "duration", took, "rows", rows)
	}
}

// systemName returns the database's db.system.name.
func (d *database) systemName() string {
	if d.dialect == DialectPostgres {
		return "postgresql"
	}
	return string(d.dialect)
}

// statementName is how a statement is named in spans.
type statementName struct {
	operation, collection, span string
}

// statementName returns the statement's verb, its first table, and its span
// name, e.g., "SELECT gebra_tuple".
func (d *database) statementName(query string) statementName {
	if n, ok := d.names.Load(query); ok {
		return n.(statementName)
	}
	fields := strings.Fields(query)
	n := statementName{operation: "QUERY"}
	if len(fields) > 0 {
		n.operation = strings.ToUpper(fields[0])
	}
	for i, f := range fields {
		switch strings.ToUpper(f) {
		case "FROM", "INTO", "UPDATE", "TABLE":
			if i+1 < len(fields) && !strings.HasPrefix(fields[i+1], "(") {
				n.collection = strings.Trim(fields[i+1], "`\"(),")
			}
		}
		if n.collection != "" {
			break
		}
	}
	n.span = strings.TrimSpace(n.operation + " " + n.collection)
	d.names.Store(query, n)
	return n
}

// statementErrorType classifies a statement's error for error.type.
func statementErrorType(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	}
	if code := sqlState(err); code != "" {
		return code
	}
	return "error"
}

// sqlState returns the database's code of a statement's error: the SQLSTATE
// on Postgres and the error number on MySQL.
func sqlState(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	var myErr *mysql.MySQLError
	if errors.As(err, &myErr) {
		return strconv.Itoa(int(myErr.Number))
	}
	return ""
}

// exec runs a statement and returns the number of rows it affected.
func (db dbtx) exec(ctx context.Context, query string, args ...any) (int64, error) {
	query, args = db.rebind(query, args)
	s := db.begin(ctx, query)
	res, err := db.q.ExecContext(ctx, query, args...)
	var n int64
	if err == nil {
		n, err = res.RowsAffected()
	}
	s.end(n, err)
	return n, err
}

// query runs a statement and calls scan for each row it returns.
func (db dbtx) query(ctx context.Context, query string, args []any, scan func(rows *sql.Rows) error) error {
	query, args = db.rebind(query, args)
	s := db.begin(ctx, query)
	var n int64
	err := func() error {
		rows, err := db.q.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			n++
			if err := scan(rows); err != nil {
				return err
			}
		}
		return rows.Err()
	}()
	s.end(n, err)
	return err
}

// queryRow runs a statement and scans its first row into dest. It returns
// false when the statement returns no row.
func (db dbtx) queryRow(ctx context.Context, query string, args []any, dest ...any) (bool, error) {
	found := false
	err := db.query(ctx, query, args, func(rows *sql.Rows) error {
		if found {
			return nil
		}
		found = true
		return rows.Scan(dest...)
	})
	return found, err
}

// queryAll runs a statement and returns its rows, which scan reads.
func queryAll[T any](ctx context.Context, db dbtx, query string, args []any, scan func(rows *sql.Rows, v *T) error) ([]T, error) {
	var out []T
	err := db.query(ctx, query, args, func(rows *sql.Rows) error {
		var v T
		if err := scan(rows, &v); err != nil {
			return err
		}
		out = append(out, v)
		return nil
	})
	return out, err
}

// queryColumn runs a statement that returns one column and returns its
// values.
func queryColumn[T any](ctx context.Context, db dbtx, query string, args ...any) ([]T, error) {
	return queryAll(ctx, db, query, args, func(rows *sql.Rows, v *T) error {
		return rows.Scan(valueScanner{reflect.ValueOf(v).Elem()})
	})
}

// queryStructs runs a statement and returns its rows as values of the
// struct type T. A column goes to the field of the same name without
// underscores and case, e.g., store_id to StoreID, including the fields of
// embedded structs, and columns without a field are dropped. NULL becomes the
// zero value, and values convert between the types that the databases return
// for one column type, e.g., integers to bool and text to time.Time.
func queryStructs[T any](ctx context.Context, db dbtx, query string, args ...any) ([]T, error) {
	var out []T
	var fields [][]int
	err := db.query(ctx, query, args, func(rows *sql.Rows) error {
		if fields == nil {
			cols, err := rows.Columns()
			if err != nil {
				return err
			}
			fields = structFields(reflect.TypeFor[T](), cols)
		}
		out = append(out, *new(T))
		v := reflect.ValueOf(&out[len(out)-1]).Elem()
		dest := make([]any, len(fields))
		for i, f := range fields {
			if f == nil {
				dest[i] = new(any)
				continue
			}
			dest[i] = valueScanner{v.FieldByIndex(f)}
		}
		return rows.Scan(dest...)
	})
	return out, err
}

// structFieldIndexes caches the fields of struct types by normalized name.
var structFieldIndexes sync.Map

// structFields returns the index of the field of t that takes each column,
// or nil for a column without one.
func structFields(t reflect.Type, cols []string) [][]int {
	byName, ok := structFieldIndexes.Load(t)
	if !ok {
		m := map[string][]int{}
		for _, f := range reflect.VisibleFields(t) {
			if f.IsExported() && !f.Anonymous {
				if _, dup := m[strings.ToLower(f.Name)]; !dup {
					m[strings.ToLower(f.Name)] = f.Index
				}
			}
		}
		byName, _ = structFieldIndexes.LoadOrStore(t, m)
	}
	out := make([][]int, len(cols))
	for i, c := range cols {
		out[i] = byName.(map[string][]int)[strings.ToLower(strings.ReplaceAll(c, "_", ""))]
	}
	return out
}

// valueScanner scans a column into a field of a struct or a value of a
// column, with the conversions that queryStructs describes.
type valueScanner struct{ v reflect.Value }

func (s valueScanner) Scan(src any) error {
	v := s.v
	if src == nil {
		v.SetZero()
		return nil
	}
	if v.Kind() == reflect.Pointer {
		p := reflect.New(v.Type().Elem())
		if err := (valueScanner{p.Elem()}).Scan(src); err != nil {
			return err
		}
		v.Set(p)
		return nil
	}
	switch d := v.Addr().Interface().(type) {
	case *time.Time:
		switch x := src.(type) {
		case time.Time:
			*d = x
			return nil
		case string:
			return parseTime(d, x)
		case []byte:
			return parseTime(d, string(x))
		}
	case *bool:
		switch x := src.(type) {
		case bool:
			*d = x
			return nil
		case int64:
			*d = x != 0
			return nil
		case []byte:
			*d = string(x) == "1" || strings.EqualFold(string(x), "true")
			return nil
		case string:
			*d = x == "1" || strings.EqualFold(x, "true")
			return nil
		}
	case *string:
		switch x := src.(type) {
		case string:
			*d = x
			return nil
		case []byte:
			*d = string(x)
			return nil
		}
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var n sql.NullInt64
		if err := n.Scan(src); err != nil {
			return err
		}
		v.SetInt(n.Int64)
		return nil
	case reflect.Float32, reflect.Float64:
		var n sql.NullFloat64
		if err := n.Scan(src); err != nil {
			return err
		}
		v.SetFloat(n.Float64)
		return nil
	case reflect.String:
		var n sql.NullString
		if err := n.Scan(src); err != nil {
			return err
		}
		v.SetString(n.String)
		return nil
	}
	return errors.Newf("cannot scan %T into %s", src, v.Type())
}

// timeLayouts are the forms in which SQLite returns times that lost their
// column type, e.g., in COALESCE.
var timeLayouts = []string{
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02T15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02T15:04:05.999999999",
	time.RFC3339Nano,
}

func parseTime(dest *time.Time, s string) error {
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			*dest = t
			return nil
		}
	}
	return errors.Newf("cannot parse time %q", s)
}

// insertID runs an INSERT of one row into a table whose key is an
// auto-incremented id column, and returns the id.
func (db dbtx) insertID(ctx context.Context, query string, args ...any) (int64, error) {
	if db.dialect != DialectMySQL {
		var id int64
		_, err := db.queryRow(ctx, query+" RETURNING id", args, &id)
		return id, err
	}
	query, args = db.rebind(query, args)
	s := db.begin(ctx, query)
	res, err := db.q.ExecContext(ctx, query, args...)
	var id int64
	if err == nil {
		id, err = res.LastInsertId()
	}
	s.end(1, err)
	return id, err
}

// onConflict says what an INSERT does with a row whose key exists.
type onConflict struct {
	// ignore keeps the existing row.
	ignore bool
	// key lists the key columns, and update the columns that take the new
	// row's values.
	key, update []string
}

// insertRows inserts rows of the columns into the table, as many per
// statement as fit the placeholder limits of every database, and returns the
// number of rows inserted.
func (db dbtx) insertRows(ctx context.Context, table string, cols []string, rows [][]any, conflict onConflict) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	per := max(1, 30000/len(cols))
	var total int64
	for start := 0; start < len(rows); start += per {
		batch := rows[start:min(start+per, len(rows))]
		var b strings.Builder
		verb := "INSERT INTO "
		if conflict.ignore && db.dialect == DialectMySQL {
			verb = "INSERT IGNORE INTO "
		}
		b.WriteString(verb + table + " (" + strings.Join(cols, ", ") + ") VALUES ")
		row := placeholders(len(cols))
		args := make([]any, 0, len(batch)*len(cols))
		for i, r := range batch {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(row)
			args = append(args, r...)
		}
		switch {
		case conflict.ignore && db.dialect != DialectMySQL:
			b.WriteString(" ON CONFLICT DO NOTHING")
		case len(conflict.update) > 0 && db.dialect == DialectMySQL:
			sets := make([]string, len(conflict.update))
			for i, c := range conflict.update {
				sets[i] = c + " = VALUES(" + c + ")"
			}
			b.WriteString(" ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", "))
		case len(conflict.update) > 0:
			sets := make([]string, len(conflict.update))
			for i, c := range conflict.update {
				sets[i] = c + " = excluded." + c
			}
			fmt.Fprintf(&b, " ON CONFLICT (%s) DO UPDATE SET %s", strings.Join(conflict.key, ", "), strings.Join(sets, ", "))
		}
		n, err := db.exec(ctx, b.String(), args...)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// ignoreDone drops the error of rolling back a transaction that the
// database/sql package already rolled back because its context ended.
func ignoreDone(err error) error {
	if errors.Is(err, sql.ErrTxDone) {
		return nil
	}
	return err
}

// inTx reports whether db runs statements in a transaction.
func (db dbtx) inTx() bool {
	_, ok := db.q.(*sql.Tx)
	return ok
}

// tx runs fn in a transaction, which commits when fn returns nil and rolls
// back otherwise. In a transaction already, fn runs in it.
func (db dbtx) tx(ctx context.Context, opts *sql.TxOptions, fn func(tx dbtx) error) (err error) {
	if db.inTx() {
		return fn(db)
	}
	var tx *sql.Tx
	s := db.begin(ctx, "BEGIN")
	switch q := db.q.(type) {
	case *sql.DB:
		tx, err = q.BeginTx(ctx, opts)
	case *sql.Conn:
		tx, err = q.BeginTx(ctx, opts)
	default:
		err = errors.Newf("cannot begin a transaction on %T", db.q)
	}
	s.end(0, err)
	if err != nil {
		return errors.Wrap(err, "begin transaction")
	}
	committed := false
	defer func() {
		if !committed {
			s := db.begin(ctx, "ROLLBACK")
			s.end(0, ignoreDone(tx.Rollback()))
		}
	}()
	if err := fn(dbtx{database: db.database, q: tx}); err != nil {
		return err
	}
	committed = true
	s = db.begin(ctx, "COMMIT")
	err = tx.Commit()
	s.end(0, err)
	return errors.Wrap(err, "commit")
}
