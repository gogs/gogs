package gebra

// The applier benchmark measures the engine on Postgres. It builds its
// dataset in a fresh database, runs the workloads, appends one JSON line per
// result to GEBRA_BENCH_OUT, and drops the database:
//
//	GOGS_DATABASE_TYPE=postgres PGHOST=127.0.0.1 PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres PGSSLMODE=disable \
//	GEBRA_BENCH=applier GEBRA_BENCH_SCALE=1 GEBRA_BENCH_ROUND=1 GEBRA_BENCH_OUT=$PWD/applier-bench.jsonl \
//	go test -p 1 -count=1 -timeout 4h -run 'TestApplierBench$' -v ./internal/gebra
//
// Then print the tables:
//
//	GEBRA_BENCH=summary GEBRA_BENCH_OUT=$PWD/applier-bench.jsonl go test -count=1 -run TestApplierBenchSummary -v ./internal/gebra
//
// GEBRA_BENCH_SCALE 1 is about 46k tuples per store and 10 about 457k, with
// GEBRA_BENCH_STORES stores (default 4). GEBRA_BENCH_USERS, GEBRA_BENCH_TEAMS,
// GEBRA_BENCH_ORGS and GEBRA_BENCH_REPOS override the dataset's sizes.
// GEBRA_BENCH_TEMPLATE copies a database kept with GEBRA_BENCH_KEEP instead
// of loading the dataset. GEBRA_BENCH_DURATION sets each workload's length (default 8s),
// GEBRA_BENCH_WORKLOADS a comma-separated subset of w1, w4, w16, w64, w16wait,
// w64wait, w16x4, b16, c64, mix, mixmin, mixhigh, mixmin1, mixhigh1, listmin,
// fanout, failover, and modelswap (all by default), GEBRA_BENCH_CONNS the connection pool size (default 96, so
// Postgres needs max_connections of at least 150 for the failover workload),
// GEBRA_BENCH_FANOUT as users,repos,subteams the fan-out (default
// 5000,500,1000), and GEBRA_BENCH_KEEP keeps the database. The realistic
// workload, real, is not part of the default set; applierbench_real_test.go
// describes it and its settings.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

func benchEnv(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func benchDBOpts(name string) conf.DatabaseOpts {
	return conf.DatabaseOpts{
		Type: "postgres", Host: os.ExpandEnv("$PGHOST:$PGPORT"), Name: name, Schema: "public",
		User: os.Getenv("PGUSER"), Password: os.Getenv("PGPASSWORD"), SSLMode: os.Getenv("PGSSLMODE"),
	}
}

func benchOpen(t testing.TB, name string, conns int) *sql.DB {
	t.Helper()
	dsn, err := dbx.NewDSN(benchDBOpts(name))
	require.NoError(t, err)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(conns)
	db.SetMaxIdleConns(conns)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// applierBench is one run of the benchmark against one package.
type applierBench struct {
	t      *testing.T
	ctx    context.Context
	dbName string
	db     *sql.DB
	admin  *sql.DB
	e      *Engine
	reader *sdkmetric.ManualReader
	stores []string
	scale  int
	base   benchResult
	out    *os.File
}

// benchResult is one workload of one run, as a line of the output file.
type benchResult struct {
	Variant  string `json:"variant"`
	Scale    int    `json:"scale"`
	Round    string `json:"round"`
	Workload string `json:"workload"`
	Load     string `json:"load"`

	// Ops is writes or checks per second, and the latencies are theirs. For
	// mix and list workloads they are the writes, and Check* the checks or
	// list queries.
	Ops    float64 `json:"ops"`
	N      int     `json:"n"`
	P50    float64 `json:"p50_ms"`
	P99    float64 `json:"p99_ms"`
	Max    float64 `json:"max_ms"`
	Errors int     `json:"errors"`
	Err    string  `json:"err,omitempty"`

	CheckOps float64 `json:"check_ops,omitempty"`
	CheckP50 float64 `json:"check_p50_ms,omitempty"`
	CheckP99 float64 `json:"check_p99_ms,omitempty"`
	CheckMax float64 `json:"check_max_ms,omitempty"`
	// Fallbacks counts queries that evaluated the graph, by reason.
	Fallbacks map[string]int64 `json:"fallbacks,omitempty"`

	// Lag is the time from a write's commit to the applier batch that
	// applied it, from the gebra.applier.lag histogram: quantiles are bucket
	// bounds, and -1 means above the top bucket. Batch is writes per applier
	// batch.
	LagP50   float64 `json:"lag_p50_ms,omitempty"`
	LagP99   float64 `json:"lag_p99_ms,omitempty"`
	LagMax   float64 `json:"lag_max_ms,omitempty"`
	Batches  int64   `json:"batches,omitempty"`
	BatchAvg float64 `json:"batch_avg,omitempty"`
	BatchP99 float64 `json:"batch_p99,omitempty"`
	// CatchUp is how long the views took to include every write after the
	// workload stopped.
	CatchUp float64 `json:"catch_up_s,omitempty"`

	// LockSamples is how often the run sampled pg_locks, and LockWaits the
	// backends found waiting for a lock, by lock type and relation.
	LockSamples int            `json:"lock_samples,omitempty"`
	LockWaits   map[string]int `json:"lock_waits,omitempty"`

	// Extra holds the measurements of fanout and failover.
	Extra map[string]float64 `json:"extra,omitempty"`

	// Rate is the arrival rate of the realistic workload, Achieved the rate
	// of arrivals it dispatched, PerOp its operations by kind, and
	// Violations the changes that checks with their revision did not see.
	Rate       float64                 `json:"rate,omitempty"`
	Achieved   float64                 `json:"achieved,omitempty"`
	PerOp      map[string]benchOpStats `json:"per_op,omitempty"`
	Violations int                     `json:"violations,omitempty"`
}

// TestApplierBench runs the benchmark when GEBRA_BENCH is "applier".
func TestApplierBench(t *testing.T) {
	if os.Getenv("GEBRA_BENCH") != "applier" {
		t.Skip("set GEBRA_BENCH=applier to run the applier benchmark")
	}
	if os.Getenv("GOGS_DATABASE_TYPE") != "postgres" {
		t.Fatal("the applier benchmark runs on Postgres")
	}
	ctx := context.Background()
	scale, err := strconv.Atoi(benchEnv("GEBRA_BENCH_SCALE", "1"))
	require.NoError(t, err)
	conns, err := strconv.Atoi(benchEnv("GEBRA_BENCH_CONNS", "96"))
	require.NoError(t, err)
	dsn, err := dbx.NewDSN(benchDBOpts("postgres"))
	require.NoError(t, err)
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("gebra_bench_%s_%d", benchVariant, time.Now().UnixNano())
	createBenchDB(t, admin, name)
	if os.Getenv("GEBRA_BENCH_KEEP") == "" {
		t.Cleanup(func() {
			_, err := admin.Exec(fmt.Sprintf("DROP DATABASE %q WITH (FORCE)", name))
			require.NoError(t, err)
		})
	}
	db := benchOpen(t, name, conns)
	reader := sdkmetric.NewManualReader()
	opts := benchOptions()
	opts.MeterProvider = sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	e, err := New(ctx, logx.NewNoopLogger(), db, DialectPostgres, opts)
	require.NoError(t, err)
	t.Cleanup(e.Close)
	b := &applierBench{
		t: t, ctx: ctx, dbName: name, db: db, admin: admin, e: e, reader: reader, scale: scale,
		base: benchResult{Variant: benchVariant, Scale: scale, Round: benchEnv("GEBRA_BENCH_ROUND", "")},
	}
	if path := os.Getenv("GEBRA_BENCH_OUT"); path != "" {
		b.out, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		require.NoError(t, err)
		defer func() { _ = b.out.Close() }()
	}
	b.load()

	d, err := time.ParseDuration(benchEnv("GEBRA_BENCH_DURATION", "8s"))
	require.NoError(t, err)
	users, teams, _, repos, gen, _ := benchLadderDataset(scale)
	// Writes toggle tuples that the dataset does not have: a write of an
	// existing tuple changes nothing, and deleting it afterwards would remove
	// it from the dataset.
	dataset := map[TupleKey]bool{}
	gen(func(k TupleKey) { dataset[k] = true })
	single := func(seed uint64, w, i int) *WriteRequest {
		rng := rand.New(rand.NewPCG(seed*1000+uint64(w), uint64(i/2)))
		k := benchWriteKey(rng, dataset, users, teams, repos)
		req := &WriteRequest{OnDuplicate: "ignore", OnMissing: "ignore"}
		if i%2 == 0 {
			req.Writes = []TupleKey{k}
		} else {
			req.Deletes = []TupleKey{k}
		}
		return req
	}
	batch := func(seed uint64, w, i int) *WriteRequest {
		rng := rand.New(rand.NewPCG(seed*1000+uint64(w), uint64(i/2)))
		seen := map[TupleKey]bool{}
		var keys []TupleKey
		for len(keys) < 100 {
			if k := benchWriteKey(rng, dataset, users, teams, repos); !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
		}
		req := &WriteRequest{OnDuplicate: "ignore", OnMissing: "ignore"}
		if i%2 == 0 {
			req.Writes = keys
		} else {
			req.Deletes = keys
		}
		return req
	}
	// latest holds the last revision a writer of the first store got, which
	// mixmin checkers pass as their minimum revision.
	var latest atomic.Value
	latest.Store("")
	writer := func(seed uint64, stores int, wait bool, gen func(seed uint64, w, i int) *WriteRequest) func(ctx context.Context, w, i int) error {
		return func(ctx context.Context, w, i int) error {
			req := gen(seed, w, i)
			benchSetWait(req, wait)
			store := w % min(stores, len(b.stores))
			rev, err := e.Write(ctx, b.stores[store], req)
			if store == 0 && rev != "" {
				latest.Store(rev)
			}
			return err
		}
	}
	qs := benchChecks(scale)
	checker := func(c func() Consistency) func(ctx context.Context, w, i int) error {
		return func(ctx context.Context, w, i int) error {
			q := qs[(w*131+i)%len(qs)]
			_, err := e.Check(ctx, b.stores[0], &CheckRequest{
				TupleKey:    CheckTupleKey{User: q[0], Relation: q[1], Object: q[2]},
				Consistency: c(),
			})
			return err
		}
	}
	// lister lists a page of the repos that a user of a check query reads.
	lister := func(c func() Consistency) func(ctx context.Context, w, i int) error {
		return func(ctx context.Context, w, i int) error {
			q := qs[(w*131+i)%len(qs)]
			_, _, err := e.ListObjects(ctx, b.stores[0], &ListObjectsRequest{
				Type: "repo", Relation: "reader", User: q[0], PageSize: 50, Consistency: c(),
			})
			return err
		}
	}
	byDefault := func() Consistency { return Consistency{} }
	minRevision := func() Consistency { return Consistency{MinRevision: latest.Load().(string)} }
	higher := func() Consistency { return Consistency{Preference: ConsistencyHigher} }

	// Warm up connections and caches.
	b.loop(16, 2*time.Second, writer(99, 4, false, single), false).wait()
	b.loop(16, time.Second, checker(byDefault), false).wait()
	b.awaitViews()

	all := "w1,w4,w16,w64,w16wait,w64wait,w16x4,b16,c64,mix,mixmin,mixhigh,mixmin1,mixhigh1,listmin,fanout,failover,modelswap"
	for _, wl := range strings.Split(benchEnv("GEBRA_BENCH_WORKLOADS", all), ",") {
		var r benchResult
		switch wl {
		case "w1", "w4", "w16", "w64":
			n, _ := strconv.Atoi(wl[1:])
			r = b.measure(wl, n, d, writer(uint64(n), 1, false, single), 0, nil)
		case "w16wait", "w64wait":
			n, _ := strconv.Atoi(strings.TrimSuffix(wl[1:], "wait"))
			r = b.measure(wl, n, d, writer(uint64(n)+100, 1, true, single), 0, nil)
		case "w16x4":
			r = b.measure(wl, 16, d, writer(5, 4, false, single), 0, nil)
		case "b16":
			r = b.measure(wl, 16, d, writer(6, 1, false, batch), 0, nil)
		case "c64":
			r = b.measure(wl, 64, d, checker(byDefault), 0, nil)
		case "mix":
			r = b.measure(wl, 16, d, writer(7, 1, false, single), 64, checker(byDefault))
		case "mixmin":
			r = b.measure(wl, 16, d, writer(8, 1, false, single), 64, checker(minRevision))
		case "mixhigh":
			r = b.measure(wl, 16, d, writer(9, 1, false, single), 64, checker(higher))
		case "mixmin1":
			r = b.measure(wl, 1, d, writer(10, 1, false, single), 64, checker(minRevision))
		case "mixhigh1":
			r = b.measure(wl, 1, d, writer(11, 1, false, single), 64, checker(higher))
		case "listmin":
			r = b.measure(wl, 16, d, writer(12, 1, false, single), 16, lister(minRevision))
		case "real":
			for _, r := range b.realLoad(d) {
				b.emit(r)
			}
			continue
		case "fanout":
			r = b.fanout()
		case "modelswap":
			r = b.modelSwap(writer(13, 1, false, single), checker(byDefault))
		case "failover":
			r = b.base
			r.Workload = wl
			r.Load = benchLoadAvg()
			r.Extra = benchFailover(b)
			if r.Extra == nil {
				continue
			}
		default:
			t.Fatalf("unknown workload %q", wl)
		}
		b.emit(r)
	}
}

func (b *applierBench) emit(r benchResult) {
	line, err := json.Marshal(r)
	require.NoError(b.t, err)
	b.t.Log(string(line))
	if b.out != nil {
		_, err = fmt.Fprintln(b.out, string(line))
		require.NoError(b.t, err)
	}
}

// load creates benchStores() stores with the ladder dataset. The first one is loaded
// with COPY and its view rebuilt, and the others copy its rows.
func (b *applierBench) load() {
	t, ctx, e := b.t, b.ctx, b.e
	start := time.Now()
	if os.Getenv("GEBRA_BENCH_TEMPLATE") != "" {
		stores, _, err := e.ListStores(ctx, "", 0, "")
		require.NoError(t, err)
		require.NotEmpty(t, stores, "the template database has the benchmark's stores")
		for _, st := range stores {
			benchAwait(t, e, st.ID)
			b.stores = append(b.stores, st.ID)
		}
		t.Logf("%s: reused %d stores of the template in %s", benchVariant, len(b.stores), time.Since(start))
		return
	}
	_, _, _, _, gen, attrs := benchLadderDataset(b.scale)
	m, err := ParseDSL(benchLadderModel)
	require.NoError(t, err)
	st, err := e.CreateStore(ctx, "s0")
	require.NoError(t, err)
	modelID, err := e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	benchAwait(t, e, st.ID)
	cm, err := e.model(ctx, e.db.handle(), st.ID, modelID)
	require.NoError(t, err)
	now := time.Now().UTC()
	n := b.copyRows("gebra_tuple", []string{"store_id", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "condition_name", "condition_context", "created_at"}, func(emit func([]any)) {
		gen(func(k TupleKey) {
			tp, err := parseTupleKey(k)
			if err == nil {
				err = cm.validateTuple(tp)
			}
			if err != nil {
				panic(fmt.Sprintf("invalid generated tuple %+v: %v", k, err))
			}
			cctx, _ := tp.contextJSON()
			emit([]any{st.ID, tp.objectType, tp.objectID, tp.relation, tp.userType, tp.userID, tp.userRelation, tp.conditionName, cctx, now})
		})
	})
	b.copyRows("gebra_object_attribute", []string{"store_id", "object_type", "name", "object_id", "value"}, func(emit func([]any)) {
		attrs(func(object, name string, v any) {
			obj, _ := parseObject(object)
			enc, err := encodeAttribute(cm.types[obj.typ].attributes[name], v)
			if err != nil {
				panic(err)
			}
			emit([]any{st.ID, obj.typ, name, obj.id, enc})
		})
	})
	_, err = b.db.ExecContext(ctx, "ANALYZE")
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	benchAwait(t, e, st.ID)
	b.stores = []string{st.ID}
	for i := 1; i < benchStores(); i++ {
		cp, err := e.CreateStore(ctx, fmt.Sprintf("s%d", i))
		require.NoError(t, err)
		_, err = e.WriteAuthorizationModel(ctx, cp.ID, m)
		require.NoError(t, err)
		benchAwait(t, e, cp.ID)
		for _, q := range []string{
			`INSERT INTO gebra_tuple (store_id, object_type, object_id, relation, user_type, user_id, user_relation, condition_name, condition_context, created_at)
				SELECT ?, object_type, object_id, relation, user_type, user_id, user_relation, condition_name, condition_context, created_at FROM gebra_tuple WHERE store_id = ? ORDER BY id`,
			`INSERT INTO gebra_object_attribute (store_id, object_type, name, object_id, value) SELECT ?, object_type, name, object_id, value FROM gebra_object_attribute WHERE store_id = ?`,
			`INSERT INTO gebra_view_member (store_id, gen, set_key, user_type, user_id, set_type, set_rel, set_id)
				SELECT c.store_id, c.gen, set_key, user_type, user_id, set_type, set_rel, set_id FROM gebra_view_member x
				JOIN gebra_view_state c ON c.store_id = ? JOIN gebra_view_state s ON s.store_id = x.store_id AND s.gen = x.gen WHERE x.store_id = ?`,
			`INSERT INTO gebra_view_edge (store_id, gen, child_key, parent_key, refs)
				SELECT c.store_id, c.gen, child_key, parent_key, refs FROM gebra_view_edge x
				JOIN gebra_view_state c ON c.store_id = ? JOIN gebra_view_state s ON s.store_id = x.store_id AND s.gen = x.gen WHERE x.store_id = ?`,
			`INSERT INTO gebra_view_reach (store_id, gen, desc_key, anc_type, anc_rel, anc_id, desc_type, desc_rel, desc_id)
				SELECT c.store_id, c.gen, desc_key, anc_type, anc_rel, anc_id, desc_type, desc_rel, desc_id FROM gebra_view_reach x
				JOIN gebra_view_state c ON c.store_id = ? JOIN gebra_view_state s ON s.store_id = x.store_id AND s.gen = x.gen WHERE x.store_id = ?`,
		} {
			_, err = e.db.handle().exec(ctx, q, cp.ID, st.ID)
			require.NoError(t, err)
		}
		_, err = e.db.handle().exec(ctx, `UPDATE gebra_view_state SET member_rows = s.member_rows, edge_rows = s.edge_rows, reach_rows = s.reach_rows
			FROM gebra_view_state s WHERE s.store_id = ? AND gebra_view_state.store_id = ?`, st.ID, cp.ID)
		require.NoError(t, err)
		b.stores = append(b.stores, cp.ID)
	}
	// A parallel vacuum can need more shared memory than containers have.
	_, err = b.db.ExecContext(ctx, "VACUUM (ANALYZE, PARALLEL 0)")
	require.NoError(t, err)
	t.Logf("%s: loaded %d stores of %d tuples in %s", benchVariant, len(b.stores), n, time.Since(start))
}

func (b *applierBench) copyRows(table string, cols []string, gen func(emit func([]any))) int {
	conn, err := b.db.Conn(b.ctx)
	require.NoError(b.t, err)
	defer func() { _ = conn.Close() }()
	rows := make(chan []any, 10_000)
	go func() {
		defer close(rows)
		gen(func(r []any) { rows <- r })
	}()
	var n int64
	err = conn.Raw(func(dc any) error {
		var err error
		n, err = dc.(*stdlib.Conn).Conn().CopyFrom(b.ctx, pgx.Identifier{table}, cols, &benchRows{ch: rows})
		return err
	})
	for range rows {
	}
	require.NoError(b.t, err)
	return int(n)
}

type benchRows struct {
	ch  <-chan []any
	cur []any
}

func (r *benchRows) Next() bool {
	var ok bool
	r.cur, ok = <-r.ch
	return ok
}

func (r *benchRows) Values() ([]any, error) { return r.cur, nil }
func (r *benchRows) Err() error             { return nil }

// awaitViews waits until every store's view includes every write and returns
// how long that took.
func (b *applierBench) awaitViews() time.Duration {
	start := time.Now()
	for _, s := range b.stores {
		benchAwait(b.t, b.e, s)
	}
	return time.Since(start)
}

func (b *applierBench) measure(name string, workers int, d time.Duration, fn func(ctx context.Context, w, i int) error, checkers int, check func(ctx context.Context, w, i int) error) benchResult {
	r := b.base
	r.Workload = name
	r.Load = benchLoadAvg()
	before := b.metrics()
	var checks *benchStats
	loadDiag := b.diag(name+"_load", "")
	if checkers > 0 {
		checks = b.loop(checkers, d, check, false)
	}
	s := b.loop(workers, d, fn, true)
	s.wait()
	loadDiag.finish(&r)
	if checks != nil {
		checks.wait()
		r.CheckOps, r.CheckP50, r.CheckP99, r.CheckMax = checks.summary()
	}
	r.Ops, r.P50, r.P99, r.Max = s.summary()
	r.N, r.Errors = len(s.samples), s.errs
	if checks != nil {
		r.Errors += checks.errs
	}
	if s.lastErr != nil {
		r.Err = s.lastErr.Error()
	} else if checks != nil && checks.lastErr != nil {
		r.Err = checks.lastErr.Error()
	}
	r.LockSamples, r.LockWaits = s.lockSamples, s.lockWaits
	if name != "c64" {
		pw, pc := b.diagPending()
		cu := b.diag(name+"_catchup", "")
		r.CatchUp = b.awaitViews().Seconds()
		cu.extra["catchup_writes"], cu.extra["catchup_changes"] = float64(pw), float64(pc)
		if r.CatchUp > 0 {
			cu.extra["catchup_changes_per_s"] = float64(pc) / r.CatchUp
		}
		cu.finish(&r)
	}
	b.metrics().since(before, &r)
	return r
}

// fanout adds a team with many members and subteams to an org with many
// repos while writers keep writing to the same store with Wait, then removes
// it again, and reports how long the writers queued behind it took. Writes
// that failed, e.g., because the applier did not apply them within the wait
// timeout, count with the time they took, and are also counted apart.
func (b *applierBench) fanout() benchResult {
	t, ctx, e := b.t, b.ctx, b.e
	r := b.base
	r.Workload = "fanout"
	r.Load = benchLoadAvg()
	r.Extra = map[string]float64{}
	var users, repos, subteams int
	_, err := fmt.Sscanf(benchEnv("GEBRA_BENCH_FANOUT", "5000,500,1000"), "%d,%d,%d", &users, &repos, &subteams)
	require.NoError(t, err)
	m, err := ParseDSL(benchLadderModel)
	require.NoError(t, err)
	st, err := e.CreateStore(ctx, "fanout")
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	benchAwait(t, e, st.ID)
	now := time.Now().UTC()
	b.copyRows("gebra_tuple", []string{"store_id", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "condition_name", "condition_context", "created_at"}, func(emit func([]any)) {
		tuple := func(objType, objID, rel, userType, userID, userRel string) {
			emit([]any{st.ID, objType, objID, rel, userType, userID, userRel, "", "", now})
		}
		for i := range users {
			tuple("team", "fan", "member", "user", fmt.Sprintf("f%06d", i), "")
		}
		for i := range subteams {
			tuple("team", "fan", "member", "team", fmt.Sprintf("fs%05d", i), "member")
			tuple("team", fmt.Sprintf("fs%05d", i), "member", "user", fmt.Sprintf("s%05d", i), "")
		}
		for i := range repos {
			tuple("repo", fmt.Sprintf("fan%05d", i), "owner_org", "org", "fan", "")
		}
	})
	_, err = b.db.ExecContext(ctx, "ANALYZE gebra_tuple")
	require.NoError(t, err)
	_, err = e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	benchAwait(t, e, st.ID)
	_, err = b.db.ExecContext(ctx, "ANALYZE")
	require.NoError(t, err)

	link := benchKey("team:fan#member", "member", "org:fan")
	for _, step := range []struct {
		name string
		req  func() *WriteRequest
	}{
		{"add", func() *WriteRequest { return &WriteRequest{Writes: []TupleKey{link}} }},
		{"remove", func() *WriteRequest { return &WriteRequest{Deletes: []TupleKey{link}} }},
	} {
		var mu sync.Mutex
		type sample struct {
			start time.Time
			took  time.Duration
			err   error
		}
		var samples []sample
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for w := range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rng := rand.New(rand.NewPCG(uint64(w), 77))
				for i := 0; ; i++ {
					select {
					case <-stop:
						return
					default:
					}
					k := benchKey(fmt.Sprintf("user:w%06d", rng.IntN(10_000)), "member", fmt.Sprintf("team:w%04d", rng.IntN(1000)))
					req := &WriteRequest{OnDuplicate: "ignore", OnMissing: "ignore"}
					if i%2 == 0 {
						req.Writes = []TupleKey{k}
					} else {
						req.Deletes = []TupleKey{k}
					}
					benchSetWait(req, true)
					t0 := time.Now()
					_, err := e.Write(ctx, st.ID, req)
					took := time.Since(t0)
					mu.Lock()
					samples = append(samples, sample{t0, took, err})
					mu.Unlock()
				}
			}()
		}
		time.Sleep(2 * time.Second)
		before := b.metrics()
		req := step.req()
		benchSetWait(req, false)
		t0 := time.Now()
		rev, err := e.Write(ctx, st.ID, req)
		require.NoError(t, err)
		committed := time.Since(t0)
		benchAwaitRevision(t, e, st.ID, rev)
		applied := time.Since(t0)
		time.Sleep(2 * time.Second)
		close(stop)
		wg.Wait()
		after := b.metrics()
		var queued, idle []time.Duration
		failed := map[string]int{}
		for _, s := range samples {
			name := ""
			switch {
			case s.start.Before(t0):
				name = "before"
				idle = append(idle, s.took)
			case s.start.Before(t0.Add(applied)):
				name = "queued"
				queued = append(queued, s.took)
			default:
				continue
			}
			switch {
			case s.err == nil:
			case benchNotApplied(s.err):
				failed[name+"_timed_out"]++
			default:
				failed[name+"_errors"]++
			}
		}
		ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
		r.Extra[step.name+"_commit_ms"] = ms(committed)
		r.Extra[step.name+"_applied_ms"] = ms(applied)
		r.Extra[step.name+"_reach_rows"] = float64(after.reachChanged - before.reachChanged)
		for name, ds := range map[string][]time.Duration{"before": idle, "queued": queued} {
			p50, p99, maxMs := benchQuantiles(ds)
			r.Extra[step.name+"_writes_"+name+"_n"] = float64(len(ds))
			r.Extra[step.name+"_writes_"+name+"_p50_ms"] = p50
			r.Extra[step.name+"_writes_"+name+"_p99_ms"] = p99
			r.Extra[step.name+"_writes_"+name+"_max_ms"] = maxMs
			r.Extra[step.name+"_writes_"+name+"_timed_out_n"] = float64(failed[name+"_timed_out"])
			r.Extra[step.name+"_writes_"+name+"_errors_n"] = float64(failed[name+"_errors"])
		}
	}
	return r
}

// modelSwap writes a model in which org owners are org members, which changes
// the readers of every repo and so the reach rows of most sets, while 16
// writers and 64 checkers with the default consistency run on the first store.
// It reports how long until the model went live, the writes and checks while
// its view was built, the applier lag, and the peak Go heap, sampled every
// 50ms. It writes the benchmark's model back afterwards.
func (b *applierBench) modelSwap(write, check func(ctx context.Context, w, i int) error) benchResult {
	t, ctx, e := b.t, b.ctx, b.e
	r := b.base
	r.Workload = "modelswap"
	r.Load = benchLoadAvg()
	r.Extra = map[string]float64{}
	org := "define member: [user, team#member]\n    define owner: [user]"
	require.Contains(t, benchLadderModel, org)
	swapped, err := ParseDSL(strings.Replace(benchLadderModel, org, "define member: [user, team#member] or owner\n    define owner: [user]", 1))
	require.NoError(t, err)
	base, err := ParseDSL(benchLadderModel)
	require.NoError(t, err)
	storeID := b.stores[0]

	var mu sync.Mutex
	var writes, checks []time.Duration
	errs := 0
	stop := make(chan struct{})
	var wg sync.WaitGroup
	run := func(n int, fn func(ctx context.Context, w, i int) error, samples *[]time.Duration) {
		for w := range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var local []time.Duration
				failed := 0
				for i := 0; ; i++ {
					select {
					case <-stop:
						mu.Lock()
						*samples = append(*samples, local...)
						errs += failed
						mu.Unlock()
						return
					default:
					}
					t0 := time.Now()
					if err := fn(ctx, w, i); err != nil {
						failed++
					}
					local = append(local, time.Since(t0))
				}
			}()
		}
	}
	run(16, write, &writes)
	run(64, check, &checks)
	time.Sleep(time.Second)

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	heapBefore := ms.HeapAlloc
	heapPeak := heapBefore
	before := b.metrics()
	mu.Lock()
	writes, checks = writes[:0], checks[:0]
	mu.Unlock()
	swapDiag := b.diag("modelswap", storeID)
	cycles := b.diagBuild()
	start := time.Now()
	_, err = e.WriteAuthorizationModel(ctx, storeID, swapped)
	require.NoError(t, err)
	for {
		runtime.ReadMemStats(&ms)
		heapPeak = max(heapPeak, ms.HeapAlloc)
		status, err := e.ViewStatus(ctx, storeID)
		require.NoError(t, err)
		if status.Migration == nil {
			break
		}
		require.Less(t, time.Since(start), time.Hour, "the model did not go live within an hour")
		time.Sleep(50 * time.Millisecond)
	}
	flip := time.Since(start)
	close(stop)
	wg.Wait()
	swapDiag.finish(&r)
	cycles.finish(&r)
	b.metrics().since(before, &r)

	ws, cs := &benchStats{samples: writes, elapsed: flip}, &benchStats{samples: checks, elapsed: flip}
	r.Ops, r.P50, r.P99, r.Max = ws.summary()
	r.CheckOps, r.CheckP50, r.CheckP99, r.CheckMax = cs.summary()
	r.N, r.Errors = len(writes), errs
	r.Extra["flip_ms"] = float64(flip) / float64(time.Millisecond)
	r.Extra["writes_per_s"], r.Extra["write_p99_ms"] = r.Ops, r.P99
	r.Extra["checks_per_s"], r.Extra["check_p99_ms"] = r.CheckOps, r.CheckP99
	r.Extra["lag_p99_ms"], r.Extra["errors"] = r.LagP99, float64(r.Errors)
	r.Extra["heap_before_mb"] = float64(heapBefore) / (1 << 20)
	r.Extra["heap_peak_mb"] = float64(heapPeak) / (1 << 20)

	_, err = e.WriteAuthorizationModel(ctx, storeID, base)
	require.NoError(t, err)
	benchAwait(t, e, storeID)
	return r
}

func benchQuantiles(ds []time.Duration) (p50, p99, maxMs float64) {
	if len(ds) == 0 {
		return 0, 0, 0
	}
	slices.Sort(ds)
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	n := len(ds)
	return ms(ds[n/2]), ms(ds[min(n-1, n*99/100)]), ms(ds[n-1])
}

func benchLoadAvg() string {
	load, _ := os.ReadFile("/proc/loadavg")
	f := strings.Fields(string(load))
	if len(f) < 3 {
		return ""
	}
	return strings.Join(f[:3], " ")
}

// benchMetrics is a snapshot of the engine's cumulative metrics.
type benchMetrics struct {
	fallbacks    map[string]int64
	lagBounds    []float64
	lagCounts    []uint64
	batchBounds  []float64
	batchCounts  []uint64
	batchSum     int64
	batchN       uint64
	reachChanged int64
}

func (b *applierBench) metrics() benchMetrics {
	var rm metricdata.ResourceMetrics
	require.NoError(b.t, b.reader.Collect(b.ctx, &rm))
	out := benchMetrics{fallbacks: map[string]int64{}}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			switch data := m.Data.(type) {
			case metricdata.Sum[int64]:
				if m.Name != "gebra.view.fallbacks" {
					continue
				}
				for _, dp := range data.DataPoints {
					reason, _ := dp.Attributes.Value(attribute.Key("gebra.view.fallback"))
					out.fallbacks[reason.AsString()] += dp.Value
				}
			case metricdata.Histogram[float64]:
				if m.Name != "gebra.applier.lag" {
					continue
				}
				for _, dp := range data.DataPoints {
					out.lagBounds = dp.Bounds
					out.lagCounts = addCounts(out.lagCounts, dp.BucketCounts)
				}
			case metricdata.Histogram[int64]:
				for _, dp := range data.DataPoints {
					switch m.Name {
					case "gebra.applier.batch.writes":
						out.batchBounds = dp.Bounds
						out.batchCounts = addCounts(out.batchCounts, dp.BucketCounts)
						out.batchSum += dp.Sum
						out.batchN += dp.Count
					case "gebra.view.reach.changed_per_change":
						out.reachChanged += dp.Sum
					}
				}
			}
		}
	}
	return out
}

func addCounts(acc, add []uint64) []uint64 {
	if acc == nil {
		acc = make([]uint64, len(add))
	}
	for i, c := range add {
		acc[i] += c
	}
	return acc
}

// since adds what happened between two snapshots to r.
func (m benchMetrics) since(before benchMetrics, r *benchResult) {
	for reason, n := range m.fallbacks {
		if d := n - before.fallbacks[reason]; d > 0 {
			if r.Fallbacks == nil {
				r.Fallbacks = map[string]int64{}
			}
			r.Fallbacks[reason] = d
		}
	}
	delta := func(after, before []uint64) []uint64 {
		out := slices.Clone(after)
		for i := range before {
			if i < len(out) {
				out[i] -= before[i]
			}
		}
		return out
	}
	if m.lagCounts != nil {
		lag := delta(m.lagCounts, before.lagCounts)
		r.LagP50 = aboveTopBucket(1000 * bucketQuantile(m.lagBounds, lag, 0.5))
		r.LagP99 = aboveTopBucket(1000 * bucketQuantile(m.lagBounds, lag, 0.99))
		r.LagMax = aboveTopBucket(1000 * bucketQuantile(m.lagBounds, lag, 1))
	}
	if n := m.batchN - before.batchN; n > 0 {
		r.Batches = int64(n)
		r.BatchAvg = float64(m.batchSum-before.batchSum) / float64(n)
		r.BatchP99 = aboveTopBucket(bucketQuantile(m.batchBounds, delta(m.batchCounts, before.batchCounts), 0.99))
	}
}

// aboveTopBucket returns -1 for a quantile above the top bucket, which JSON
// cannot encode as +Inf.
func aboveTopBucket(q float64) float64 {
	if math.IsInf(q, 1) {
		return -1
	}
	return q
}

// bucketQuantile returns the upper bound of the bucket that holds the
// quantile, or +Inf for the last bucket.
func bucketQuantile(bounds []float64, counts []uint64, q float64) float64 {
	var total uint64
	for _, c := range counts {
		total += c
	}
	if total == 0 {
		return 0
	}
	want := uint64(math.Ceil(q * float64(total)))
	var seen uint64
	for i, c := range counts {
		seen += c
		if seen >= want && c > 0 || seen >= want && i == len(counts)-1 {
			if i < len(bounds) {
				return bounds[i]
			}
			return math.Inf(1)
		}
	}
	return math.Inf(1)
}

// benchStats is the outcome of a closed loop.
type benchStats struct {
	samples     []time.Duration
	elapsed     time.Duration
	errs        int
	lastErr     error
	lockSamples int
	lockWaits   map[string]int
	done        sync.WaitGroup
}

func (s *benchStats) wait() { s.done.Wait() }

func (s *benchStats) summary() (ops, p50, p99, maxMs float64) {
	if len(s.samples) == 0 {
		return 0, 0, 0, 0
	}
	p50, p99, maxMs = benchQuantiles(s.samples)
	return float64(len(s.samples)) / s.elapsed.Seconds(), p50, p99, maxMs
}

// loop runs fn on workers goroutines for the duration in the background.
// With sampleLocks, it samples pg_locks every 100ms for backends of the
// benchmark database that wait for a lock.
func (b *applierBench) loop(workers int, d time.Duration, fn func(ctx context.Context, w, i int) error, sampleLocks bool) *benchStats {
	s := &benchStats{lockWaits: map[string]int{}}
	s.done.Add(1)
	var mu sync.Mutex
	stop := make(chan struct{})
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		if !sampleLocks {
			return
		}
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			var waits []struct {
				What string
				N    int
			}
			rows, err := b.admin.QueryContext(b.ctx, `SELECT l.locktype || ' ' || coalesce(l.relation::regclass::text, ''), count(*)
				FROM pg_locks l JOIN pg_stat_activity a ON a.pid = l.pid
				WHERE NOT l.granted AND a.datname = $1 GROUP BY 1`, b.dbName)
			if err != nil {
				continue
			}
			for rows.Next() {
				var w struct {
					What string
					N    int
				}
				if rows.Scan(&w.What, &w.N) == nil {
					waits = append(waits, w)
				}
			}
			if rows.Err() != nil {
				waits = nil
			}
			_ = rows.Close()
			mu.Lock()
			s.lockSamples++
			for _, w := range waits {
				s.lockWaits[w.What] += w.N
			}
			mu.Unlock()
		}
	}()
	start := time.Now()
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []time.Duration
			for i := 0; time.Since(start) < d; i++ {
				ctx, cancel := context.WithTimeout(b.ctx, 2*time.Minute)
				t0 := time.Now()
				err := fn(ctx, w, i)
				local = append(local, time.Since(t0))
				cancel()
				if err != nil {
					mu.Lock()
					s.errs++
					s.lastErr = err
					mu.Unlock()
				}
			}
			mu.Lock()
			s.samples = append(s.samples, local...)
			mu.Unlock()
		}()
	}
	go func() {
		wg.Wait()
		s.elapsed = time.Since(start)
		close(stop)
		<-sampled
		s.done.Done()
	}()
	return s
}

func benchKey(user, relation, object string) TupleKey {
	return TupleKey{User: user, Relation: relation, Object: object}
}

// benchWriteKey returns a random tuple of the kinds that the benchmark
// writes, which is not in the dataset.
func benchWriteKey(rng *rand.Rand, dataset map[TupleKey]bool, users, teams, repos int) TupleKey {
	for {
		var k TupleKey
		switch rng.IntN(4) {
		case 0:
			k = benchKey(fmt.Sprintf("user:u%07d", rng.IntN(users)), "member", fmt.Sprintf("team:t%06d", rng.IntN(teams)))
		case 1:
			k = benchKey(fmt.Sprintf("team:t%06d#member", rng.IntN(teams)), "reader", fmt.Sprintf("repo:r%08d", rng.IntN(repos)))
		default:
			k = benchKey(fmt.Sprintf("user:u%07d", rng.IntN(users)), "reader", fmt.Sprintf("repo:r%08d", rng.IntN(repos)))
		}
		if !dataset[k] {
			return k
		}
	}
}

// benchChecks returns the benchmark's check queries.
func benchChecks(scale int) [][3]string {
	users, _, orgs, repos, _, _ := benchLadderDataset(scale)
	rng := rand.New(rand.NewPCG(uint64(scale), 1))
	rels := []string{"reader", "reader", "can_read", "writer", "admin"}
	var qs [][3]string
	for range 4096 {
		if rng.IntN(2) == 0 {
			o := rng.IntN(orgs)
			user := fmt.Sprintf("user:u%07d", (o*101+rng.IntN(50)*97)%users)
			r := o + orgs*rng.IntN(repos/orgs)
			qs = append(qs, [3]string{user, rels[rng.IntN(len(rels))], fmt.Sprintf("repo:r%08d", r)})
			continue
		}
		user := fmt.Sprintf("user:u%07d", rng.IntN(users))
		qs = append(qs, [3]string{user, rels[rng.IntN(len(rels))], fmt.Sprintf("repo:r%08d", rng.IntN(repos))})
	}
	return qs
}

// createBenchDB creates the benchmark's database, as a copy of the database
// GEBRA_BENCH_TEMPLATE when set. A template is the database of an earlier run
// of the same package with the same dataset settings, kept with
// GEBRA_BENCH_KEEP, so that a run
// skips loading the dataset. Nothing may be connected to the template.
func createBenchDB(t testing.TB, admin *sql.DB, name string) {
	t.Helper()
	q := fmt.Sprintf("CREATE DATABASE %q", name)
	if tpl := os.Getenv("GEBRA_BENCH_TEMPLATE"); tpl != "" {
		q += fmt.Sprintf(" TEMPLATE %q", tpl)
	}
	_, err := admin.Exec(q)
	require.NoError(t, err)
}

// benchStores is how many stores the benchmark loads, GEBRA_BENCH_STORES or
// 4. Only workloads with several writers use more than the first.
func benchStores() int {
	n, err := strconv.Atoi(benchEnv("GEBRA_BENCH_STORES", "4"))
	if err != nil || n < 1 {
		panic("invalid GEBRA_BENCH_STORES")
	}
	return n
}

// benchLadderDataset generates a GitHub-like dataset at scale s without
// duplicates, about 45k tuples per unit of s.
func benchLadderDataset(s int) (users, teams, orgs, repos int, gen func(emit func(TupleKey)), attrs func(emit func(object, name string, v any))) {
	users, teams, orgs, repos = 5000*s, 500*s, 20*s, 10_000*s
	// GEBRA_BENCH_USERS, GEBRA_BENCH_TEAMS, GEBRA_BENCH_ORGS and
	// GEBRA_BENCH_REPOS override the sizes that the scale gives.
	for _, d := range []struct {
		env string
		n   *int
	}{{"GEBRA_BENCH_USERS", &users}, {"GEBRA_BENCH_TEAMS", &teams}, {"GEBRA_BENCH_ORGS", &orgs}, {"GEBRA_BENCH_REPOS", &repos}} {
		if v := os.Getenv(d.env); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				panic(fmt.Sprintf("invalid %s %q", d.env, v))
			}
			*d.n = n
		}
	}
	u := func(i int) string { return fmt.Sprintf("user:u%07d", i%users) }
	tm := func(i int) string { return fmt.Sprintf("team:t%06d", i%teams) }
	og := func(i int) string { return fmt.Sprintf("org:o%05d", i%orgs) }
	rp := func(i int) string { return fmt.Sprintf("repo:r%08d", i) }
	orgMember := func(o, k int) int { return o*101 + k*97 }
	gen = func(emit func(TupleKey)) {
		for t := range teams {
			for k := range 8 {
				emit(benchKey(u(t*37+k*997), "member", tm(t)))
			}
			if t >= 100 {
				emit(benchKey(tm(t)+"#member", "member", tm(int(uint32(t*2654435761)%uint32(t)))))
			}
		}
		for i := range users {
			emit(benchKey(u(i), "member", "team:everyone"))
		}
		for o := range orgs {
			for k := range 50 {
				emit(benchKey(u(orgMember(o, k)), "member", og(o)))
			}
			emit(benchKey(u(o*53+11), "owner", og(o)))
			emit(benchKey(u(o*53+12), "owner", og(o)))
		}
		for r := range repos {
			emit(benchKey(og(r), "owner_org", rp(r)))
			emit(benchKey(u(r*13+2), "reader", rp(r)))
			if r%5 == 0 {
				emit(benchKey(u(r*31), "admin", rp(r)))
			}
			if r%10 == 0 {
				emit(benchKey(tm(r*3+5)+"#member", "admin", rp(r)))
				emit(benchKey(u(orgMember(r%orgs, (r/orgs)%50)), "blocked", rp(r)))
				if bl := u(r*19 + 7); bl != u(orgMember(r%orgs, (r/orgs)%50)) {
					emit(benchKey(bl, "blocked", rp(r)))
				}
			}
			if r%10 < 3 {
				emit(benchKey(u(r*17+1), "writer", rp(r)))
				emit(benchKey(tm(r*11+3)+"#member", "writer", rp(r)))
			}
			if r%5 < 2 {
				emit(benchKey(tm(r*7)+"#member", "reader", rp(r)))
			}
			if r%50 == 0 {
				emit(benchKey("user:*", "reader", rp(r)))
			}
		}
	}
	attrs = func(emit func(object, name string, v any)) {
		base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
		for r := range repos {
			emit(rp(r), "name", fmt.Sprintf("%08x", uint32(r*2654435761)))
			emit(rp(r), "updated_at", base.Add(time.Duration(uint32(r*40503))*time.Second).Format(time.RFC3339))
		}
	}
	return users, teams, orgs, repos, gen, attrs
}

const benchLadderModel = `model
  schema 1.1
type user
type team
  relations
    define member: [user, team#member]
type org
  relations
    define member: [user, team#member]
    define owner: [user]
type repo
  attributes
    name: string
    updated_at: time
  relations
    define owner_org: [org]
    define admin: [user, team#member] or owner from owner_org
    define writer: [user, team#member] or admin
    define reader: [user, user:*, team#member] or writer or member from owner_org
    define blocked: [user]
    define can_read: reader but not blocked
`
