package telemetry

import (
	"context"
	"math"
	"runtime"
	"runtime/metrics"
	"sync"
	"time"
)

const (
	healthStep       = 15 * time.Second
	healthPoints     = 240
	healthLongEvery  = 20
	healthLongPoints = 288
)

// Gauge is a measurement of the server.
type Gauge struct {
	Key string
	// Limit is the most the value can be, e.g., the size of a pool, or 0 when
	// there is no limit.
	Limit float64
	// Read returns the value now, and false when it cannot be measured.
	Read func() (float64, bool)
	// rate is set for values per second or minute, which are averaged over the
	// long step instead of taking the highest.
	rate bool
}

// Disk is a directory, e.g., of the repositories, with the key of the gauge
// that measures the file system it is on.
type Disk struct {
	Key  string
	Path string
}

// counter tells how much a value that only grows, e.g., the CPU time of the
// process, grew between two readings of it.
type counter struct {
	last float64
	at   time.Time
}

// take returns how much the value grew since the last call and how long ago
// that was, and false on the first call.
func (c *counter) take(value float64, now time.Time) (delta float64, elapsed time.Duration, ok bool) {
	last, at := c.last, c.at
	c.last, c.at = value, now
	if at.IsZero() {
		return 0, 0, false
	}
	return value - last, now.Sub(at), true
}

// per returns how many of the count there are per the given duration, e.g., a
// minute, when they took the elapsed time to add up.
func per(unit time.Duration, count float64, elapsed time.Duration) (float64, bool) {
	if elapsed <= 0 {
		return 0, false
	}
	return count * float64(unit) / float64(elapsed), true
}

// CounterGauge returns the gauge of how fast a count that only grows, e.g., of
// failed deliveries, grows per the given duration.
func CounterGauge(key string, unit time.Duration, read func() float64) Gauge {
	var c counter
	return Gauge{Key: key, rate: true, Read: func() (float64, bool) {
		delta, elapsed, ok := c.take(read(), time.Now())
		if !ok {
			return 0, false
		}
		return per(unit, delta, elapsed)
	}}
}

type point struct {
	at    time.Time
	value float64
}

type pointRing struct {
	points []point
	next   int
	full   bool
}

func (r *pointRing) push(p point) {
	r.points[r.next] = p
	r.next = (r.next + 1) % len(r.points)
	if r.next == 0 {
		r.full = true
	}
}

// ordered returns the points oldest first.
func (r *pointRing) ordered() []point {
	if !r.full {
		return r.points[:r.next]
	}
	return append(append([]point(nil), r.points[r.next:]...), r.points[:r.next]...)
}

type gaugeState struct {
	Gauge
	short pointRing
	long  pointRing
	// sum, peak and n accumulate the short points of the current long step.
	sum  float64
	peak float64
	n    int
}

// Health samples gauges every 15 seconds and keeps the last hour of them, and
// the last 24 hours in 5-minute steps.
type Health struct {
	mu     sync.Mutex
	gauges []*gaugeState
	ticks  int
}

// NewHealth returns a Health of the given gauges, the requests and the memory
// of the store when it is not nil, the Go runtime and, where supported, the
// process.
func NewHealth(store *Store, gauges ...Gauge) *Health {
	var all []Gauge
	if store != nil {
		all = append(all, storeGauges(store)...)
	}
	all = append(all, runtimeGauges()...)
	all = append(all, processGauges()...)
	all = append(all, gauges...)

	h := &Health{}
	for _, g := range all {
		h.gauges = append(h.gauges, &gaugeState{
			Gauge: g,
			short: pointRing{points: make([]point, healthPoints)},
			long:  pointRing{points: make([]point, healthLongPoints)},
		})
	}
	return h
}

// Run samples the gauges until ctx is done.
func (h *Health) Run(ctx context.Context) {
	ticker := time.NewTicker(healthStep)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.sample(now)
		}
	}
}

func (h *Health) sample(now time.Time) {
	values := make([]float64, len(h.gauges))
	measured := make([]bool, len(h.gauges))
	for i, g := range h.gauges {
		values[i], measured[i] = g.Read()
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	h.ticks++
	long := h.ticks%healthLongEvery == 0
	for i, g := range h.gauges {
		if measured[i] {
			g.short.push(point{at: now, value: values[i]})
			g.sum += values[i]
			g.peak = max(g.peak, values[i])
			g.n++
		}
		if !long || g.n == 0 {
			continue
		}
		value := g.peak
		if g.rate {
			value = g.sum / float64(g.n)
		}
		g.long.push(point{at: now, value: value})
		g.sum, g.peak, g.n = 0, 0, 0
	}
}

type HealthSeries struct {
	Key    string       `json:"key"`
	Points [][2]float64 `json:"points"`
	Limit  float64      `json:"limit"`
}

type HealthReport struct {
	Range       string         `json:"range"`
	StepSeconds int            `json:"stepSeconds"`
	Start       time.Time      `json:"start"`
	End         time.Time      `json:"end"`
	Series      []HealthSeries `json:"series"`
}

// Report returns the last hour of the gauges, or the last 24 hours when long
// is set.
func (h *Health) Report(now time.Time, rangeName string, long bool) HealthReport {
	out := HealthReport{
		Range:       rangeName,
		StepSeconds: int(healthStep / time.Second),
		Start:       now.Add(-healthStep * healthPoints),
		End:         now,
		Series:      []HealthSeries{},
	}
	if long {
		out.StepSeconds *= healthLongEvery
		out.Start = now.Add(-healthStep * healthLongEvery * healthLongPoints)
	}
	if h == nil {
		return out
	}

	// The rings hold a number of points, which reach back further than the
	// range when sampling paused, e.g., while the machine was suspended. Unix
	// times tell, since the monotonic clock that `Before` reads stops along
	// with the process.
	start := out.Start.UnixMilli()

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, g := range h.gauges {
		ring := &g.short
		if long {
			ring = &g.long
		}
		points := ring.ordered()
		series := HealthSeries{Key: g.Key, Points: make([][2]float64, 0, len(points)), Limit: g.Limit}
		for _, p := range points {
			if p.at.UnixMilli() < start {
				continue
			}
			series.Points = append(series.Points, [2]float64{float64(p.at.UnixMilli()), math.Round(p.value*100) / 100})
		}
		out.Series = append(out.Series, series)
	}
	return out
}

// storeGauges returns the gauges of the requests the store records and of the
// memory its traces take.
func storeGauges(store *Store) []Gauge {
	_, limit := store.MemoryUse()
	// The gauge of requests takes the interval that the gauges after it read.
	var in Interval
	perRequest := func(total *time.Duration) func() (float64, bool) {
		return func() (float64, bool) {
			if in.Requests == 0 {
				return 0, true
			}
			return ms(*total / time.Duration(in.Requests)), true
		}
	}
	return []Gauge{
		{Key: "requests", rate: true, Read: func() (float64, bool) {
			in = store.TakeInterval()
			return per(time.Second, float64(in.Requests), in.Elapsed)
		}},
		{Key: "failedRequests", rate: true, Read: func() (float64, bool) {
			return per(time.Minute, float64(in.Errors), in.Elapsed)
		}},
		{Key: "p50", Read: func() (float64, bool) { return ms(in.P50), true }},
		{Key: "p95", Read: func() (float64, bool) { return ms(in.P95), true }},
		{Key: "p99", Read: func() (float64, bool) { return ms(in.P99), true }},
		{Key: "dbTime", Read: perRequest(&in.DB)},
		{Key: "gitTime", Read: perRequest(&in.Git)},
		{Key: "otherTime", Read: perRequest(&in.Other)},
		{Key: "droppedSpans", rate: true, Read: func() (float64, bool) {
			return per(time.Minute, float64(in.DroppedSpans), in.Elapsed)
		}},
		{Key: "traceMemory", Limit: float64(limit), Read: func() (float64, bool) {
			used, _ := store.MemoryUse()
			return float64(used), true
		}},
	}
}

// runtimeHistogram reads what a histogram of the Go runtime collected between
// two samples of it.
type runtimeHistogram struct {
	last []uint64
}

// p99Ms returns the p99 of the durations the histogram collected since the
// last call, in milliseconds, and false when there is nothing to compare with.
func (h *runtimeHistogram) p99Ms(value metrics.Value) (float64, bool) {
	if value.Kind() != metrics.KindFloat64Histogram {
		return 0, false
	}
	hist := value.Float64Histogram()
	counts := append([]uint64(nil), hist.Counts...)
	previous := h.last
	h.last = counts
	if len(previous) != len(counts) {
		return 0, false
	}
	var total uint64
	for i := range counts {
		total += counts[i] - previous[i]
	}
	if total == 0 {
		return 0, true
	}
	rank := uint64(math.Ceil(0.99 * float64(total)))
	var seen uint64
	for i := range counts {
		seen += counts[i] - previous[i]
		if seen >= rank {
			// Buckets[i+1] is the upper bound of bucket i, which is infinite
			// for the last one.
			upper := hist.Buckets[i+1]
			if math.IsInf(upper, 1) {
				upper = hist.Buckets[i]
			}
			return upper * 1000, true
		}
	}
	return 0, true
}

func runtimeGauges() []Gauge {
	const (
		heap = iota
		gcPauses
		memory
		allocated
		gcCycles
		gcCPU
		cpu
		schedLatencies
	)
	samples := []metrics.Sample{
		heap:           {Name: "/memory/classes/heap/objects:bytes"},
		gcPauses:       {Name: "/sched/pauses/total/gc:seconds"},
		memory:         {Name: "/memory/classes/total:bytes"},
		allocated:      {Name: "/gc/heap/allocs:bytes"},
		gcCycles:       {Name: "/gc/cycles/total:gc-cycles"},
		gcCPU:          {Name: "/cpu/classes/gc/total:cpu-seconds"},
		cpu:            {Name: "/cpu/classes/total:cpu-seconds"},
		schedLatencies: {Name: "/sched/latencies:seconds"},
	}
	number := func(i int) (float64, bool) {
		if samples[i].Value.Kind() != metrics.KindUint64 {
			return 0, false
		}
		return float64(samples[i].Value.Uint64()), true
	}
	// growth is the gauge of how fast the count of a sample grows.
	growth := func(key string, i int, unit time.Duration) Gauge {
		var c counter
		return Gauge{Key: key, rate: true, Read: func() (float64, bool) {
			value, ok := number(i)
			if !ok {
				return 0, false
			}
			delta, elapsed, ok := c.take(value, time.Now())
			if !ok {
				return 0, false
			}
			return per(unit, delta, elapsed)
		}}
	}
	var pauses, latencies runtimeHistogram
	var lastGCCPU, lastCPU float64
	// The gauge of the heap reads the samples that the gauges after it use.
	return []Gauge{
		{Key: "heap", Read: func() (float64, bool) {
			metrics.Read(samples)
			return number(heap)
		}},
		{Key: "goMemory", Read: func() (float64, bool) { return number(memory) }},
		growth("allocRate", allocated, time.Second),
		{Key: "goroutines", Read: func() (float64, bool) {
			return float64(runtime.NumGoroutine()), true
		}},
		{Key: "gcPause", Read: func() (float64, bool) { return pauses.p99Ms(samples[gcPauses].Value) }},
		growth("gcCycles", gcCycles, time.Minute),
		// The share of the CPU time the process could use that the garbage
		// collector took since the last sample, in percent.
		{Key: "gcCPU", Read: func() (float64, bool) {
			if samples[gcCPU].Value.Kind() != metrics.KindFloat64 || samples[cpu].Value.Kind() != metrics.KindFloat64 {
				return 0, false
			}
			gc, total := samples[gcCPU].Value.Float64(), samples[cpu].Value.Float64()
			gcDelta, totalDelta := gc-lastGCCPU, total-lastCPU
			first := lastCPU == 0
			lastGCCPU, lastCPU = gc, total
			if first {
				return 0, false
			}
			if totalDelta <= 0 {
				return 0, true
			}
			return gcDelta / totalDelta * 100, true
		}},
		// How long goroutines waited to run once they could.
		{Key: "schedLatency", Read: func() (float64, bool) { return latencies.p99Ms(samples[schedLatencies].Value) }},
	}
}
