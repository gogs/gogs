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
	// rate is set for values per second, which are averaged over the long step
	// instead of taking the highest.
	rate bool
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

// NewHealth returns a Health of the given gauges, the request rate and p95 of
// the store when it is not nil, and the heap, goroutines, GC pauses and, where
// supported, open files of the process.
func NewHealth(store *Store, gauges ...Gauge) *Health {
	var all []Gauge
	if store != nil {
		var p95 time.Duration
		all = append(all,
			Gauge{Key: "requests", rate: true, Read: func() (float64, bool) {
				var n int
				var elapsed time.Duration
				n, elapsed, p95 = store.TakeInterval()
				if elapsed <= 0 {
					return 0, false
				}
				return float64(n) / elapsed.Seconds(), true
			}},
			// It reads what the gauge of requests took, which comes before it.
			Gauge{Key: "p95", Read: func() (float64, bool) { return ms(p95), true }},
		)
	}
	all = append(all, runtimeGauges()...)
	all = append(all, gauges...)
	if g, ok := openFilesGauge(); ok {
		all = append(all, g)
	}

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

func runtimeGauges() []Gauge {
	samples := []metrics.Sample{
		{Name: "/memory/classes/heap/objects:bytes"},
		{Name: "/sched/pauses/total/gc:seconds"},
	}
	var lastPauses []uint64
	read := func() {
		metrics.Read(samples)
	}
	return []Gauge{
		{Key: "heap", Read: func() (float64, bool) {
			read()
			if samples[0].Value.Kind() != metrics.KindUint64 {
				return 0, false
			}
			return float64(samples[0].Value.Uint64()), true
		}},
		{Key: "goroutines", Read: func() (float64, bool) {
			return float64(runtime.NumGoroutine()), true
		}},
		// The p99 of the GC pauses since the last sample, from the samples the
		// gauge of the heap read.
		{Key: "gcPause", Read: func() (float64, bool) {
			if samples[1].Value.Kind() != metrics.KindFloat64Histogram {
				return 0, false
			}
			hist := samples[1].Value.Float64Histogram()
			counts := append([]uint64(nil), hist.Counts...)
			previous := lastPauses
			lastPauses = counts
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
					// Buckets[i+1] is the upper bound of bucket i, which is
					// infinite for the last one.
					upper := hist.Buckets[i+1]
					if math.IsInf(upper, 1) {
						upper = hist.Buckets[i]
					}
					return upper * 1000, true
				}
			}
			return 0, true
		}},
	}
}
