package gebra

// The HTTP benchmark runs the realistic workload of applierbench_real_test.go
// over HTTP, against Gebra's server or against OpenFGA, so that both get the
// same requests, from the same seeds, at the same times, on the same dataset
// and the same Postgres. Latency counts from when each request was due, as
// in process, and results go to GEBRA_BENCH_OUT with variant "gebra-http" or
// "openfga".
//
// With GEBRA_BENCH_TARGET=gebra, the benchmark loads the dataset in process,
// as TestApplierBench does, and closes its engine. Then it runs cmd/gebra as
// its own process, as OpenFGA runs, on the same database on 127.0.0.1, with
// GEBRA_BENCH_CONNS connections (default 96, as OpenFGA's) and a max
// staleness of GEBRA_BENCH_MAX_STALENESS (default 1s, the TTL of OpenFGA's
// caches), and stops it at the end. GEBRA_BENCH_GOMAXPROCS sets cmd/gebra's
// GOMAXPROCS, which otherwise is the number of CPUs. As the servers share the
// machine with Postgres and the load generator, a Go server that runs on every
// CPU wakes more threads than there are CPUs left and delays Postgres; set
// GOMAXPROCS for OpenFGA's script to compare the two at the same value.
// GEBRA_BENCH_GEBRA_BIN is a prebuilt cmd/gebra, which the benchmark
// otherwise builds. The benchmark prints the path of cmd/gebra's log, which it
// keeps when the test fails:
//
//	GOGS_DATABASE_TYPE=postgres PGHOST=127.0.0.1 PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres PGSSLMODE=disable \
//	GEBRA_BENCH=http GEBRA_BENCH_TARGET=gebra GEBRA_BENCH_SCALE=1 GEBRA_BENCH_STORES=1 GEBRA_BENCH_OUT=$PWD/http-bench.jsonl \
//	go test -count=1 -timeout 4h -run 'TestApplierBenchHTTP$' -v ./internal/gebra
//
// With GEBRA_BENCH_TARGET=openfga, it talks to the OpenFGA server at
// GEBRA_OPENFGA_URL, which testdata/openfga-bench.sh sets up and runs on the
// same Postgres. It creates a store with the benchmark's model without
// attributes, which OpenFGA lacks, and writes the dataset's tuples with the
// Write API, 100 per request and many requests at once. GEBRA_BENCH_OPENFGA_DB
// names OpenFGA's database, which the benchmark then vacuums and analyzes as
// it does its own. GEBRA_BENCH_VERIFY=n first loads the dataset into Gebra in
// process too, and compares n checks and n lists of the workload's kinds
// between the two, the lists both capped at 50 and in full:
//
//	testdata/openfga-bench.sh # In another terminal, from internal/gebra.
//	PGHOST=127.0.0.1 PGPORT=5432 PGUSER=postgres PGPASSWORD=postgres PGSSLMODE=disable GOGS_DATABASE_TYPE=postgres \
//	GEBRA_OPENFGA_URL=http://127.0.0.1:18080 GEBRA_BENCH_OPENFGA_DB=openfga_bench GEBRA_BENCH_VERIFY=300 \
//	GEBRA_BENCH=http GEBRA_BENCH_TARGET=openfga GEBRA_BENCH_SCALE=1 GEBRA_BENCH_OUT=$PWD/http-bench.jsonl \
//	go test -count=1 -timeout 4h -run 'TestApplierBenchHTTP$' -v ./internal/gebra
//
// Then print both side by side with TestApplierBenchSummary. The dataset's
// sizes, the rates, and the shares of the workload come from the same
// settings as in process.
//
// What differs between the two targets:
//
//   - A check with a session's token sends Gebra the write's revision as
//     min_revision. OpenFGA has no revisions, so it sends
//     "consistency": "HIGHER_CONSISTENCY", which reads the latest data and
//     skips OpenFGA's caches. Other checks and lists send no consistency.
//   - Default checks and lists may be up to 1s stale on both: OpenFGA's
//     caches keep results for 1s, and Gebra's queries evaluate the graph
//     once its view lags its oldest pending write by more than 1s.
//   - A list asks Gebra for a page of 50 sorted repos. OpenFGA neither pages
//     nor sorts, so its server returns at most 50 (--listObjects-max-results),
//     which lets it stop at any 50 it finds first. This favors OpenFGA.
//   - Neither target's results have the in-process engine's metrics, i.e.,
//     fallbacks, applier lag and batches. Gebra's have the catch-up, which
//     the benchmark reads from the view status, and OpenFGA has no view.

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/dbx"
)

// benchHTTPConns bounds the connections of the benchmark's HTTP client. It is
// above what the realistic workload has in flight, i.e., 1024 reads and the
// change sessions, so that requests never queue in the client.
const benchHTTPConns = 4096

// benchHTTPClient returns the HTTP client of both targets, which keeps
// connections alive and does not compress.
func benchHTTPClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		MaxIdleConns:        benchHTTPConns,
		MaxIdleConnsPerHost: benchHTTPConns,
		MaxConnsPerHost:     benchHTTPConns,
		IdleConnTimeout:     5 * time.Minute,
		DisableCompression:  true,
	}}
}

// realHTTP is a server with OpenFGA's HTTP API as the target of the realistic
// workload. Gebra's tokens are revisions. OpenFGA has none, so its tokens ask
// for higher consistency.
type realHTTP struct {
	c       *fgaClient
	storeID string
	openFGA bool
}

const realFGAToken = "HIGHER_CONSISTENCY"

func (r realHTTP) check(ctx context.Context, q [3]string, token string) (bool, error) {
	body := map[string]any{"tuple_key": map[string]string{"user": q[0], "relation": q[1], "object": q[2]}}
	switch {
	case token == "":
	case r.openFGA:
		body["consistency"] = token
	default:
		body["min_revision"] = token
	}
	var out struct {
		Allowed bool `json:"allowed"`
	}
	err := r.c.do(ctx, http.MethodPost, "/stores/"+r.storeID+"/check", body, &out)
	return out.Allowed, err
}

func (r realHTTP) list(ctx context.Context, user string) error {
	_, err := r.listObjects(ctx, user)
	return err
}

func (r realHTTP) listObjects(ctx context.Context, user string) ([]string, error) {
	body := map[string]any{"type": "repo", "relation": "reader", "user": user}
	if !r.openFGA {
		body["page_size"] = 50
	}
	var out struct {
		Objects []string `json:"objects"`
	}
	err := r.c.do(ctx, http.MethodPost, "/stores/"+r.storeID+"/list-objects", body, &out)
	return out.Objects, err
}

func (r realHTTP) write(ctx context.Context, req *WriteRequest) (string, error) {
	rev, err := r.c.writeRevision(ctx, r.storeID, req)
	if err == nil && r.openFGA {
		rev = realFGAToken
	}
	return rev, err
}

// TestApplierBenchHTTP runs the realistic workload over HTTP when
// GEBRA_BENCH is "http".
func TestApplierBenchHTTP(t *testing.T) {
	if os.Getenv("GEBRA_BENCH") != "http" {
		t.Skip("set GEBRA_BENCH=http and GEBRA_BENCH_TARGET to gebra or openfga to run the HTTP benchmark")
	}
	scale, err := strconv.Atoi(benchEnv("GEBRA_BENCH_SCALE", "1"))
	require.NoError(t, err)
	d, err := time.ParseDuration(benchEnv("GEBRA_BENCH_DURATION", "8s"))
	require.NoError(t, err)
	client := &fgaClient{http: benchHTTPClient()}
	var b *applierBench
	var target realHTTP
	var await func() time.Duration
	switch name := os.Getenv("GEBRA_BENCH_TARGET"); name {
	case "gebra":
		b = newRealBench(t, "gebra-http", scale)
		b.e.Close()
		b.e = nil
		require.NoError(t, b.db.Close())
		client.base = startGebra(t, b.dbName)
		target = realHTTP{c: client, storeID: b.stores[0]}
		await = func() time.Duration {
			ctx, cancel := context.WithTimeout(b.ctx, time.Hour)
			defer cancel()
			took, err := target.awaitView(ctx)
			require.NoError(t, err)
			return took
		}
	case "openfga":
		client.base = openFGAURL()
		require.NotEmpty(t, client.base, "set GEBRA_OPENFGA_URL to the OpenFGA server")
		b = &applierBench{
			t: t, ctx: context.Background(), scale: scale,
			base: benchResult{Variant: "openfga", Scale: scale, Round: benchEnv("GEBRA_BENCH_ROUND", "")},
		}
		target = realHTTP{c: client, storeID: b.loadOpenFGA(client), openFGA: true}
		if v := os.Getenv("GEBRA_BENCH_VERIFY"); v != "" {
			n, err := strconv.Atoi(v)
			require.NoError(t, err, "GEBRA_BENCH_VERIFY")
			// A subtest, so that Gebra's database is gone before the benchmark.
			t.Run("verify", func(t *testing.T) { realVerify(t, scale, n, target) })
		}
	default:
		t.Fatalf("set GEBRA_BENCH_TARGET to gebra or openfga, not %q", name)
	}
	if path := os.Getenv("GEBRA_BENCH_OUT"); path != "" {
		b.out, err = os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		require.NoError(t, err)
		defer func() { _ = b.out.Close() }()
	}
	for _, r := range b.realRun(d, target, await) {
		b.emit(r)
	}
}

// startGebra runs cmd/gebra on the database until t ends and returns its URL.
// It runs GEBRA_BENCH_GEBRA_BIN, or else builds cmd/gebra. Its output goes to
// a log file, which is kept when t fails.
func startGebra(t *testing.T, dbName string) string {
	bin := os.Getenv("GEBRA_BENCH_GEBRA_BIN")
	if bin == "" {
		bin = filepath.Join(t.TempDir(), "gebra")
		out, err := exec.Command("go", "build", "-o", bin, "gogs.io/gogs/cmd/gebra").CombinedOutput()
		require.NoError(t, err, "build cmd/gebra: %s", out)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	dsn, err := dbx.NewDSN(benchDBOpts(dbName))
	require.NoError(t, err)
	// The default staleness is the TTL of OpenFGA's caches in
	// testdata/openfga-bench.sh, so that default queries may be as stale on
	// both.
	staleness := benchEnv("GEBRA_BENCH_MAX_STALENESS", "1s")
	_, err = time.ParseDuration(staleness)
	require.NoError(t, err, "GEBRA_BENCH_MAX_STALENESS")
	args := []string{
		"--addr", addr,
		"--db-type", "postgres",
		"--db-max-open-conns", benchEnv("GEBRA_BENCH_CONNS", "96"),
		"--max-staleness", staleness,
	}
	logFile, err := os.CreateTemp("", "gebra-bench-*.log")
	require.NoError(t, err)
	logPath := logFile.Name()
	t.Logf("gebra-http: running %s %s, logging to %s", bin, strings.Join(args, " "), logPath)

	cmd := exec.Command(bin, append(args, "--db-dsn", dsn)...)
	// Spans are not exported without an OTLP endpoint, and not recorded
	// either with this, like OpenFGA's with tracing disabled.
	cmd.Env = append(os.Environ(), "OTEL_TRACES_SAMPLER=always_off")
	if procs := os.Getenv("GEBRA_BENCH_GOMAXPROCS"); procs != "" {
		cmd.Env = append(cmd.Env, "GOMAXPROCS="+procs)
	}
	cmd.Stdout, cmd.Stderr = logFile, logFile
	require.NoError(t, cmd.Start())
	_ = logFile.Close()
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = cmd.Wait()
		close(exited)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Signal(syscall.SIGTERM)
		select {
		case <-exited:
			if waitErr != nil {
				t.Errorf("cmd/gebra exited with %v, see %s", waitErr, logPath)
			}
		case <-time.After(30 * time.Second):
			_ = cmd.Process.Kill()
			<-exited
			t.Errorf("cmd/gebra did not stop within 30s, see %s", logPath)
		}
		if !t.Failed() {
			_ = os.Remove(logPath)
		}
	})

	base := "http://" + addr
	probe := &fgaClient{base: base, http: &http.Client{Timeout: time.Second}}
	deadline := time.Now().Add(time.Minute)
	for {
		select {
		case <-exited:
			t.Fatalf("cmd/gebra exited with %v before it answered, see %s", waitErr, logPath)
		default:
		}
		err := probe.do(context.Background(), http.MethodGet, "/healthz", nil, nil)
		if err == nil {
			return base
		}
		require.True(t, time.Now().Before(deadline), "cmd/gebra did not answer within a minute: %v, see %s", err, logPath)
		time.Sleep(50 * time.Millisecond)
	}
}

// awaitView waits until the store's view includes every write, as
// benchAwait does in process, and returns how long that took.
func (r realHTTP) awaitView(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	for {
		var st ViewStatus
		if err := r.c.do(ctx, http.MethodGet, "/stores/"+r.storeID+"/view-status", nil, &st); err != nil {
			return 0, err
		}
		if st.PendingWrites == 0 && st.Migration == nil {
			return time.Since(start), nil
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// newRealBench creates a database and loads the benchmark's dataset into an
// engine on it, as TestApplierBench does. The database is dropped when t
// ends, unless GEBRA_BENCH_KEEP is set.
func newRealBench(t *testing.T, variant string, scale int) *applierBench {
	if os.Getenv("GOGS_DATABASE_TYPE") != "postgres" {
		t.Fatal("the benchmark runs on Postgres")
	}
	ctx := context.Background()
	conns, err := strconv.Atoi(benchEnv("GEBRA_BENCH_CONNS", "96"))
	require.NoError(t, err)
	dsn, err := dbx.NewDSN(benchDBOpts("postgres"))
	require.NoError(t, err)
	admin, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	t.Cleanup(func() { _ = admin.Close() })
	name := fmt.Sprintf("gebra_bench_%s_%d", strings.ReplaceAll(variant, "-", "_"), time.Now().UnixNano())
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
	b := &applierBench{
		t: t, ctx: ctx, dbName: name, db: db, admin: admin, e: e, reader: reader, scale: scale,
		base: benchResult{Variant: variant, Scale: scale, Round: benchEnv("GEBRA_BENCH_ROUND", "")},
	}
	// Callers that close the engine early set b.e to nil.
	t.Cleanup(func() {
		if b.e != nil {
			b.e.Close()
		}
	})
	b.load()
	return b
}

// loadOpenFGA creates a store on OpenFGA with the benchmark's model and
// tuples and returns its ID. It writes 100 tuples per request, OpenFGA's
// limit, with 32 requests at once, and retries each request up to 5 times.
func (b *applierBench) loadOpenFGA(c *fgaClient) string {
	t, ctx := b.t, b.ctx
	start := time.Now()
	m, err := ParseDSL(benchLadderModel)
	require.NoError(t, err)
	storeID, err := c.createStore(ctx, fmt.Sprintf("bench-%d", time.Now().UnixNano()))
	require.NoError(t, err)
	_, err = c.writeModel(ctx, storeID, m)
	require.NoError(t, err)

	_, _, _, _, gen, _ := benchLadderDataset(b.scale)
	batches := make(chan []TupleKey, 64)
	var written atomic.Int64
	var mu sync.Mutex
	var firstErr error
	var wg sync.WaitGroup
	for range 32 {
		wg.Go(func() {
			for batch := range batches {
				var err error
				for attempt := range 5 {
					time.Sleep(time.Duration(attempt*attempt) * 100 * time.Millisecond)
					if err = c.write(ctx, storeID, &WriteRequest{Writes: batch, OnDuplicate: "ignore"}); err == nil {
						written.Add(int64(len(batch)))
						break
					}
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		})
	}
	batch := make([]TupleKey, 0, 100)
	gen(func(k TupleKey) {
		batch = append(batch, k)
		if len(batch) == 100 {
			batches <- batch
			batch = make([]TupleKey, 0, 100)
		}
	})
	if len(batch) > 0 {
		batches <- batch
	}
	close(batches)
	wg.Wait()
	require.NoError(t, firstErr)
	loaded := time.Since(start)

	if name := os.Getenv("GEBRA_BENCH_OPENFGA_DB"); name != "" {
		// A parallel vacuum can need more shared memory than containers have.
		_, err := benchOpen(t, name, 1).ExecContext(ctx, "VACUUM (ANALYZE, PARALLEL 0)")
		require.NoError(t, err)
	} else {
		t.Log("GEBRA_BENCH_OPENFGA_DB is not set, so OpenFGA's tables are not vacuumed and analyzed after the load")
	}
	t.Logf("openfga: loaded store %s with %d tuples in %s, %s with the vacuum", storeID, written.Load(), loaded, time.Since(start))
	return storeID
}

// realVerify loads the benchmark's dataset into Gebra in process and compares
// n checks and n lists of the realistic workload's kinds with OpenFGA. OpenFGA
// lists at most 50 objects, which must be among Gebra's, and its streaming
// list, which has no cap, must equal Gebra's.
func realVerify(t *testing.T, scale, n int, fga realHTTP) {
	b := newRealBench(t, "verify", scale)
	gebra := realEngine{e: b.e, storeID: b.stores[0]}
	ctx := b.ctx
	g := newRealGen(scale, 0)
	allowed := 0
	for range n {
		q := g.check()
		want, err := gebra.check(ctx, q, "")
		require.NoError(t, err)
		got, err := fga.check(ctx, q, realFGAToken)
		require.NoError(t, err)
		require.Equal(t, want, got, "check %v", q)
		if want {
			allowed++
		}
	}
	capped, empty := 0, 0
	for range n {
		user := benchUser(g.userIdx())
		all, _, err := b.e.ListObjects(ctx, b.stores[0], &ListObjectsRequest{Type: "repo", Relation: "reader", User: user})
		require.NoError(t, err)
		got, err := fga.listObjects(ctx, user)
		require.NoError(t, err)
		// With as many distinct objects as Gebra's up to 50, all of them
		// Gebra's, OpenFGA's are the same set when Gebra has at most 50.
		require.Len(t, got, min(len(all), 50), "objects of %s", user)
		set := make(map[string]bool, len(all))
		for _, o := range all {
			set[o] = true
		}
		for _, o := range got {
			require.True(t, set[o], "%s reads %s on OpenFGA but not on Gebra, or OpenFGA returned it twice", user, o)
			delete(set, o)
		}
		// The streaming endpoint has no cap, so it gives OpenFGA's full set.
		streamed, err := fga.c.streamedListObjects(ctx, fga.storeID, &diffQuery{Type: "repo", Relation: "reader", User: user})
		require.NoError(t, err)
		require.ElementsMatch(t, all, streamed, "objects of %s", user)
		if len(all) > 50 {
			capped++
		}
		if len(all) == 0 {
			empty++
		}
	}
	t.Logf("verify: %d checks agree (%d allowed), %d lists agree in full and capped at 50 (%d above 50 objects, %d empty)", n, allowed, n, capped, empty)
}
