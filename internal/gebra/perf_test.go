package gebra

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbx"
)

// The performance suite measures hard query shapes and a scale ladder on
// Postgres, in process and over HTTP:
//
//	GEBRA_PERF=depth,width,rewrites,lists,writes,ladder GEBRA_PERF_OUT=perf.md \
//	GOGS_DATABASE_TYPE=postgres PGHOST=... go test -run TestPerf -v -timeout 10h ./internal/gebra/
//
// GEBRA_PERF_LADDER lists the scale factors of the ladder (1 is about 45k
// tuples), GEBRA_PERF_SIZES overrides the sizes of the other workloads, and
// GEBRA_BIN names a cmd/gebra binary to measure over HTTP. Each workload uses
// a fresh database, which is dropped afterwards unless GEBRA_PERF_KEEP is
// set.

type perfEnv struct {
	t      *testing.T
	ctx    context.Context
	db     *sql.DB
	e      *Engine
	dbName string
	dbOID  int64
	out    *os.File
}

func newPerfEnv(t *testing.T, label string) *perfEnv {
	t.Helper()
	if os.Getenv("GOGS_DATABASE_TYPE") != "postgres" {
		t.Skip("the performance suite runs on Postgres")
	}
	ctx := context.Background()
	opts := conf.DatabaseOpts{
		Type: "postgres", Host: os.ExpandEnv("$PGHOST:$PGPORT"), Schema: "public",
		User: os.Getenv("PGUSER"), Password: os.Getenv("PGPASSWORD"), SSLMode: os.Getenv("PGSSLMODE"),
	}
	adminDSN, err := dbx.NewDSN(opts)
	require.NoError(t, err)
	admin, err := sql.Open("pgx", adminDSN)
	require.NoError(t, err)
	name := fmt.Sprintf("gebra_perf_%s_%d", label, time.Now().Unix())
	_, err = admin.Exec(fmt.Sprintf("CREATE DATABASE %q", name))
	require.NoError(t, err)
	// GEBRA_PERF_AUTO_EXPLAIN logs the plans of statements slower than the
	// given duration to the server log, e.g., 10s, and the parameters of
	// statements that fail, e.g., when a timeout cancels them.
	if d := os.Getenv("GEBRA_PERF_AUTO_EXPLAIN"); d != "" {
		for _, set := range []string{
			"session_preload_libraries = 'auto_explain'",
			fmt.Sprintf("auto_explain.log_min_duration = %q", d),
			"log_parameter_max_length_on_error = -1",
		} {
			_, err = admin.Exec(fmt.Sprintf("ALTER DATABASE %q SET %s", name, set))
			require.NoError(t, err)
		}
	}
	// GEBRA_PERF_DB_SET applies settings to the database, e.g., "jit = off".
	for _, set := range strings.Split(os.Getenv("GEBRA_PERF_DB_SET"), ";") {
		if strings.TrimSpace(set) != "" {
			_, err = admin.Exec(fmt.Sprintf("ALTER DATABASE %q SET %s", name, set))
			require.NoError(t, err)
		}
	}
	opts.Name = name
	dsn, err := dbx.NewDSN(opts)
	require.NoError(t, err)
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	db.SetMaxOpenConns(130)
	db.SetMaxIdleConns(130)
	_, err = db.ExecContext(ctx, "CREATE EXTENSION IF NOT EXISTS pg_stat_statements")
	require.NoError(t, err)
	e, err := New(ctx, logx.NewNoopLogger(), db, DialectPostgres, Options{})
	require.NoError(t, err)
	p := &perfEnv{t: t, ctx: ctx, db: db, e: e, dbName: name}
	_, err = e.db.handle().queryRow(ctx, "SELECT oid FROM pg_database WHERE datname = ?", []any{name}, &p.dbOID)
	require.NoError(t, err)
	if path := os.Getenv("GEBRA_PERF_OUT"); path != "" {
		p.out, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		e.Close()
		_ = db.Close()
		if os.Getenv("GEBRA_PERF_KEEP") == "" {
			_, _ = admin.Exec(fmt.Sprintf("DROP DATABASE %q", name))
		}
		_ = admin.Close()
		if p.out != nil {
			_ = p.out.Close()
		}
	})
	return p
}

// logf writes a line to the test log and to the Markdown output.
func (p *perfEnv) logf(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	p.t.Log(line)
	if p.out != nil {
		_, _ = fmt.Fprintln(p.out, line)
	}
}

// environment records the load average and other processes that may compete
// for the machine.
func (p *perfEnv) environment(what string) {
	load, _ := os.ReadFile("/proc/loadavg")
	out, _ := exec.Command("ps", "-eo", "pid,pcpu,rss,args").Output()
	var others []string
	self := strconv.Itoa(os.Getpid())
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) < 4 || f[0] == self {
			continue
		}
		cmd := strings.Join(f[3:], " ")
		if strings.Contains(cmd, ".test") || strings.Contains(cmd, "go test") || strings.Contains(cmd, "gebra -addr") || strings.Contains(cmd, "openfga run") {
			if len(cmd) > 70 {
				cmd = cmd[:70]
			}
			others = append(others, fmt.Sprintf("pid %s cpu %s%% (%s)", f[0], f[1], cmd))
		}
	}
	p.logf("\n<!-- %s at %s: load %s; other processes: %s -->", what, time.Now().UTC().Format(time.RFC3339), strings.TrimSpace(string(load)), strings.Join(others, "; "))
}

// perfStore is a store loaded by bulk copy and a rebuild of the view.
type perfStore struct {
	id string
	// oldModel is an earlier copy of the latest model. Queries that name it
	// evaluate the graph.
	oldModel  string
	tuples    int
	seed      time.Duration
	rebuild   time.Duration
	rebuildMB float64
	counts    viewCounts
}

// load creates a store with the model, copies the generated tuples and
// attributes into it, and rebuilds the view.
func (p *perfEnv) load(dsl string, tuples func(emit func(TupleKey)), attrs func(emit func(object, name string, v any))) *perfStore {
	t := p.t
	t.Helper()
	ctx := p.ctx
	st, err := p.e.CreateStore(ctx, "perf")
	require.NoError(t, err)
	m, err := ParseDSL(dsl)
	require.NoError(t, err)
	s := &perfStore{id: st.ID}
	s.oldModel, err = p.e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	awaitAppliedWithin(t, p.e, st.ID, time.Hour)
	cm, err := p.e.model(ctx, p.e.db.handle(), st.ID, s.oldModel)
	require.NoError(t, err)

	start := time.Now()
	now := time.Now().UTC()
	rows := make(chan []any, 10_000)
	go func() {
		defer close(rows)
		tuples(func(k TupleKey) {
			tp, err := parseTupleKey(k)
			if err == nil {
				err = cm.validateTuple(tp)
			}
			if err != nil {
				panic(fmt.Sprintf("invalid generated tuple %+v: %v", k, err))
			}
			cctx, _ := tp.contextJSON()
			rows <- []any{st.ID, tp.objectType, tp.objectID, tp.relation, tp.userType, tp.userID, tp.userRelation, tp.conditionName, cctx, now}
		})
	}()
	s.tuples = p.copyRows("gebra_tuple", []string{"store_id", "object_type", "object_id", "relation", "user_type", "user_id", "user_relation", "condition_name", "condition_context", "created_at"}, rows)
	if attrs != nil {
		arows := make(chan []any, 10_000)
		go func() {
			defer close(arows)
			attrs(func(object, name string, v any) {
				obj, _ := parseObject(object)
				enc, err := encodeAttribute(cm.types[obj.typ].attributes[name], v)
				if err != nil {
					panic(err)
				}
				arows <- []any{st.ID, obj.typ, name, obj.id, enc}
			})
		}()
		p.copyRows("gebra_object_attribute", []string{"store_id", "object_type", "name", "object_id", "value"}, arows)
	}
	_, err = p.db.ExecContext(ctx, "ANALYZE gebra_tuple; ANALYZE gebra_object_attribute")
	require.NoError(t, err)
	s.seed = time.Since(start)

	before := goRSS()
	start = time.Now()
	_, err = p.e.WriteAuthorizationModel(ctx, st.ID, m)
	require.NoError(t, err)
	awaitAppliedWithin(t, p.e, st.ID, time.Hour)
	s.rebuild = time.Since(start)
	s.rebuildMB = float64(goHWM()-before) / 1024
	_, err = p.db.ExecContext(ctx, "ANALYZE gebra_view_member; ANALYZE gebra_view_edge; ANALYZE gebra_view_reach")
	require.NoError(t, err)
	vs := servingState(t, p.e.db.handle(), st.ID)
	s.counts = viewCounts{member: vs.MemberRows, edge: vs.EdgeRows, reach: vs.ReachRows}
	return s
}

// copyRows copies rows into a table with COPY and returns how many it copied.
func (p *perfEnv) copyRows(table string, cols []string, rows <-chan []any) int {
	conn, err := p.db.Conn(p.ctx)
	require.NoError(p.t, err)
	defer func() { _ = conn.Close() }()
	var n int64
	err = conn.Raw(func(dc any) error {
		pc := dc.(*stdlib.Conn).Conn()
		var err error
		n, err = pc.CopyFrom(p.ctx, pgx.Identifier{table}, cols, &chanSource{ch: rows})
		return err
	})
	// Drain the generator so that it does not block forever after a failure.
	for range rows {
	}
	require.NoError(p.t, err)
	return int(n)
}

type chanSource struct {
	ch  <-chan []any
	cur []any
}

func (c *chanSource) Next() bool {
	var ok bool
	c.cur, ok = <-c.ch
	return ok
}
func (c *chanSource) Values() ([]any, error) { return c.cur, nil }
func (c *chanSource) Err() error             { return nil }

func procStatus(field string) int64 {
	f, err := os.Open("/proc/self/status")
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), field+":"); ok {
			n, _ := strconv.ParseInt(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 10, 64)
			return n
		}
	}
	return 0
}

// goRSS and goHWM return the resident and peak resident memory of the test
// process in kB. Writing 5 to clear_refs resets the peak.
func goRSS() int64 { return procStatus("VmRSS") }
func goHWM() int64 { return procStatus("VmHWM") }
func resetHWM()    { _ = os.WriteFile("/proc/self/clear_refs", []byte("5"), 0) }

// dbExecTime returns the total statement execution time of the database so
// far, from pg_stat_statements. It leaves out the measurement queries, whose
// own time would otherwise inflate the share of short operations.
func (p *perfEnv) dbExecTime() time.Duration {
	var ms float64
	_, _ = p.e.db.handle().queryRow(p.ctx, "SELECT COALESCE(SUM(total_exec_time), 0) FROM pg_stat_statements WHERE dbid = ? AND query NOT LIKE '%pg_stat%'", []any{p.dbOID}, &ms)
	return time.Duration(ms * float64(time.Millisecond))
}

type latencies struct {
	n, errs        int
	p50, p99, max  time.Duration
	total, dbTotal time.Duration
	lastErr        error
}

func (l latencies) String() string {
	if l.n == 0 {
		return "no samples"
	}
	db := ""
	switch {
	case l.dbTotal < 0:
		// pg_stat_statements evicted entries during the run. Postgres 15 keeps
		// one entry per length of an IN list, so the 5000 entries fill up.
		db = ", DB exec n/a"
	case l.total > 0:
		db = fmt.Sprintf(", DB exec %.0f%%", 100*float64(l.dbTotal)/float64(l.total))
	}
	e := ""
	if l.errs > 0 {
		e = fmt.Sprintf(", %d errors (%v)", l.errs, l.lastErr)
	}
	return fmt.Sprintf("p50 %s, p99 %s, max %s (n=%d%s%s)", fmtDur(l.p50), fmtDur(l.p99), fmtDur(l.max), l.n, db, e)
}

func fmtDur(d time.Duration) string {
	switch {
	case d >= time.Second:
		return fmt.Sprintf("%.2fs", d.Seconds())
	case d >= time.Millisecond:
		return fmt.Sprintf("%.1fms", float64(d)/float64(time.Millisecond))
	}
	return fmt.Sprintf("%.0fµs", float64(d)/float64(time.Microsecond))
}

func summarize(samples []time.Duration) latencies {
	slices.Sort(samples)
	l := latencies{n: len(samples)}
	if len(samples) == 0 {
		return l
	}
	l.p50 = samples[len(samples)/2]
	l.p99 = samples[min(len(samples)-1, len(samples)*99/100)]
	l.max = samples[len(samples)-1]
	for _, s := range samples {
		l.total += s
	}
	return l
}

// opTimeout bounds a single measured operation.
const opTimeout = 120 * time.Second

// seq runs fn n times one after another and returns the latencies, with the
// share of time the database spent executing statements.
func (p *perfEnv) seq(n int, fn func(ctx context.Context, i int) error) latencies {
	var samples []time.Duration
	var errs int
	var lastErr error
	db0 := p.dbExecTime()
	for i := range n {
		ctx, cancel := context.WithTimeout(p.ctx, opTimeout)
		start := time.Now()
		err := fn(ctx, i)
		samples = append(samples, time.Since(start))
		cancel()
		if err != nil {
			errs++
			lastErr = err
		}
	}
	l := summarize(samples)
	l.dbTotal = p.dbExecTime() - db0
	l.errs, l.lastErr = errs, lastErr
	return l
}

// sysSample is resource use over an interval.
type sysSample struct {
	cpuBusy           float64 // share of all CPUs busy
	goCPU, pgCPU      float64 // CPUs used by this process and by the database's backends
	readMBs, writeMBs float64
	maxActive         int
	waits             map[string]int
	goRSSMB, pgRSSMB  float64
	pgBackends        int
}

func (s sysSample) String() string {
	var waits []string
	for k, v := range s.waits {
		waits = append(waits, fmt.Sprintf("%s=%d", k, v))
	}
	slices.Sort(waits)
	return fmt.Sprintf("machine CPU %.0f%%, Go %.1f CPUs, Postgres backends %.1f CPUs (%d conns, max %d active), disk read %.1f MB/s write %.1f MB/s, Go RSS %.0f MB, Postgres RSS %.0f MB, waits [%s]",
		100*s.cpuBusy, s.goCPU, s.pgCPU, s.pgBackends, s.maxActive, s.readMBs, s.writeMBs, s.goRSSMB, s.pgRSSMB, strings.Join(waits, " "))
}

func readCPU() (busy, total float64) {
	b, _ := os.ReadFile("/proc/stat")
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	for i, v := range f[1:] {
		n, _ := strconv.ParseFloat(v, 64)
		total += n
		if i != 3 && i != 4 { // idle, iowait
			busy += n
		}
	}
	return busy, total
}

func procCPU(pid int) float64 {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(b)
	f := strings.Fields(s[strings.LastIndexByte(s, ')')+2:])
	u, _ := strconv.ParseFloat(f[11], 64)
	k, _ := strconv.ParseFloat(f[12], 64)
	return (u + k) / 100 // clock ticks
}

func diskSectors() (read, written float64) {
	b, _ := os.ReadFile("/proc/diskstats")
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 9 && (f[2] == "vdb" || f[2] == "vda" || f[2] == "nvme0n1" || f[2] == "sda") {
			r, _ := strconv.ParseFloat(f[5], 64)
			w, _ := strconv.ParseFloat(f[9], 64)
			read += r
			written += w
		}
	}
	return read, written
}

func (p *perfEnv) backendPIDs() []int {
	pids, _ := queryColumn[int](p.ctx, p.e.db.handle(), "SELECT pid FROM pg_stat_activity WHERE datname = ?", p.dbName)
	return pids
}

func postgresRSSMB() float64 {
	out, _ := exec.Command("ps", "-u", "postgres", "-o", "rss=").Output()
	var kb float64
	for _, l := range strings.Fields(string(out)) {
		n, _ := strconv.ParseFloat(l, 64)
		kb += n
	}
	return kb / 1024
}

// loop runs fn on workers goroutines for the duration and returns the
// throughput, the latencies, and the resources used.
func (p *perfEnv) loop(workers int, d time.Duration, fn func(ctx context.Context, w, i int) error) (float64, latencies, sysSample) {
	var mu sync.Mutex
	var samples []time.Duration
	var errs atomic.Int64
	var lastErr atomic.Value
	stop := make(chan struct{})
	var s sysSample
	s.waits = map[string]int{}
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		tick := time.NewTicker(500 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
			}
			rows, _ := queryStructs[struct {
				State, WaitEventType, WaitEvent string
			}](p.ctx, p.e.db.handle(), "SELECT COALESCE(state, '') AS state, COALESCE(wait_event_type, '') AS wait_event_type, COALESCE(wait_event, '') AS wait_event FROM pg_stat_activity WHERE datname = ? AND pid <> pg_backend_pid()", p.dbName)
			active := 0
			for _, r := range rows {
				if r.State == "active" {
					active++
					if r.WaitEventType != "" {
						s.waits[r.WaitEventType+"/"+r.WaitEvent]++
					}
				}
			}
			s.maxActive = max(s.maxActive, active)
		}
	}()
	pids := p.backendPIDs()
	pgCPU0 := 0.0
	for _, pid := range pids {
		pgCPU0 += procCPU(pid)
	}
	goCPU0 := procCPU(os.Getpid())
	busy0, total0 := readCPU()
	r0, w0 := diskSectors()
	db0 := p.dbExecTime()
	start := time.Now()
	var wg sync.WaitGroup
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []time.Duration
			for i := 0; time.Since(start) < d; i++ {
				ctx, cancel := context.WithTimeout(p.ctx, opTimeout)
				t0 := time.Now()
				err := fn(ctx, w, i)
				local = append(local, time.Since(t0))
				cancel()
				if err != nil {
					errs.Add(1)
					lastErr.Store(err)
				}
			}
			mu.Lock()
			samples = append(samples, local...)
			mu.Unlock()
		}()
	}
	wg.Wait()
	elapsed := time.Since(start)
	close(stop)
	<-sampled
	dbTime := p.dbExecTime() - db0
	busy1, total1 := readCPU()
	r1, w1 := diskSectors()
	pids = p.backendPIDs()
	pgCPU1 := 0.0
	for _, pid := range pids {
		pgCPU1 += procCPU(pid)
	}
	s.cpuBusy = (busy1 - busy0) / (total1 - total0)
	s.goCPU = (procCPU(os.Getpid()) - goCPU0) / elapsed.Seconds()
	// Backends that started during the run count from zero, which is what
	// they used.
	s.pgCPU = max(0, pgCPU1-pgCPU0) / elapsed.Seconds()
	s.pgBackends = len(pids)
	s.readMBs = (r1 - r0) * 512 / 1e6 / elapsed.Seconds()
	s.writeMBs = (w1 - w0) * 512 / 1e6 / elapsed.Seconds()
	s.goRSSMB = float64(goRSS()) / 1024
	s.pgRSSMB = postgresRSSMB()
	l := summarize(samples)
	l.dbTotal = dbTime
	l.errs = int(errs.Load())
	if v := lastErr.Load(); v != nil {
		l.lastErr = v.(error)
	}
	return float64(len(samples)) / elapsed.Seconds(), l, s
}

func (p *perfEnv) tableSizes() (string, int64) {
	rows, _ := queryStructs[struct {
		Name string
		Size int64
	}](p.ctx, p.e.db.handle(), `SELECT relname AS name, pg_total_relation_size(c.oid) AS size FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE n.nspname = 'public' AND c.relkind = 'r' AND relname LIKE 'gebra_%' ORDER BY size DESC`)
	var parts []string
	var total int64
	for _, r := range rows {
		total += r.Size
		if r.Size > 1<<20 {
			parts = append(parts, fmt.Sprintf("%s %.0f MB", strings.TrimPrefix(r.Name, "gebra_"), float64(r.Size)/1e6))
		}
	}
	return strings.Join(parts, ", "), total
}

func (p *perfEnv) storeLine(s *perfStore) {
	p.logf("- store: %d tuples; view member %d, edge %d, reach %d rows; copy+analyze %s, rebuild %s (Go peak +%.0f MB)",
		s.tuples, s.counts.member, s.counts.edge, s.counts.reach, fmtDur(s.seed), fmtDur(s.rebuild), s.rebuildMB)
}

func (p *perfEnv) check(ctx context.Context, s *perfStore, graph bool, user, rel, obj string, contextual []TupleKey, reqCtx map[string]any) error {
	req := &CheckRequest{TupleKey: CheckTupleKey{User: user, Relation: rel, Object: obj}, ContextualTuples: contextual, Context: reqCtx}
	if graph {
		req.AuthorizationModelID = s.oldModel
	}
	_, err := p.e.Check(ctx, s.id, req)
	return err
}

func (p *perfEnv) checkIs(s *perfStore, want bool, user, rel, obj string) {
	ok, err := p.e.Check(p.ctx, s.id, &CheckRequest{TupleKey: CheckTupleKey{User: user, Relation: rel, Object: obj}})
	require.NoError(p.t, err)
	require.Equal(p.t, want, ok, "check %s %s %s", user, rel, obj)
}

// measureCheck measures a check on the view and on the graph in alternating
// rounds.
func (p *perfEnv) measureCheck(label string, s *perfStore, n int, user, rel, obj string, contextual []TupleKey, reqCtx map[string]any) {
	for round := range 2 {
		for _, graph := range []bool{false, true} {
			path := "view"
			if graph {
				path = "graph"
			}
			if graph && contextual != nil {
				continue
			}
			l := p.seq(n, func(ctx context.Context, _ int) error {
				return p.check(ctx, s, graph, user, rel, obj, contextual, reqCtx)
			})
			p.logf("  - %s [%s, round %d]: %s", label, path, round+1, l)
		}
	}
}

func perfSizes(def []int) []int {
	v := os.Getenv("GEBRA_PERF_SIZES")
	if v == "" {
		return def
	}
	var out []int
	for _, s := range strings.Split(v, ",") {
		n, err := strconv.Atoi(s)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

func perfEnabled(t *testing.T, name string) {
	t.Helper()
	for _, w := range strings.Split(os.Getenv("GEBRA_PERF"), ",") {
		if w == name || w == "all" {
			return
		}
	}
	t.Skip("set GEBRA_PERF to include " + name)
}

func TestPerfDepth(t *testing.T) {
	perfEnabled(t, "depth")
	p := newPerfEnv(t, "depth")
	p.environment("depth")
	p.logf("\n### Depth\n")
	for _, d := range perfSizes([]int{5, 25, 100, 500}) {
		var dsl strings.Builder
		dsl.WriteString("model\n  schema 1.1\ntype user\ntype group\n  relations\n    define member: [user, group#member]\n")
		dsl.WriteString("type folder\n  relations\n    define parent: [folder]\n    define viewer: [user] or viewer from parent\n")
		dsl.WriteString("type doc\n  relations\n    define c0: [user]\n")
		for i := 1; i <= d; i++ {
			fmt.Fprintf(&dsl, "    define c%d: c%d\n", i, i-1)
		}
		compileStart := time.Now()
		m, err := ParseDSL(dsl.String())
		require.NoError(t, err)
		_, err = compileModel(m, DefaultConditionCostLimit)
		require.NoError(t, err)
		compile := time.Since(compileStart)
		s := p.load(dsl.String(), func(emit func(TupleKey)) {
			for i := range d {
				emit(tk(fmt.Sprintf("group:g%d#member", i+1), "member", fmt.Sprintf("group:g%d", i)))
				emit(tk(fmt.Sprintf("folder:f%d", i), "parent", fmt.Sprintf("folder:f%d", i+1)))
			}
			for i := 0; i <= d; i++ {
				emit(tk(fmt.Sprintf("user:u%d", i), "member", fmt.Sprintf("group:g%d", i)))
			}
			emit(tk("user:deep", "member", fmt.Sprintf("group:g%d", d)))
			emit(tk("user:top", "viewer", "folder:f0"))
			emit(tk("user:a", "c0", "doc:1"))
		}, nil)
		p.logf("\n#### Depth %d (model compile %s)\n", d, fmtDur(compile))
		p.storeLine(s)
		p.checkIs(s, true, "user:deep", "member", "group:g0")
		p.checkIs(s, true, "user:top", "viewer", fmt.Sprintf("folder:f%d", d))
		p.checkIs(s, true, "user:a", fmt.Sprintf("c%d", d), "doc:1")
		n := 50
		if d >= 500 {
			n = 10
		}
		p.measureCheck("group nesting, allowed", s, n, "user:deep", "member", "group:g0", nil, nil)
		p.measureCheck("group nesting, denied", s, n, "user:nobody", "member", "group:g0", nil, nil)
		p.measureCheck("folder TTU chain, allowed", s, n, "user:top", "viewer", fmt.Sprintf("folder:f%d", d), nil, nil)
		p.measureCheck("folder TTU chain, denied", s, n, "user:nobody", "viewer", fmt.Sprintf("folder:f%d", d), nil, nil)
		p.measureCheck("computed userset chain", s, n, "user:a", fmt.Sprintf("c%d", d), "doc:1", nil, nil)
		l := p.seq(n, func(ctx context.Context, _ int) error {
			_, _, err := p.e.ListObjects(ctx, s.id, &ListObjectsRequest{Type: "group", Relation: "member", User: "user:deep"})
			return err
		})
		p.logf("  - list groups of the deepest user (%d results): %s", d+1, l)
		l = p.seq(n, func(ctx context.Context, _ int) error {
			_, _, err := p.e.ListUsers(ctx, s.id, &ListUsersRequest{Object: "group:g0", Relation: "member", UserFilters: []UserFilter{{Type: "user"}}})
			return err
		})
		p.logf("  - list users of the top group (%d results): %s", d+2, l)
		l = p.seq(n, func(ctx context.Context, i int) error {
			k := tk(fmt.Sprintf("group:x%d#member", i), "member", fmt.Sprintf("group:g%d", d))
			if _, err := p.e.Write(ctx, s.id, &WriteRequest{Writes: []TupleKey{k}, Wait: true}); err != nil {
				return err
			}
			_, err := p.e.Write(ctx, s.id, &WriteRequest{Deletes: []TupleKey{k}, Wait: true})
			return err
		})
		p.logf("  - write and delete a group nested under the deepest group (reach rows %d each way): %s", d+1, l)
		mid := tk(fmt.Sprintf("group:g%d#member", d/2+1), "member", fmt.Sprintf("group:g%d", d/2))
		l = p.seq(3, func(ctx context.Context, _ int) error {
			if _, err := p.e.Write(ctx, s.id, &WriteRequest{Deletes: []TupleKey{mid}, Wait: true}); err != nil {
				return err
			}
			_, err := p.e.Write(ctx, s.id, &WriteRequest{Writes: []TupleKey{mid}, Wait: true})
			return err
		})
		p.logf("  - delete and restore the middle link (rederives about %d reach rows each way): %s", (d/2)*(d/2+1), l)
	}
}

func TestPerfWidth(t *testing.T) {
	perfEnabled(t, "width")
	p := newPerfEnv(t, "width")
	p.environment("width")
	p.logf("\n### Width\n")
	const dsl = `model
  schema 1.1
type user
  attributes
    name: string
type team
  relations
    define member: [user, team#member]
type doc
  relations
    define viewer: [user, user:*, team#member]
    define blocked: [user]
    define can_view: viewer but not blocked
`
	for _, n := range perfSizes([]int{10_000, 100_000, 1_000_000}) {
		multi := min(n/10, 10_000)
		direct := min(n, 100_000)
		s := p.load(dsl, func(emit func(TupleKey)) {
			for i := range n {
				emit(tk(fmt.Sprintf("user:w%07d", i), "member", "team:big"))
			}
			emit(tk("team:big#member", "viewer", "doc:shared"))
			emit(tk("user:*", "viewer", "doc:pub"))
			for i := range direct {
				emit(tk(fmt.Sprintf("user:w%07d", i), "viewer", "doc:direct"))
			}
			for i := range multi {
				emit(tk("user:multi", "member", fmt.Sprintf("team:m%05d", i)))
				emit(tk(fmt.Sprintf("team:m%05d#member", i), "viewer", fmt.Sprintf("doc:md%05d", i)))
			}
			emit(tk("user:w0000001", "blocked", "doc:shared"))
		}, func(emit func(object, name string, v any)) {
			for i := range n {
				emit(fmt.Sprintf("user:w%07d", i), "name", fmt.Sprintf("%x", uint32(i*2654435761)))
			}
		})
		p.logf("\n#### Team of %d members, one user in %d teams, a doc shared with %d users\n", n, multi, direct)
		p.storeLine(s)
		last := fmt.Sprintf("user:w%07d", n-1)
		p.checkIs(s, true, last, "viewer", "doc:shared")
		p.measureCheck("member of the big team", s, 20, last, "member", "team:big", nil, nil)
		p.measureCheck("viewer of a doc shared with the big team", s, 20, last, "viewer", "doc:shared", nil, nil)
		p.measureCheck("can_view (exclusion) of that doc", s, 20, last, "can_view", "doc:shared", nil, nil)
		p.measureCheck("non-member of the big team", s, 20, "user:nobody", "viewer", "doc:shared", nil, nil)
		p.measureCheck(fmt.Sprintf("viewer of a doc with %d direct viewers", direct), s, 20, fmt.Sprintf("user:w%07d", direct-1), "viewer", "doc:direct", nil, nil)
		p.measureCheck("wildcard grant", s, 20, "user:someone", "viewer", "doc:pub", nil, nil)
		p.measureCheck(fmt.Sprintf("user in %d teams, last team's doc", multi), s, 20, "user:multi", "viewer", fmt.Sprintf("doc:md%05d", multi-1), nil, nil)
		for _, q := range []struct {
			label string
			req   ListUsersRequest
		}{
			{"list users of the shared doc, first page of 100 by ID", ListUsersRequest{Object: "doc:shared", Relation: "viewer", PageSize: 100}},
			{"list users of the shared doc, first page of 100 by name", ListUsersRequest{Object: "doc:shared", Relation: "viewer", PageSize: 100, Sort: Sort{Attribute: "name"}}},
			{"list users of the shared doc, all", ListUsersRequest{Object: "doc:shared", Relation: "viewer"}},
			{"list users of the shared doc with can_view (verified), first page of 100", ListUsersRequest{Object: "doc:shared", Relation: "can_view", PageSize: 100}},
			{"list users of the direct doc, all", ListUsersRequest{Object: "doc:direct", Relation: "viewer"}},
		} {
			req := q.req
			req.UserFilters = []UserFilter{{Type: "user"}}
			var count int
			l := p.seq(3, func(ctx context.Context, _ int) error {
				users, _, err := p.e.ListUsers(ctx, s.id, &req)
				count = len(users)
				return err
			})
			p.logf("  - %s (%d results): %s", q.label, count, l)
		}
		for _, q := range []struct {
			label string
			req   ListObjectsRequest
		}{
			{fmt.Sprintf("list docs of the user in %d teams, all", multi), ListObjectsRequest{Type: "doc", Relation: "viewer", User: "user:multi"}},
			{fmt.Sprintf("list docs of the user in %d teams, first page of 100", multi), ListObjectsRequest{Type: "doc", Relation: "viewer", User: "user:multi", PageSize: 100}},
			{fmt.Sprintf("list docs of the user in %d teams, graph (older model)", multi), ListObjectsRequest{Type: "doc", Relation: "viewer", User: "user:multi", AuthorizationModelID: s.oldModel}},
			{"list docs of a member of the big team", ListObjectsRequest{Type: "doc", Relation: "viewer", User: last}},
		} {
			req := q.req
			var count int
			l := p.seq(3, func(ctx context.Context, _ int) error {
				objs, _, err := p.e.ListObjects(ctx, s.id, &req)
				count = len(objs)
				return err
			})
			p.logf("  - %s (%d results): %s", q.label, count, l)
		}
		l := p.seq(20, func(ctx context.Context, i int) error {
			k := tk(fmt.Sprintf("user:new%d", i), "member", "team:big")
			if _, err := p.e.Write(ctx, s.id, &WriteRequest{Writes: []TupleKey{k}, Wait: true}); err != nil {
				return err
			}
			_, err := p.e.Write(ctx, s.id, &WriteRequest{Deletes: []TupleKey{k}, Wait: true})
			return err
		})
		p.logf("  - add and remove a member of the big team: %s", l)
		l = p.seq(5, func(ctx context.Context, i int) error {
			k := tk("team:big#member", "viewer", fmt.Sprintf("doc:new%d", i))
			if _, err := p.e.Write(ctx, s.id, &WriteRequest{Writes: []TupleKey{k}, Wait: true}); err != nil {
				return err
			}
			_, err := p.e.Write(ctx, s.id, &WriteRequest{Deletes: []TupleKey{k}, Wait: true})
			return err
		})
		p.logf("  - share and unshare a doc with the big team: %s", l)
	}
}

func TestPerfRewrites(t *testing.T) {
	perfEnabled(t, "rewrites")
	p := newPerfEnv(t, "rewrites")
	p.environment("rewrites")
	p.logf("\n### Hard rewrites\n")
	const dsl = `model
  schema 1.1
type user
type group
  relations
    define banned: [user]
    define member: [user, group#member, group#allowed]
    define allowed: member but not banned
    define cmember: [user with ok, group#cmember with ok]
type folder
  relations
    define parent: [folder]
    define viewer: [user, group#member] or viewer from parent
    define blocked: [user, group#member] or blocked from parent
    define editor: [user, group#member] or editor from parent
type doc
  relations
    define parent: [folder]
    define viewer: [user, group#member] or viewer from parent
    define blocked: [user, group#member] or blocked from parent
    define editor: [user, group#member] or editor from parent
    define banned: [user]
    define can_view: viewer but not blocked
    define can_edit: (editor and viewer) but not blocked
    define x1: can_view but not banned
    define x2: (x1 and editor) but not banned
    define x3: (x2 or can_edit) but not blocked
    define cview: [user with ok, group#cmember with ok] or viewer
condition ok(n: int) {
  n > 0
}
`
	for _, d := range perfSizes([]int{25, 100, 500}) {
		s := p.load(dsl, func(emit func(TupleKey)) {
			for i := range d {
				emit(tk(fmt.Sprintf("group:g%d#member", i+1), "member", fmt.Sprintf("group:g%d", i)))
				emit(tk(fmt.Sprintf("group:a%d#allowed", i+1), "member", fmt.Sprintf("group:a%d", i)))
				emit(TupleKey{User: fmt.Sprintf("group:c%d#cmember", i+1), Relation: "cmember", Object: fmt.Sprintf("group:c%d", i), Condition: &RelationshipCondition{Name: "ok"}})
				emit(tk(fmt.Sprintf("folder:f%d", i), "parent", fmt.Sprintf("folder:f%d", i+1)))
			}
			emit(tk("group:a0#allowed", "member", fmt.Sprintf("group:a%d", d)))
			emit(tk("user:deep", "member", fmt.Sprintf("group:g%d", d)))
			emit(tk("user:ca", "member", fmt.Sprintf("group:a%d", d)))
			emit(tk("user:cb", "member", fmt.Sprintf("group:a%d", d)))
			emit(tk("user:cb", "banned", "group:a5"))
			emit(TupleKey{User: "user:deep", Relation: "cmember", Object: fmt.Sprintf("group:c%d", d), Condition: &RelationshipCondition{Name: "ok"}})
			emit(tk(fmt.Sprintf("folder:f%d", d), "parent", "doc:1"))
			emit(tk("group:g0#member", "viewer", "folder:f0"))
			emit(tk("group:g0#member", "editor", "folder:f0"))
			emit(tk("user:blk", "blocked", fmt.Sprintf("folder:f%d", d/2)))
			emit(TupleKey{User: "group:c0#cmember", Relation: "cview", Object: "doc:1", Condition: &RelationshipCondition{Name: "ok"}})
		}, nil)
		p.logf("\n#### Hard rewrites over depth %d\n", d)
		p.storeLine(s)
		p.checkIs(s, true, "user:deep", "x3", "doc:1")
		p.checkIs(s, true, "user:ca", "allowed", "group:a0")
		p.checkIs(s, false, "user:cb", "allowed", "group:a0")
		n := 20
		if d >= 500 {
			n = 5
		}
		irrelevant := []TupleKey{tk("user:zz", "banned", "doc:1")}
		for _, rel := range []string{"can_view", "can_edit", "x1", "x2", "x3"} {
			p.measureCheck(rel+" over deep groups and folders", s, n, "user:deep", rel, "doc:1", nil, nil)
			p.measureCheck(rel+" with a contextual tuple (graph on every hop)", s, n, "user:deep", rel, "doc:1", irrelevant, nil)
		}
		p.measureCheck("exclusion inside a cycle, allowed", s, n, "user:ca", "allowed", "group:a0", nil, nil)
		p.measureCheck("exclusion inside a cycle, banned on the way", s, n, "user:cb", "allowed", "group:a0", nil, nil)
		p.measureCheck("condition on every hop of a deep chain", s, n, "user:deep", "cmember", "group:c0", nil, map[string]any{"n": 1})
		p.measureCheck("conditional relation over the deep chain, from a doc", s, n, "user:deep", "cview", "doc:1", nil, map[string]any{"n": 1})
		for _, q := range []struct {
			label string
			req   ListObjectsRequest
		}{
			{"list groups allowed for a member of the cycle", ListObjectsRequest{Type: "group", Relation: "allowed", User: "user:ca"}},
			{"list docs with x3", ListObjectsRequest{Type: "doc", Relation: "x3", User: "user:deep"}},
			{"list groups with the conditional relation", ListObjectsRequest{Type: "group", Relation: "cmember", User: "user:deep", Context: map[string]any{"n": 1}}},
		} {
			req := q.req
			var count int
			l := p.seq(n, func(ctx context.Context, _ int) error {
				objs, _, err := p.e.ListObjects(ctx, s.id, &req)
				count = len(objs)
				return err
			})
			p.logf("  - %s (%d results): %s", q.label, count, l)
		}
		var count int
		l := p.seq(n, func(ctx context.Context, _ int) error {
			users, _, err := p.e.ListUsers(ctx, s.id, &ListUsersRequest{Object: "group:a0", Relation: "allowed", UserFilters: []UserFilter{{Type: "user"}}})
			count = len(users)
			return err
		})
		p.logf("  - list users allowed in the cycle (%d results): %s", count, l)
	}
}

const listsModel = `model
  schema 1.1
type user
type org
  relations
    define member: [user, team#member]
type team
  relations
    define member: [user]
type repo
  attributes
    name: string
  relations
    define owner_org: [org]
    define blocked: [user]
    define reader: [user, user:*, team#member] or member from owner_org
    define can_read: reader but not blocked
`

func TestPerfLists(t *testing.T) {
	perfEnabled(t, "lists")
	p := newPerfEnv(t, "lists")
	p.environment("lists")
	p.logf("\n### Lists\n")
	for _, k := range perfSizes([]int{1000, 100_000, 1_000_000}) {
		s := p.load(listsModel, func(emit func(TupleKey)) {
			for i := range k {
				r := fmt.Sprintf("repo:r%07d", i)
				emit(tk("org:o", "owner_org", r))
				if i%10 == 0 {
					emit(tk("user:l", "blocked", r))
				}
			}
			emit(tk("user:l", "member", "org:o"))
			for i := range 100_000 {
				emit(tk(fmt.Sprintf("user:h%06d", i), "member", "team:hot"))
			}
			emit(tk("team:hot#member", "reader", "repo:hot"))
		}, func(emit func(object, name string, v any)) {
			for i := range k {
				emit(fmt.Sprintf("repo:r%07d", i), "name", fmt.Sprintf("%08x", uint32(i*2654435761)))
			}
		})
		p.logf("\n#### A user who can read %d repos\n", k)
		p.storeLine(s)
		var mid cursorRow
		_, err := p.e.db.handle().queryRow(p.ctx, `SELECT value AS v, object_id AS id FROM gebra_object_attribute WHERE store_id = ? AND object_type = 'repo' AND name = 'name'
			ORDER BY value COLLATE "C", object_id COLLATE "C" OFFSET ? LIMIT 1`, []any{s.id, k / 2}, &mid.V, &mid.ID)
		require.NoError(t, err)
		midID := cursorRow{ID: fmt.Sprintf("r%07d", k/2)}
		for _, q := range []struct {
			label string
			req   ListObjectsRequest
		}{
			{"reader, by name, first page of 100", ListObjectsRequest{Relation: "reader", Sort: Sort{Attribute: "name"}, PageSize: 100}},
			{"reader, by name, page of 100 in the middle", ListObjectsRequest{Relation: "reader", Sort: Sort{Attribute: "name"}, PageSize: 100, ContinuationToken: mid.token()}},
			{"reader, by ID, page of 100 in the middle", ListObjectsRequest{Relation: "reader", PageSize: 100, ContinuationToken: midID.token()}},
			{"reader, by name descending, first page of 100", ListObjectsRequest{Relation: "reader", Sort: Sort{Attribute: "name", Descending: true}, PageSize: 100}},
			{"can_read (verified), by name, first page of 100", ListObjectsRequest{Relation: "can_read", Sort: Sort{Attribute: "name"}, PageSize: 100}},
			{"can_read (verified), by name, page of 100 in the middle", ListObjectsRequest{Relation: "can_read", Sort: Sort{Attribute: "name"}, PageSize: 100, ContinuationToken: mid.token()}},
			{"reader, all", ListObjectsRequest{Relation: "reader"}},
			{"can_read (verified), all", ListObjectsRequest{Relation: "can_read"}},
			{"reader, graph (older model), first page of 100", ListObjectsRequest{Relation: "reader", PageSize: 100, AuthorizationModelID: s.oldModel}},
		} {
			req := q.req
			req.Type, req.User = "repo", "user:l"
			var count int
			n := 5
			if req.PageSize == 0 {
				n = 2
			}
			l := p.seq(n, func(ctx context.Context, _ int) error {
				objs, _, err := p.e.ListObjects(ctx, s.id, &req)
				count = len(objs)
				return err
			})
			p.logf("  - list %s (%d results): %s", q.label, count, l)
		}
		var count int
		l := p.seq(2, func(ctx context.Context, _ int) error {
			count = 0
			return p.e.StreamListObjects(ctx, s.id, &ListObjectsRequest{Type: "repo", Relation: "reader", User: "user:l"}, func(string) error {
				count++
				return nil
			})
		})
		p.logf("  - streamed list of every readable repo (%d results): %s", count, l)
		for _, q := range []struct {
			label string
			req   ListUsersRequest
		}{
			{"list users of a repo read by 100k users, first page of 100", ListUsersRequest{Relation: "reader", PageSize: 100}},
			{"list users of a repo read by 100k users, all", ListUsersRequest{Relation: "reader"}},
			{"list users of that repo with can_read (verified), first page of 100", ListUsersRequest{Relation: "can_read", PageSize: 100}},
		} {
			req := q.req
			req.Object, req.UserFilters = "repo:hot", []UserFilter{{Type: "user"}}
			l := p.seq(3, func(ctx context.Context, _ int) error {
				users, _, err := p.e.ListUsers(ctx, s.id, &req)
				count = len(users)
				return err
			})
			p.logf("  - %s (%d results): %s", q.label, count, l)
		}
	}
}

func TestPerfWrites(t *testing.T) {
	perfEnabled(t, "writes")
	p := newPerfEnv(t, "writes")
	p.environment("writes")
	p.logf("\n### Writes\n")
	for _, sz := range perfSizes([]int{10_000, 100_000}) {
		repos := min(sz, 10_000)
		s := p.load(listsModel, func(emit func(TupleKey)) {
			for i := range sz {
				emit(tk(fmt.Sprintf("user:f%06d", i), "member", "team:fan"))
			}
			for i := range repos {
				emit(tk("org:fan", "owner_org", fmt.Sprintf("repo:f%05d", i)))
			}
		}, nil)
		p.logf("\n#### Fan-out: a team of %d members added to an org with %d repos\n", sz, repos)
		p.storeLine(s)
		link := tk("team:fan#member", "member", "org:fan")
		var adds, dels []time.Duration
		for range 3 {
			t0 := time.Now()
			_, err := p.e.Write(p.ctx, s.id, &WriteRequest{Writes: []TupleKey{link}, Wait: true})
			require.NoError(t, err)
			adds = append(adds, time.Since(t0))
			p.checkIs(s, true, "user:f000001", "reader", "repo:f00001")
			t0 = time.Now()
			_, err = p.e.Write(p.ctx, s.id, &WriteRequest{Deletes: []TupleKey{link}, Wait: true})
			require.NoError(t, err)
			dels = append(dels, time.Since(t0))
		}
		p.logf("  - add the team to the org (about %d reach rows): %s", repos, summarize(adds))
		p.logf("  - remove it again (rederivation): %s", summarize(dels))
	}
	for _, l := range perfSizes([]int{100, 500}) {
		s := p.load(listsModel+"type group\n  relations\n    define member: [user, group#member]\n", func(emit func(TupleKey)) {
			for i := range l {
				emit(tk(fmt.Sprintf("group:c%d#member", (i+1)%l), "member", fmt.Sprintf("group:c%d", i)))
				emit(tk(fmt.Sprintf("user:c%d", i), "member", fmt.Sprintf("group:c%d", i)))
			}
		}, nil)
		p.logf("\n#### A cycle of %d groups\n", l)
		p.storeLine(s)
		link := tk("group:c1#member", "member", "group:c0")
		var dels, adds []time.Duration
		for range 3 {
			t0 := time.Now()
			_, err := p.e.Write(p.ctx, s.id, &WriteRequest{Deletes: []TupleKey{link}, Wait: true})
			require.NoError(t, err)
			dels = append(dels, time.Since(t0))
			t0 = time.Now()
			_, err = p.e.Write(p.ctx, s.id, &WriteRequest{Writes: []TupleKey{link}, Wait: true})
			require.NoError(t, err)
			adds = append(adds, time.Since(t0))
		}
		p.logf("  - delete a link of the cycle (rederives %d reach rows): %s", l*(l-1)/2, summarize(dels))
		p.logf("  - restore it: %s", summarize(adds))
	}
}

// ladderDataset generates the GitHub-like dataset at scale s without
// duplicates, about 45k tuples per unit of s.
func ladderDataset(s int) (users, teams, orgs, repos int, gen func(emit func(TupleKey)), attrs func(emit func(object, name string, v any))) {
	users, teams, orgs, repos = 5000*s, 500*s, 20*s, 10_000*s
	u := func(i int) string { return fmt.Sprintf("user:u%07d", i%users) }
	tm := func(i int) string { return fmt.Sprintf("team:t%06d", i%teams) }
	og := func(i int) string { return fmt.Sprintf("org:o%05d", i%orgs) }
	rp := func(i int) string { return fmt.Sprintf("repo:r%08d", i) }
	orgMember := func(o, k int) int { return o*101 + k*97 }
	gen = func(emit func(TupleKey)) {
		for t := range teams {
			for k := range 8 {
				emit(tk(u(t*37+k*997), "member", tm(t)))
			}
			if t >= 100 {
				emit(tk(tm(t)+"#member", "member", tm(int(uint32(t*2654435761)%uint32(t)))))
			}
		}
		for i := range users {
			emit(tk(u(i), "member", "team:everyone"))
		}
		for o := range orgs {
			for k := range 50 {
				emit(tk(u(orgMember(o, k)), "member", og(o)))
			}
			emit(tk(u(o*53+11), "owner", og(o)))
			emit(tk(u(o*53+12), "owner", og(o)))
		}
		for r := range repos {
			emit(tk(og(r), "owner_org", rp(r)))
			emit(tk(u(r*13+2), "reader", rp(r)))
			if r%5 == 0 {
				emit(tk(u(r*31), "admin", rp(r)))
			}
			if r%10 == 0 {
				emit(tk(tm(r*3+5)+"#member", "admin", rp(r)))
				emit(tk(u(orgMember(r%orgs, (r/orgs)%50)), "blocked", rp(r)))
				if b := u(r*19 + 7); b != u(orgMember(r%orgs, (r/orgs)%50)) {
					emit(tk(b, "blocked", rp(r)))
				}
			}
			if r%10 < 3 {
				emit(tk(u(r*17+1), "writer", rp(r)))
				emit(tk(tm(r*11+3)+"#member", "writer", rp(r)))
			}
			if r%5 < 2 {
				emit(tk(tm(r*7)+"#member", "reader", rp(r)))
			}
			if r%50 == 0 {
				emit(tk("user:*", "reader", rp(r)))
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

const ladderModel = `model
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

func TestPerfLadder(t *testing.T) {
	perfEnabled(t, "ladder")
	steps := []int{1, 10, 100}
	if v := os.Getenv("GEBRA_PERF_LADDER"); v != "" {
		steps = nil
		for _, f := range strings.Split(v, ",") {
			n, _ := strconv.Atoi(f)
			steps = append(steps, n)
		}
	}
	for _, scale := range steps {
		t.Run(fmt.Sprint(scale), func(t *testing.T) { perfLadderStep(t, scale) })
	}
}

func perfLadderStep(t *testing.T, scale int) {
	p := newPerfEnv(t, fmt.Sprintf("ladder%d", scale))
	p.environment(fmt.Sprintf("ladder step %d", scale))
	users, teams, orgs, repos, gen, attrs := ladderDataset(scale)
	resetHWM()
	s := p.load(ladderModel, gen, attrs)
	sizes, total := p.tableSizes()
	p.logf("\n### Scale step %d: %d users, %d teams, %d orgs, %d repos\n", scale, users, teams, orgs, repos)
	p.storeLine(s)
	p.logf("- Postgres size %.2f GB (%s); Go RSS %.0f MB, peak %.0f MB; Postgres RSS %.0f MB", float64(total)/1e9, sizes, float64(goRSS())/1024, float64(goHWM())/1024, postgresRSSMB())

	rng := rand.New(rand.NewPCG(uint64(scale), 1))
	type q struct{ user, rel, obj string }
	rels := []string{"reader", "reader", "can_read", "writer", "admin"}
	var qs []q
	for range 4096 {
		var user string
		if rng.IntN(2) == 0 {
			o := rng.IntN(orgs)
			user = fmt.Sprintf("user:u%07d", (o*101+rng.IntN(50)*97)%users)
			// Org members read their org's repos, which makes half the checks
			// allowed.
			r := o + orgs*rng.IntN(repos/orgs)
			qs = append(qs, q{user, rels[rng.IntN(len(rels))], fmt.Sprintf("repo:r%08d", r)})
			continue
		}
		user = fmt.Sprintf("user:u%07d", rng.IntN(users))
		qs = append(qs, q{user, rels[rng.IntN(len(rels))], fmt.Sprintf("repo:r%08d", rng.IntN(repos))})
	}
	for round := range 2 {
		for _, graph := range []bool{false, true} {
			n := 500
			if graph {
				n = 200
			}
			l := p.seq(n, func(ctx context.Context, i int) error {
				c := qs[(i*7+round)%len(qs)]
				return p.check(ctx, s, graph, c.user, c.rel, c.obj, nil, nil)
			})
			path := "view"
			if graph {
				path = "graph"
			}
			p.logf("  - check, sequential, %s, round %d: %s", path, round+1, l)
		}
	}
	for round := range 2 {
		ops, l, sys := p.loop(64, 20*time.Second, func(ctx context.Context, w, i int) error {
			c := qs[(w*131+i)%len(qs)]
			return p.check(ctx, s, false, c.user, c.rel, c.obj, nil, nil)
		})
		p.logf("  - check, 64 workers closed loop, round %d: %.0f checks/s, %s; %s", round+1, ops, l, sys)
	}
	sample := func(i int) string {
		o := i % orgs
		return fmt.Sprintf("user:u%07d", (o*101+(i/orgs)%50*97)%users)
	}
	for _, lq := range []struct {
		label string
		req   ListObjectsRequest
	}{
		{"list reader by updated_at, first page of 50", ListObjectsRequest{Relation: "reader", Sort: Sort{Attribute: "updated_at", Descending: true}, PageSize: 50}},
		{"list can_read (verified) by updated_at, first page of 50", ListObjectsRequest{Relation: "can_read", Sort: Sort{Attribute: "updated_at", Descending: true}, PageSize: 50}},
		{"list reader by name, all", ListObjectsRequest{Relation: "reader", Sort: Sort{Attribute: "name"}}},
	} {
		req := lq.req
		var count atomic.Int64
		n := 100
		if req.PageSize == 0 {
			n = 20
		}
		l := p.seq(n, func(ctx context.Context, i int) error {
			r := req
			r.Type, r.User = "repo", sample(i)
			objs, _, err := p.e.ListObjects(ctx, s.id, &r)
			count.Add(int64(len(objs)))
			return err
		})
		p.logf("  - %s (avg %d results): %s", lq.label, count.Load()/int64(n), l)
	}
	ops, l, sys := p.loop(16, 15*time.Second, func(ctx context.Context, w, i int) error {
		r := sample(w*1000 + i)
		_, _, err := p.e.ListObjects(ctx, s.id, &ListObjectsRequest{Type: "repo", Relation: "reader", User: r, Sort: Sort{Attribute: "updated_at", Descending: true}, PageSize: 50})
		return err
	})
	p.logf("  - sorted first page, 16 workers: %.0f lists/s, %s; %s", ops, l, sys)

	ops, l, sys = p.loop(8, 15*time.Second, func(ctx context.Context, w, i int) error {
		wr := rand.New(rand.NewPCG(uint64(w), uint64(i/2)))
		var k TupleKey
		switch wr.IntN(4) {
		case 0:
			k = tk(fmt.Sprintf("user:u%07d", wr.IntN(users)), "member", fmt.Sprintf("team:t%06d", wr.IntN(teams)))
		case 1:
			k = tk(fmt.Sprintf("team:t%06d#member", wr.IntN(teams)), "reader", fmt.Sprintf("repo:r%08d", wr.IntN(repos)))
		default:
			k = tk(fmt.Sprintf("user:u%07d", wr.IntN(users)), "reader", fmt.Sprintf("repo:r%08d", wr.IntN(repos)))
		}
		req := &WriteRequest{OnDuplicate: "ignore", OnMissing: "ignore", Wait: true}
		if i%2 == 0 {
			req.Writes = []TupleKey{k}
		} else {
			req.Deletes = []TupleKey{k}
		}
		_, err := p.e.Write(ctx, s.id, req)
		return err
	})
	p.logf("  - writes, 8 workers (one tuple each, half deletes): %.0f writes/s, %s; %s", ops, l, sys)

	if bin := os.Getenv("GEBRA_BIN"); bin != "" {
		p.httpLadder(bin, s, func(i int) (string, string, string) {
			c := qs[i%len(qs)]
			return c.user, c.rel, c.obj
		})
	}
}

// httpLadder starts the cmd/gebra binary on the step's database and measures
// checks over HTTP.
func (p *perfEnv) httpLadder(bin string, s *perfStore, next func(i int) (string, string, string)) {
	t := p.t
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := l.Addr().String()
	_ = l.Close()
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", os.Getenv("PGUSER"), os.Getenv("PGPASSWORD"), os.Getenv("PGHOST"), os.Getenv("PGPORT"), p.dbName)
	cmd := exec.Command(bin, "-addr", addr, "-db-type", "postgres", "-db-dsn", dsn, "-db-max-open-conns", "100")
	logPath := filepath.Join(t.TempDir(), "gebra.log")
	logFile, err := os.Create(logPath)
	require.NoError(t, err)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	require.NoError(t, cmd.Start())
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()
	client := &http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: 200, MaxConnsPerHost: 200}, Timeout: opTimeout}
	base := "http://" + addr
	for range 100 {
		if resp, err := client.Get(base + "/healthz"); err == nil {
			_ = resp.Body.Close()
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	do := func(ctx context.Context, i int) error {
		user, rel, obj := next(i)
		body := fmt.Sprintf(`{"tuple_key":{"user":%q,"relation":%q,"object":%q}}`, user, rel, obj)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/stores/"+s.id+"/check", strings.NewReader(body))
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		var buf [512]byte
		_, _ = resp.Body.Read(buf[:])
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}
	lat := p.seq(500, do)
	p.logf("  - HTTP check, sequential: %s", lat)
	for round := range 2 {
		cpu0, t0 := procCPU(cmd.Process.Pid), time.Now()
		ops, lat, sys := p.loop(64, 20*time.Second, func(ctx context.Context, w, i int) error { return do(ctx, w*977+i) })
		serverCPU := (procCPU(cmd.Process.Pid) - cpu0) / time.Since(t0).Seconds()
		rss := 0.0
		if b, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", cmd.Process.Pid)); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if v, ok := strings.CutPrefix(line, "VmRSS:"); ok {
					kb, _ := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(v), " kB"), 64)
					rss = kb / 1024
				}
			}
		}
		p.logf("  - HTTP check, 64 workers, round %d: %.0f checks/s, %s; server %.1f CPUs, RSS %.0f MB; %s", round+1, ops, lat, serverCPU, rss, sys)
	}
}
