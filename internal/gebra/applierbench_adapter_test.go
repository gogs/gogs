package gebra

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"unknwon.dev/x/logx"
)

// This file holds the parts of the applier benchmark in applierbench_test.go
// that depend on the engine's write path.

const benchVariant = "applier"

func benchOptions() Options { return Options{} }

func benchSetWait(req *WriteRequest, wait bool) { req.Wait = wait }

func benchAwait(t testing.TB, e *Engine, storeID string) {
	awaitAppliedWithin(t, e, storeID, time.Hour)
}

func benchAwaitRevision(t testing.TB, e *Engine, storeID, revision string) {
	t.Helper()
	id, err := decodeIntToken(revision)
	require.NoError(t, err)
	pending, err := e.waitUntilApplied(context.Background(), e.db.handle(), storeID, id, time.Now().Add(time.Hour))
	require.NoError(t, err)
	require.Zero(t, pending)
}

// benchNotApplied reports whether a write with Wait failed because the
// applier did not apply it in time.
func benchNotApplied(err error) bool { return errors.Is(err, ErrNotApplied) }

// benchFailover starts a second engine on the benchmark's database, as a
// second process, while 16 writers write unique tuples to the second store.
// It kills the applier twice: first by terminating the leader connection of
// the benchmark's engine, then by closing the second engine once it leads.
// The benchmark's engine can win the first takeover by reconnecting, so the
// first kill repeats until the second engine leads, up to five times. It
// reports how long until an engine committed its first batch as the new
// applier and which engine that was, and checks that every write was applied
// exactly once.
func benchFailover(b *applierBench) map[string]float64 {
	t, ctx := b.t, b.ctx
	storeID := b.stores[1]
	other, err := New(ctx, logx.NewNoopLogger(), benchOpen(t, b.dbName, 24), DialectPostgres, Options{})
	require.NoError(t, err)
	otherClosed := false
	defer func() {
		if !otherClosed {
			other.Close()
		}
	}()
	benchAwait(t, b.e, storeID)
	require.True(t, b.e.lead(ctx, storeID), "the benchmark's engine applies the store")

	var writeVia atomic.Pointer[Engine]
	writeVia.Store(other)
	var written atomic.Int64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for w := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				k := benchKey(fmt.Sprintf("user:fo%02d%07d", w, i), "member", fmt.Sprintf("team:fo%04d", i%1000))
				if _, err := writeVia.Load().Write(ctx, storeID, &WriteRequest{Writes: []TupleKey{k}}); err == nil {
					written.Add(1)
				}
			}
		}()
	}
	// leading returns the benchmark engine's leader connection and whether
	// it leads the store.
	leading := func() (pid int64, held bool) {
		b.e.leader.mu.Lock()
		defer b.e.leader.mu.Unlock()
		return b.e.leader.pid, b.e.leader.held[storeID]
	}
	// takeover kills the applier and waits for the first batch of the new
	// one. The benchmark's engine keeps applying until it notices that it
	// lost its leader connection, so its batches count only once it leads
	// on a new connection, or when it did not lead before.
	takeover := func(kill func()) (ms float64, byOther bool) {
		time.Sleep(2 * time.Second)
		oldPID, wasHeld := leading()
		benchApplied, otherApplied := b.e.appliedSignal(), other.appliedSignal()
		start := time.Now()
		kill()
		deadline := time.After(time.Minute)
		for {
			select {
			case <-otherApplied:
				return float64(time.Since(start)) / float64(time.Millisecond), true
			case <-benchApplied:
				if pid, held := leading(); held && (pid != oldPID || !wasHeld) {
					return float64(time.Since(start)) / float64(time.Millisecond), false
				}
				benchApplied = b.e.appliedSignal()
			case <-deadline:
				t.Fatal("no engine took over within a minute")
				return 0, false
			}
		}
	}
	terminate := func() {
		pid, _ := leading()
		_, err := b.admin.ExecContext(ctx, "SELECT pg_terminate_backend($1)", pid)
		require.NoError(t, err)
	}
	out := map[string]float64{}
	byOther := false
	for attempt := 1; attempt <= 5 && !byOther; attempt++ {
		var ms float64
		ms, byOther = takeover(terminate)
		out["takeover_terminate_attempts"] = float64(attempt)
		if attempt == 1 {
			out["takeover_terminate_ms"] = ms
		}
		if !byOther {
			out["takeover_terminate_won_by_bench"]++
		}
	}
	writeVia.Store(b.e)
	if byOther {
		ms, _ := takeover(func() {
			other.Close()
			otherClosed = true
		})
		out["takeover_close_ms"] = ms
	} else {
		out["takeover_close_skipped"] = 1
	}
	time.Sleep(time.Second)
	close(stop)
	wg.Wait()
	start := time.Now()
	benchAwait(t, b.e, storeID)
	out["catch_up_ms"] = float64(time.Since(start)) / float64(time.Millisecond)

	var counts struct {
		Changes, Distinct, Members int64
	}
	_, err = b.e.db.handle().queryRow(ctx, `SELECT
		(SELECT COUNT(*) FROM gebra_change WHERE store_id = ? AND object_type = 'team' AND object_id LIKE 'fo%') AS changes,
		(SELECT COUNT(DISTINCT (user_id, object_id)) FROM gebra_change WHERE store_id = ? AND object_type = 'team' AND object_id LIKE 'fo%') AS distinct,
		(SELECT COUNT(*) FROM gebra_view_member m JOIN gebra_view_state v ON v.store_id = m.store_id AND v.gen = m.gen
			WHERE m.store_id = ? AND m.set_type = 'team' AND m.set_id LIKE 'fo%') AS members`,
		[]any{storeID, storeID, storeID}, &counts.Changes, &counts.Distinct, &counts.Members)
	require.NoError(t, err)
	out["writes"] = float64(written.Load())
	out["changes"] = float64(counts.Changes)
	out["duplicate_changes"] = float64(counts.Changes - counts.Distinct)
	out["missing_changes"] = float64(written.Load() - counts.Distinct)
	out["missing_view_members"] = float64(written.Load() - counts.Members)
	require.Equal(t, written.Load(), counts.Changes, "every write recorded once")
	require.Equal(t, counts.Changes, counts.Distinct, "no change recorded twice")
	require.Equal(t, written.Load(), counts.Members, "every write in the view")
	return out
}

// TestApplierBenchSummary prints the results in GEBRA_BENCH_OUT as tables
// when GEBRA_BENCH is "summary".
func TestApplierBenchSummary(t *testing.T) {
	if os.Getenv("GEBRA_BENCH") != "summary" {
		t.Skip("set GEBRA_BENCH=summary and GEBRA_BENCH_OUT to print benchmark results")
	}
	f, err := os.Open(os.Getenv("GEBRA_BENCH_OUT"))
	require.NoError(t, err)
	defer func() { _ = f.Close() }()
	var rows []benchResult
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var r benchResult
		require.NoError(t, json.Unmarshal(sc.Bytes(), &r))
		rows = append(rows, r)
	}
	require.NoError(t, sc.Err())
	slices.SortStableFunc(rows, func(a, b benchResult) int {
		if a.Scale != b.Scale {
			return a.Scale - b.Scale
		}
		return strings.Compare(a.Workload+a.Round+a.Variant, b.Workload+b.Round+b.Variant)
	})
	var out strings.Builder
	scale := -1
	for _, r := range rows {
		if r.Workload == "real" {
			continue
		}
		if r.Scale != scale {
			scale = r.Scale
			fmt.Fprintf(&out, "\n#### Scale %d\n\n", scale)
			fmt.Fprintln(&out, "| workload | round | variant | load | ops/s | p50 ms | p99 ms | max ms | errors | checks/s | check p99 ms | fallbacks | lag p50/p99/max ms | batches (avg, p99) | catch-up s | lock waits/sample |")
			fmt.Fprintln(&out, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|")
		}
		if r.Extra != nil {
			keys := make([]string, 0, len(r.Extra))
			for k := range r.Extra {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			var kv []string
			for _, k := range keys {
				kv = append(kv, fmt.Sprintf("%s=%.1f", k, r.Extra[k]))
			}
			fmt.Fprintf(&out, "| %s | %s | %s | %s | %s |||||||||||| \n", r.Workload, r.Round, r.Variant, r.Load, strings.Join(kv, ", "))
			continue
		}
		waits := 0
		for _, n := range r.LockWaits {
			waits += n
		}
		perSample := ""
		if r.LockSamples > 0 {
			perSample = fmt.Sprintf("%.2f", float64(waits)/float64(r.LockSamples))
		}
		fmt.Fprintf(&out, "| %s | %s | %s | %s | %.0f | %.1f | %.1f | %.0f | %d | %.0f | %.1f | %v | %.0f/%.0f/%.0f | %d (%.1f, %.0f) | %.2f | %s |\n",
			r.Workload, r.Round, r.Variant, r.Load, r.Ops, r.P50, r.P99, r.Max, r.Errors,
			r.CheckOps, r.CheckP99, r.Fallbacks, r.LagP50, r.LagP99, r.LagMax, r.Batches, r.BatchAvg, r.BatchP99, r.CatchUp, perSample)
	}
	realSummary(&out, rows)
	t.Log(out.String())
}

// realSummary prints the realistic workload's results: the latencies of each
// kind of operation at each rate, and the highest rate at which each kind
// keeps its p99 within GEBRA_BENCH_SLO_MS (default 20) while the engine
// sustains at least 95% of the rate without errors or violations.
func realSummary(out *strings.Builder, rows []benchResult) {
	var real []benchResult
	for _, r := range rows {
		if r.Workload == "real" {
			real = append(real, r)
		}
	}
	if len(real) == 0 {
		return
	}
	slo, _ := strconv.ParseFloat(benchEnv("GEBRA_BENCH_SLO_MS", "20"), 64)
	ops := []string{realCheck, realList, realCheckToken, realWrite, realWriteBulk, realWriteFan}
	slices.SortStableFunc(real, func(a, b benchResult) int {
		if a.Scale != b.Scale {
			return a.Scale - b.Scale
		}
		if c := strings.Compare(a.Round+a.Variant, b.Round+b.Variant); c != 0 {
			return c
		}
		return int(a.Rate - b.Rate)
	})
	fmt.Fprintf(out, "\n#### Realistic load\n\nLatencies are p50/p99/p99.9 in ms, counted from when each request was due.\n\n")
	fmt.Fprintf(out, "| scale | round | variant | rate/s | achieved/s | %s | violations | errors | fallbacks | lag p50/p99/max ms | catch-up s |\n", strings.Join(ops, " | "))
	fmt.Fprintf(out, "|%s\n", strings.Repeat("---|", 10+len(ops)))
	type run struct {
		scale          int
		round, variant string
	}
	best := map[run]map[string]float64{}
	var order []run
	for _, r := range real {
		var cells []string
		for _, op := range ops {
			st, ok := r.PerOp[op]
			if !ok {
				cells = append(cells, "")
				continue
			}
			cells = append(cells, fmt.Sprintf("%.1f/%.1f/%.1f (%d)", st.P50, st.P99, st.P999, st.N))
		}
		fmt.Fprintf(out, "| %d | %s | %s | %.0f | %.0f | %s | %d | %d | %v | %.0f/%.0f/%.0f | %.2f |\n",
			r.Scale, r.Round, r.Variant, r.Rate, r.Achieved, strings.Join(cells, " | "), r.Violations, r.Errors,
			r.Fallbacks, r.LagP50, r.LagP99, r.LagMax, r.CatchUp)
		k := run{r.Scale, r.Round, r.Variant}
		if best[k] == nil {
			best[k] = map[string]float64{}
			order = append(order, k)
		}
		if r.Achieved < 0.95*r.Rate || r.Violations > 0 {
			continue
		}
		for _, op := range ops {
			if st, ok := r.PerOp[op]; ok && st.Errors == 0 && st.P99 <= slo {
				best[k][op] = max(best[k][op], r.Rate)
			}
		}
	}
	fmt.Fprintf(out, "\nHighest rate/s at which each kind's p99 stays within %.0f ms:\n\n", slo)
	fmt.Fprintf(out, "| scale | round | variant | %s |\n|%s\n", strings.Join(ops, " | "), strings.Repeat("---|", 3+len(ops)))
	for _, k := range order {
		var cells []string
		for _, op := range ops {
			if v, ok := best[k][op]; ok {
				cells = append(cells, fmt.Sprintf("%.0f", v))
			} else {
				cells = append(cells, "none")
			}
		}
		fmt.Fprintf(out, "| %d | %s | %s | %s |\n", k.scale, k.round, k.variant, strings.Join(cells, " | "))
	}
}
