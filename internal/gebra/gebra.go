// Package gebra is a fine-grained authorization engine in the style of Google
// Zanzibar. It accepts OpenFGA authorization models and offers the same
// queries, and it keeps a materialized view of union-only relations so that
// checks are single index probes and list queries are sorted, paginated, and
// unbounded.
package gebra

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"unknwon.dev/x/logx"
)

var (
	ErrInvalidArgument = errors.New("invalid argument")
	ErrNotFound        = errors.New("not found")
	// ErrMissingContext is returned when a condition needs a parameter that
	// neither the tuple nor the request provides. It is also an
	// ErrInvalidArgument.
	ErrMissingContext error = &invalidArgumentError{msg: "missing condition parameters"}
	// ErrNotApplied is returned by a write with WriteRequest.Wait when the
	// write committed but its store's applier did not apply it in time. The
	// write is durable and Write also returns its revision.
	ErrNotApplied = errors.New("write committed but not applied yet")
)

// invalidArgumentError is an ErrInvalidArgument with its own message.
type invalidArgumentError struct {
	msg string
}

func (e *invalidArgumentError) Error() string { return e.msg }

func (e *invalidArgumentError) Is(target error) bool { return target == ErrInvalidArgument }

// DefaultConditionCostLimit is the CEL evaluation cost a single condition may
// spend when Options.ConditionCostLimit is zero.
const DefaultConditionCostLimit = 10_000

// Options configures an Engine. The zero value is ready to use.
type Options struct {
	// ConditionCostLimit caps the CEL evaluation cost of one condition
	// evaluation, which protects the server from expensive expressions.
	ConditionCostLimit uint64
	// BatchCheckConcurrency is how many checks of one batch run at the same
	// time. Zero means 16.
	BatchCheckConcurrency int
	// ViewWait is how long a list query that needs a more current view waits
	// for the applier before it evaluates the graph instead. Checks and Expand
	// evaluate the graph without waiting. Zero means 100ms.
	ViewWait time.Duration
	// MaxStaleness is how far a store's view may lag behind its oldest
	// pending write before queries with the default consistency evaluate the
	// graph instead of using the view. Zero means 1s.
	MaxStaleness time.Duration
	// WaitTimeout is how long a write with WriteRequest.Wait waits for the
	// applier before it fails with ErrNotApplied. Zero means 10s.
	WaitTimeout time.Duration
	// ApplyInterval is how often the engine looks for writes that other
	// processes committed, and tries to become the applier of stores whose
	// applier is gone. Writes of this engine are applied right away when it
	// is their store's applier. Zero means 100ms.
	ApplyInterval time.Duration
	// ChangeRetention is how long the change log keeps a change after its
	// write. ReadChanges fails with ErrChangesExpired for a continuation token
	// before deleted changes. Zero means DefaultChangeRetention, and a
	// negative value keeps changes forever.
	ChangeRetention time.Duration

	// TracerProvider and MeterProvider receive the engine's spans and
	// metrics. Nil means the global providers of go.opentelemetry.io/otel.
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider

	// manualApply stops writes and the sweep from starting appliers, so that
	// tests can observe a view that lags.
	manualApply bool
	// buildChunkSize is how many tuples or sets a view build chunk covers.
	// Zero means defaultBuildChunkSize.
	buildChunkSize int
	hooks          testHooks
}

// testHooks run at points of a write or an applier batch, so that tests can
// hold transactions open or abort them there.
type testHooks struct {
	// beforeTuples runs in a write transaction before it reads the tuples
	// it changes.
	beforeTuples func(ctx context.Context, req *WriteRequest)
	// beforeWriteCommit runs in a write transaction right before it commits.
	beforeWriteCommit func(ctx context.Context, req *WriteRequest)
	// beforeApplyCommit runs in an applier transaction that applied writes,
	// right before it commits. An error aborts the batch.
	beforeApplyCommit func(ctx context.Context, storeID string) error
	// afterTupleRead runs after a query read stored tuples.
	afterTupleRead func(ctx context.Context)
	// afterStoreModels runs after a query resolved its model and the
	// store's serving view generation.
	afterStoreModels func(ctx context.Context)
	// beginSnapshot runs when a query starts a snapshot.
	beginSnapshot func(ctx context.Context)
	// statement runs before every statement.
	statement func(ctx context.Context)
	// beforeBuildChunk runs before the applier starts a view build chunk of
	// generation gen, outside of its transaction.
	beforeBuildChunk func(ctx context.Context, storeID string, gen int64)
	// beforeBuildCommit runs in a view build chunk's transaction right
	// before it commits. An error aborts the chunk.
	beforeBuildCommit func(ctx context.Context, chunk buildChunkInfo) error
}

// Engine evaluates authorization queries against stores kept in a SQL
// database. It is safe for concurrent use, and several engines, e.g., in
// different processes, may share one database.
//
// Writes only change tuples and record themselves in the inbox. One applier
// per store, which runs in one of the engines, owns everything derived from
// tuples: it applies committed writes to the materialized view in batches,
// records them in the change log, and deletes them from the inbox.
type Engine struct {
	db     *database
	logger *logx.Logger
	opts   Options

	tel *telemetry

	// models caches compiled models by ID. Models are immutable once written.
	models sync.Map
	// live caches each store's serving view by store ID, which every check
	// that uses it verifies.
	live sync.Map
	// effects holds the IDs of models whose rows of gebra_model_effect this
	// engine stored, and effectsMu serializes storing them.
	effects   sync.Map
	effectsMu sync.Mutex
	// sqliteWrites runs one write transaction at a time on SQLite, whose
	// shared cache fails a second writer at once instead of making it wait.
	// Snapshots hold it for reading.
	sqliteWrites sync.RWMutex

	// background is canceled by Close and stops the applier goroutines.
	background context.Context
	stop       context.CancelFunc
	applyMu    sync.Mutex
	// applying holds the stores whose applier goroutine is running.
	applying map[string]*applierRun
	// backoff holds the stores whose last applier batch failed, and when the
	// next attempt may start.
	backoff  map[string]*applierBackoff
	closed   bool
	appliers sync.WaitGroup
	leader   leadership

	// appliedMu guards applied, which is closed and replaced after every
	// applier batch commits, so that waiters in this process wake up.
	appliedMu sync.Mutex
	applied   chan struct{}

	// oldestMu guards oldest, which caches the time of each store's oldest
	// pending write for staleness checks.
	oldestMu sync.Mutex
	oldest   map[string]*oldestPending
	// analyzing is true while the engine updates the planner statistics of
	// the view and tuple tables, and analyzeAgain asks it to update them once
	// more when done. lastAnalyze is when the last update started. Only the
	// goroutine that set analyzing reads or writes it.
	analyzing    atomic.Bool
	analyzeAgain atomic.Bool
	lastAnalyze  time.Time
	// sinceAnalyze is how many rows applier batches changed since the last
	// update.
	sinceAnalyze atomic.Int64
}

// writeTx runs fn in a write transaction.
func (e *Engine) writeTx(ctx context.Context, fn func(tx dbtx) error, opts ...*sql.TxOptions) error {
	if e.db.dialect == DialectSQLite {
		e.sqliteWrites.Lock()
		defer e.sqliteWrites.Unlock()
	}
	var o *sql.TxOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	return e.db.handle().tx(ctx, o, fn)
}

// New returns an engine that stores its data in db, a database of the dialect,
// creating or migrating its tables as needed. Close stops its background work.
//
// On Postgres and MySQL, the pool needs at least two connections: migrations
// and appliers each hold one while they run statements on others.
func New(ctx context.Context, logger *logx.Logger, db *sql.DB, dialect Dialect, opts Options) (*Engine, error) {
	if opts.ConditionCostLimit == 0 {
		opts.ConditionCostLimit = DefaultConditionCostLimit
	}
	if opts.BatchCheckConcurrency <= 0 {
		opts.BatchCheckConcurrency = 16
	}
	if opts.ViewWait <= 0 {
		opts.ViewWait = 100 * time.Millisecond
	}
	if opts.WaitTimeout <= 0 {
		opts.WaitTimeout = 10 * time.Second
	}
	if opts.ApplyInterval <= 0 {
		opts.ApplyInterval = 100 * time.Millisecond
	}
	if opts.MaxStaleness <= 0 {
		opts.MaxStaleness = time.Second
	}
	if opts.ChangeRetention == 0 {
		opts.ChangeRetention = DefaultChangeRetention
	}
	if opts.buildChunkSize <= 0 {
		opts.buildChunkSize = defaultBuildChunkSize
	}
	switch dialect {
	case DialectPostgres, DialectMySQL, DialectSQLite:
	default:
		return nil, errors.Newf("unsupported dialect %q", dialect)
	}
	e := &Engine{
		db: &database{pool: db, dialect: dialect, logger: logger.Scoped("db"), onStatement: opts.hooks.statement}, logger: logger, opts: opts,
		applying: map[string]*applierRun{}, backoff: map[string]*applierBackoff{}, applied: make(chan struct{}),
		oldest: map[string]*oldestPending{},
		leader: leadership{held: map[string]bool{}, tried: map[string]time.Time{}},
	}
	var err error
	if e.tel, err = newTelemetry(e, opts.TracerProvider, opts.MeterProvider); err != nil {
		return nil, errors.Wrap(err, "set up telemetry")
	}
	if err := e.migrate(ctx); err != nil {
		_ = e.tel.gauges.Unregister()
		return nil, err
	}
	switch dialect {
	case DialectPostgres:
		err = e.db.pool.QueryRowContext(ctx, "SELECT current_database()").Scan(&e.db.namespace)
	case DialectMySQL:
		err = e.db.pool.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&e.db.namespace)
	}
	if err != nil {
		_ = e.tel.gauges.Unregister()
		return nil, errors.Wrap(err, "read database name")
	}
	e.db.tracer = e.tel.tracer
	e.background, e.stop = context.WithCancel(context.WithoutCancel(ctx))
	e.appliers.Add(2)
	go e.sweep()
	go e.maintenance()
	return e, nil
}

// Close stops the engine's background work and waits for it to finish.
func (e *Engine) Close() {
	e.applyMu.Lock()
	e.closed = true
	e.applyMu.Unlock()
	e.stop()
	e.appliers.Wait()
	e.releaseLeadership()
	if err := e.tel.gauges.Unregister(); err != nil {
		e.logger.Warn("Failed to unregister view gauges", "error", err)
	}
}

var idGen struct {
	sync.Mutex
	lastMillis uint64
	lastRand   [10]byte
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// newID returns a ULID: 26 characters that sort by creation time, including
// IDs created in the same millisecond by this process.
func newID() string {
	idGen.Lock()
	ms := uint64(time.Now().UnixMilli())
	if ms <= idGen.lastMillis {
		ms = idGen.lastMillis
		for i := len(idGen.lastRand) - 1; i >= 0; i-- {
			idGen.lastRand[i]++
			if idGen.lastRand[i] != 0 {
				break
			}
		}
	} else {
		_, _ = rand.Read(idGen.lastRand[:])
	}
	idGen.lastMillis = ms
	var raw [16]byte
	binary.BigEndian.PutUint16(raw[0:2], uint16(ms>>32))
	binary.BigEndian.PutUint32(raw[2:6], uint32(ms))
	copy(raw[6:], idGen.lastRand[:])
	idGen.Unlock()

	// 128 bits as 26 base32 digits, the first digit carrying the top 3 bits.
	var out [26]byte
	hi := binary.BigEndian.Uint64(raw[0:8])
	lo := binary.BigEndian.Uint64(raw[8:16])
	for i := 25; i >= 0; i-- {
		out[i] = crockford[lo&31]
		lo = lo>>5 | hi<<59
		hi >>= 5
	}
	return string(out[:])
}
