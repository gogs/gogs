package gebra

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"github.com/stretchr/testify/require"
)

// benchDiag profiles one phase of a benchmark workload when GEBRA_DIAG_DIR is
// set: the Go CPU profile, pg_stat_statements of the benchmark database, and
// the wait events of its active backends sampled every 50ms.
type benchDiag struct {
	b      *applierBench
	name   string
	dir    string
	start  time.Time
	cpu0   float64
	busy0  float64
	prof   *os.File
	stop   chan struct{}
	done   sync.WaitGroup
	mu     sync.Mutex
	waits  map[string]int
	nWaits int
	extra  map[string]float64
	// timeline holds samples of the store's inbox and build progress.
	timeline []map[string]any
}

func diagCPU() (goCPU, machineBusy float64) {
	busy, _ := readCPU()
	return procCPU(os.Getpid()), busy / 100
}

func (b *applierBench) diag(name string, timelineStore string) *benchDiag {
	dir := os.Getenv("GEBRA_DIAG_DIR")
	d := &benchDiag{b: b, name: name, dir: dir, waits: map[string]int{}, extra: map[string]float64{}, stop: make(chan struct{})}
	if dir == "" {
		return d
	}
	_, err := b.admin.ExecContext(b.ctx, "SELECT pg_stat_statements_reset()")
	require.NoError(b.t, err)
	if os.Getenv("GEBRA_DIAG_CPU") != "" {
		d.prof, err = os.Create(filepath.Join(dir, fmt.Sprintf("%s_%s.cpu", benchVariant, name)))
		require.NoError(b.t, err)
		require.NoError(b.t, pprof.StartCPUProfile(d.prof))
	}
	d.start = time.Now()
	d.cpu0, d.busy0 = diagCPU()
	d.done.Add(1)
	go func() {
		defer d.done.Done()
		tick := time.NewTicker(50 * time.Millisecond)
		defer tick.Stop()
		n := 0
		for {
			select {
			case <-d.stop:
				return
			case <-tick.C:
			}
			n++
			rows, err := b.admin.QueryContext(b.ctx, `SELECT coalesce(wait_event_type, 'CPU') || ':' || coalesce(wait_event, '') || ' ' ||
					CASE WHEN query LIKE 'INSERT INTO gebra_pending%' THEN 'pending_insert'
						WHEN query LIKE 'INSERT INTO gebra_tuple%' THEN 'tuple_insert'
						WHEN query LIKE 'COMMIT%' THEN 'commit' ELSE '' END, count(*)
				FROM pg_stat_activity WHERE datname = $1 AND state = 'active' AND pid <> pg_backend_pid() GROUP BY 1`, b.dbName)
			if err == nil {
				d.mu.Lock()
				d.nWaits++
				for rows.Next() {
					var w string
					var c int
					if rows.Scan(&w, &c) == nil {
						d.waits[w] += c
					}
				}
				d.mu.Unlock()
				// A failed sample only loses that sample.
				_ = rows.Err()
				_ = rows.Close()
			}
			if timelineStore != "" && n%5 == 0 {
				var s struct{ Writes, Changes, Oldest int64 }
				db := b.e.db.handle()
				_, _ = db.queryRow(b.ctx, `SELECT COUNT(*) AS writes, COALESCE(SUM(tuples), 0) AS changes,
					COALESCE(EXTRACT(EPOCH FROM (now() AT TIME ZONE 'UTC' - MIN(created_at))) * 1000, 0)::bigint AS oldest
					FROM gebra_inbox WHERE store_id = ?`, []any{timelineStore}, &s.Writes, &s.Changes, &s.Oldest)
				var vs viewStateRow
				if rows, err := queryStructs[viewStateRow](b.ctx, db, "SELECT * FROM gebra_view_state WHERE store_id = ?", timelineStore); err == nil && len(rows) > 0 {
					vs = rows[0]
				}
				d.mu.Lock()
				d.timeline = append(d.timeline, map[string]any{
					"t_ms": time.Since(d.start).Milliseconds(), "pending_writes": s.Writes, "pending_changes": s.Changes,
					"lag_ms": s.Oldest, "phase": vs.BuildPhase, "done": vs.BuildDone, "total": vs.BuildTotal, "chunks": vs.BuildChunks,
				})
				d.mu.Unlock()
			}
		}
	}()
	return d
}

// finish writes the phase's profiles and adds its wall time, Go CPU time,
// and any extra values to r.Extra under the phase's name.
func (d *benchDiag) finish(r *benchResult) {
	if d.dir == "" {
		return
	}
	close(d.stop)
	d.done.Wait()
	wall := time.Since(d.start)
	cpu1, busy1 := diagCPU()
	if d.prof != nil {
		pprof.StopCPUProfile()
		_ = d.prof.Close()
	}
	b := d.b
	base := filepath.Join(d.dir, fmt.Sprintf("%s_%s", benchVariant, d.name))
	rows, err := b.admin.QueryContext(b.ctx, `SELECT calls, total_exec_time, mean_exec_time, rows, total_plan_time, plans,
			shared_blks_hit, shared_blks_read, regexp_replace(query, '\s+', ' ', 'g')
		FROM pg_stat_statements s JOIN pg_database d ON d.oid = s.dbid WHERE d.datname = $1 ORDER BY total_exec_time DESC`, b.dbName)
	require.NoError(b.t, err)
	f, err := os.Create(base + ".pgss.tsv")
	require.NoError(b.t, err)
	_, _ = fmt.Fprintln(f, "calls\ttotal_ms\tmean_ms\trows\tplan_ms\tplans\thit\tread\tquery")
	for rows.Next() {
		var calls, nrows, plans, hit, read int64
		var total, mean, plan float64
		var q string
		require.NoError(b.t, rows.Scan(&calls, &total, &mean, &nrows, &plan, &plans, &hit, &read, &q))
		_, _ = fmt.Fprintf(f, "%d\t%.3f\t%.4f\t%d\t%.3f\t%d\t%d\t%d\t%s\n", calls, total, mean, nrows, plan, plans, hit, read, q)
	}
	require.NoError(b.t, rows.Err())
	_ = rows.Close()
	_ = f.Close()
	type kv struct {
		K string
		N int
	}
	var ws []kv
	for k, n := range d.waits {
		ws = append(ws, kv{k, n})
	}
	sort.Slice(ws, func(i, j int) bool { return ws[i].N > ws[j].N })
	wf, err := os.Create(base + ".waits.txt")
	require.NoError(b.t, err)
	_, _ = fmt.Fprintf(wf, "samples %d\n", d.nWaits)
	for _, w := range ws {
		_, _ = fmt.Fprintf(wf, "%8.2f %s\n", float64(w.N)/float64(max(d.nWaits, 1)), w.K)
	}
	_ = wf.Close()
	if len(d.timeline) > 0 {
		tf, err := os.Create(base + ".timeline.jsonl")
		require.NoError(b.t, err)
		for _, s := range d.timeline {
			line, _ := json.Marshal(s)
			_, _ = fmt.Fprintln(tf, string(line))
		}
		_ = tf.Close()
	}
	if r.Extra == nil {
		r.Extra = map[string]float64{}
	}
	r.Extra[d.name+"_wall_s"] = wall.Seconds()
	r.Extra[d.name+"_go_cores"] = (cpu1 - d.cpu0) / wall.Seconds()
	r.Extra[d.name+"_machine_cores"] = (busy1 - d.busy0) / wall.Seconds()
	for k, v := range d.extra {
		r.Extra[k] = v
	}
}

// diagPending returns the writes and tuple changes pending in every store.
func (b *applierBench) diagPending() (writes, changes int64) {
	_, err := b.e.db.handle().queryRow(b.ctx, `SELECT COUNT(*) AS writes, COALESCE(SUM(tuples), 0) AS changes FROM gebra_inbox`, nil, &writes, &changes)
	require.NoError(b.t, err)
	return writes, changes
}

// diagBuildCycles records, while a model build runs, how long each applier
// cycle took and how much of it the build chunk took, from the build hooks.
type diagBuildCycles struct {
	mu               sync.Mutex
	last, chunkStart time.Time
	cycles, chunk    []time.Duration
	items            map[string]int
	chunkTime        map[string]time.Duration
	installed        bool
}

func (b *applierBench) diagBuild() *diagBuildCycles {
	c := &diagBuildCycles{items: map[string]int{}, chunkTime: map[string]time.Duration{}}
	if os.Getenv("GEBRA_DIAG_DIR") == "" {
		return c
	}
	c.installed = true
	b.e.opts.hooks.beforeBuildChunk = func(ctx context.Context, storeID string, gen int64) {
		c.mu.Lock()
		defer c.mu.Unlock()
		now := time.Now()
		if !c.last.IsZero() {
			c.cycles = append(c.cycles, now.Sub(c.last))
		}
		c.last, c.chunkStart = now, now
	}
	b.e.opts.hooks.beforeBuildCommit = func(ctx context.Context, info buildChunkInfo) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		d := time.Since(c.chunkStart)
		c.chunk = append(c.chunk, d)
		c.items[info.phase] += info.items
		c.chunkTime[info.phase] += d
		return nil
	}
	return c
}

func (c *diagBuildCycles) finish(r *benchResult) {
	if !c.installed {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var cyc, ch time.Duration
	for _, d := range c.cycles {
		cyc += d
	}
	for _, d := range c.chunk {
		ch += d
	}
	if r.Extra == nil {
		r.Extra = map[string]float64{}
	}
	r.Extra["build_cycles"] = float64(len(c.cycles))
	r.Extra["build_cycle_avg_ms"] = float64(cyc.Milliseconds()) / float64(max(len(c.cycles), 1))
	r.Extra["build_chunk_avg_ms"] = float64(ch.Milliseconds()) / float64(max(len(c.chunk), 1))
	for phase, n := range c.items {
		r.Extra["build_"+phase+"_items"] = float64(n)
		r.Extra["build_"+phase+"_chunk_s"] = c.chunkTime[phase].Seconds()
		if s := c.chunkTime[phase].Seconds(); s > 0 {
			r.Extra["build_"+phase+"_items_per_chunk_s"] = float64(n) / s
		}
	}
}
