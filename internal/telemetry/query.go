package telemetry

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"time"
)

const (
	maxSearchResults = 50
	maxSearchPoints  = 2000
	maxOverviewItems = 50
	// MinTraceIDPrefix is the fewest characters of a trace ID a search matches
	// by prefix.
	MinTraceIDPrefix = 8
)

// ms converts d to milliseconds, rounded to hundredths.
func ms(d time.Duration) float64 {
	return math.Round(float64(d)/float64(time.Millisecond)*100) / 100
}

type RouteStat struct {
	Route
	Requests int     `json:"requests"`
	Errors   int     `json:"errors"`
	P50Ms    float64 `json:"p50Ms"`
	P95Ms    float64 `json:"p95Ms"`
	P99Ms    float64 `json:"p99Ms"`
	TotalMs  float64 `json:"totalMs"`
}

type OperationStat struct {
	Name  string   `json:"name"`
	Kind  SpanKind `json:"kind"`
	Calls int      `json:"calls"`
	P50Ms float64  `json:"p50Ms"`
	P95Ms float64  `json:"p95Ms"`
	// SelfMs is the time spent in the operation itself, without the operations
	// it ran.
	SelfMs float64 `json:"selfMs"`
}

type OverviewBucket struct {
	Start    time.Time `json:"start"`
	Requests int       `json:"requests"`
	P95Ms    float64   `json:"p95Ms"`
}

// Overview aggregates every request of a range, not only the kept traces.
type Overview struct {
	Enabled       bool      `json:"enabled"`
	Now           time.Time `json:"now"`
	MaxAgeMinutes int       `json:"maxAgeMinutes"`
	Range         string    `json:"range"`
	Requests      int       `json:"requests"`
	Errors        int       `json:"errors"`
	P50Ms         float64   `json:"p50Ms"`
	P95Ms         float64   `json:"p95Ms"`
	P99Ms         float64   `json:"p99Ms"`
	TotalMs       float64   `json:"totalMs"`
	// SpanSelfMs is the time spent in every span of the range, each without the
	// spans it started, which is what the shares of operations are out of.
	SpanSelfMs float64          `json:"spanSelfMs"`
	Buckets    []OverviewBucket `json:"buckets"`
	Routes     []RouteStat      `json:"routes"`
	Operations []OperationStat  `json:"operations"`
}

const overviewBucketMinutes = 5

// Overview returns the stats of the last given minutes, at most an hour, which
// should be a multiple of five.
func (s *Store) Overview(minutes int, rangeName string) Overview {
	out := Overview{
		Range:      rangeName,
		Buckets:    []OverviewBucket{},
		Routes:     []RouteStat{},
		Operations: []OperationStat{},
	}
	if s == nil {
		return out
	}
	out.Enabled = true
	out.MaxAgeMinutes = s.maxAgeMinutes()
	minutes = min(minutes, rollupMinutes)

	s.mu.Lock()
	defer s.mu.Unlock()

	out.Now = s.now().UTC()
	to := out.Now.Unix() / 60
	from := to - int64(minutes) + 1

	all := s.requests.sum(from, to)
	out.Requests = all.count
	out.Errors = all.errors
	out.P50Ms = ms(all.hist.quantile(0.5))
	out.P95Ms = ms(all.hist.quantile(0.95))
	out.P99Ms = ms(all.hist.quantile(0.99))
	out.TotalMs = ms(all.total)

	for start := from; start <= to; start += overviewBucketMinutes {
		b := s.requests.sum(start, start+overviewBucketMinutes-1)
		out.Buckets = append(out.Buckets, OverviewBucket{
			Start:    time.Unix(start*60, 0).UTC(),
			Requests: b.count,
			P95Ms:    ms(b.hist.quantile(0.95)),
		})
	}

	var spanSelf time.Duration
	for _, rs := range s.routes {
		st := rs.sum(from, to)
		if st.count == 0 {
			continue
		}
		spanSelf += st.self
		out.Routes = append(out.Routes, RouteStat{
			Route:    rs.route,
			Requests: st.count,
			Errors:   st.errors,
			P50Ms:    ms(st.hist.quantile(0.5)),
			P95Ms:    ms(st.hist.quantile(0.95)),
			P99Ms:    ms(st.hist.quantile(0.99)),
			TotalMs:  ms(st.total),
		})
	}
	slices.SortFunc(out.Routes, func(a, b RouteStat) int {
		return cmp.Or(cmp.Compare(b.TotalMs, a.TotalMs), strings.Compare(a.Name, b.Name))
	})
	out.Routes = out.Routes[:min(len(out.Routes), maxOverviewItems)]

	for name, os := range s.operations {
		st := os.sum(from, to)
		if st.count == 0 {
			continue
		}
		spanSelf += st.self
		out.Operations = append(out.Operations, OperationStat{
			Name:   name,
			Kind:   os.kind,
			Calls:  st.count,
			P50Ms:  ms(st.hist.quantile(0.5)),
			P95Ms:  ms(st.hist.quantile(0.95)),
			SelfMs: ms(st.self),
		})
	}
	out.SpanSelfMs = ms(spanSelf)
	slices.SortFunc(out.Operations, func(a, b OperationStat) int {
		return cmp.Or(cmp.Compare(b.SelfMs, a.SelfMs), strings.Compare(a.Name, b.Name))
	})
	out.Operations = out.Operations[:min(len(out.Operations), maxOverviewItems)]
	return out
}

type TraceSummary struct {
	ID         string     `json:"id"`
	Route      Route      `json:"route"`
	Method     string     `json:"method"`
	Path       string     `json:"path"`
	Status     int        `json:"status"`
	Start      time.Time  `json:"start"`
	DurationMs float64    `json:"durationMs"`
	SpanCount  int        `json:"spanCount"`
	Error      bool       `json:"error"`
	Kept       KeptReason `json:"kept"`
}

func (t *storedTrace) summary() TraceSummary {
	return TraceSummary{
		ID:         t.id,
		Route:      t.route,
		Method:     t.method,
		Path:       t.path,
		Status:     t.status,
		Start:      t.start,
		DurationMs: ms(t.duration),
		SpanCount:  len(t.spans),
		Error:      t.failed,
		Kept:       t.kept,
	}
}

type SearchPoint struct {
	ID         string    `json:"id"`
	Start      time.Time `json:"start"`
	DurationMs float64   `json:"durationMs"`
	Status     int       `json:"status"`
	Error      bool      `json:"error"`
}

type SearchQuery struct {
	// Q is a trace ID, or at least its first MinTraceIDPrefix characters. The
	// other filters are ignored when it is set.
	Q     string
	Route string
	MinMs float64
	// Status is "5xx", "4xx", or empty for all.
	Status  string
	Slowest bool
	From    time.Time
	To      time.Time
}

type SearchResult struct {
	Enabled       bool           `json:"enabled"`
	Now           time.Time      `json:"now"`
	Traces        []TraceSummary `json:"traces"`
	Total         int            `json:"total"`
	Points        []SearchPoint  `json:"points"`
	Routes        []Route        `json:"routes"`
	Oldest        *time.Time     `json:"oldest"`
	MaxAgeMinutes int            `json:"maxAgeMinutes"`
	// SlowThresholdMs is the duration from which a request is always kept, or
	// 0 when only failed requests and the slowest of each route are.
	SlowThresholdMs float64 `json:"slowThresholdMs"`
}

func (q *SearchQuery) match(t *storedTrace) bool {
	// Search lowercases and trims Q.
	if q.Q != "" {
		return len(q.Q) >= MinTraceIDPrefix && strings.HasPrefix(t.id, q.Q)
	}
	if q.Route != "" && t.route.Name != q.Route {
		return false
	}
	if q.MinMs > 0 && ms(t.duration) < q.MinMs {
		return false
	}
	switch q.Status {
	case "5xx":
		if !t.failed {
			return false
		}
	case "4xx":
		if t.status < 400 || t.status >= 500 {
			return false
		}
	}
	if !q.From.IsZero() && t.start.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && t.start.After(q.To) {
		return false
	}
	return true
}

// maxAgeMinutes returns MaxAge in whole minutes, at least one.
func (s *Store) maxAgeMinutes() int {
	return max(int(math.Ceil(s.opts.MaxAge.Minutes())), 1)
}

// Search returns the kept traces that match q, the most recent first unless q
// asks for the slowest.
func (s *Store) Search(q SearchQuery) SearchResult {
	out := SearchResult{
		Traces: []TraceSummary{},
		Points: []SearchPoint{},
		Routes: []Route{},
	}
	if s == nil {
		return out
	}
	out.Enabled = true
	out.MaxAgeMinutes = s.maxAgeMinutes()
	out.SlowThresholdMs = ms(s.opts.SlowThreshold)
	q.Q = strings.ToLower(strings.TrimSpace(q.Q))

	// Kept traces do not change, so they are sorted and summed up after the
	// lock is released, which every request takes to record its spans.
	s.mu.Lock()
	s.evict()
	out.Now = s.now().UTC()

	var matches []*storedTrace
	routes := make(map[string]Route)
	for _, r := range []*ring{&s.kept, &s.sampled} {
		for _, t := range r.traces {
			routes[t.route.Name] = t.route
			if out.Oldest == nil || t.start.Before(*out.Oldest) {
				start := t.start
				out.Oldest = &start
			}
			// Only the latest trace with an ID can be opened, e.g., when a client
			// sent the same trace ID with two requests.
			if s.byID[t.id] == t && q.match(t) {
				matches = append(matches, t)
			}
		}
	}
	// The route of the query is listed even when none of its traces is kept,
	// e.g., when it comes from the overview.
	if rs := s.routes[q.Route]; rs != nil {
		routes[q.Route] = rs.route
	}
	s.mu.Unlock()

	for _, r := range routes {
		out.Routes = append(out.Routes, r)
	}
	slices.SortFunc(out.Routes, func(a, b Route) int { return strings.Compare(a.Name, b.Name) })

	slices.SortFunc(matches, func(a, b *storedTrace) int {
		if q.Slowest {
			if c := cmp.Compare(b.duration, a.duration); c != 0 {
				return c
			}
		}
		return cmp.Or(b.start.Compare(a.start), strings.Compare(a.id, b.id))
	})
	out.Total = len(matches)
	for _, t := range matches[:min(len(matches), maxSearchResults)] {
		out.Traces = append(out.Traces, t.summary())
	}

	// The points spread over the time of every match, whatever the order of the
	// list, so that the chart does not look empty where no point fits.
	points := slices.Clone(matches)
	slices.SortFunc(points, func(a, b *storedTrace) int { return a.start.Compare(b.start) })
	n := min(len(points), maxSearchPoints)
	for i := range n {
		t := points[i*len(points)/n]
		out.Points = append(out.Points, SearchPoint{
			ID:         t.id,
			Start:      t.start,
			DurationMs: ms(t.duration),
			Status:     t.status,
			Error:      t.failed,
		})
	}
	return out
}

type Span struct {
	ID         string      `json:"id"`
	ParentID   string      `json:"parentId"`
	Name       string      `json:"name"`
	Kind       SpanKind    `json:"kind"`
	StartMs    float64     `json:"startMs"`
	DurationMs float64     `json:"durationMs"`
	Error      string      `json:"error"`
	Attributes []Attribute `json:"attributes"`
}

type TraceRouteStats struct {
	Requests int     `json:"requests"`
	P50Ms    float64 `json:"p50Ms"`
	P95Ms    float64 `json:"p95Ms"`
	// FasterPercent is the share of the requests of the route that were faster
	// than this trace.
	FasterPercent float64 `json:"fasterPercent"`
}

type TraceOperationStats struct {
	Calls int     `json:"calls"`
	P50Ms float64 `json:"p50Ms"`
	P95Ms float64 `json:"p95Ms"`
}

type Trace struct {
	TraceSummary
	Spans        []Span                         `json:"spans"`
	DroppedSpans int                            `json:"droppedSpans"`
	RouteStats   *TraceRouteStats               `json:"routeStats"`
	Operations   map[string]TraceOperationStats `json:"operations"`
}

// Get returns the kept trace with the given ID, compared to the last hour of
// its route and operations.
func (s *Store) Get(id string) (*Trace, bool) {
	if s == nil {
		return nil, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evict()

	t := s.byID[strings.ToLower(id)]
	if t == nil {
		return nil, false
	}

	out := &Trace{
		TraceSummary: t.summary(),
		Spans:        make([]Span, 0, len(t.spans)),
		DroppedSpans: t.dropped,
		Operations:   make(map[string]TraceOperationStats),
	}
	for i := range t.spans {
		sp := &t.spans[i]
		attrs := sp.attrs
		if attrs == nil {
			attrs = []Attribute{}
		}
		out.Spans = append(out.Spans, Span{
			ID:         sp.id,
			ParentID:   sp.parentID,
			Name:       sp.name,
			Kind:       sp.kind,
			StartMs:    ms(sp.start.Sub(t.start)),
			DurationMs: ms(sp.duration),
			Error:      sp.err,
			Attributes: attrs,
		})
	}
	// The root of a trace that continues a remote one has a parent that is not
	// in the trace.
	out.Spans[0].ParentID = ""

	// The rollups only cover the last hour, which says nothing about how usual
	// an older trace was.
	to := s.now().Unix() / 60
	from := to - rollupMinutes + 1
	if t.end().Unix()/60 < from {
		return out, true
	}
	if rs := s.routes[t.route.Name]; rs != nil {
		st := rs.sum(from, to)
		if st.count > 0 {
			out.RouteStats = &TraceRouteStats{
				Requests:      st.count,
				P50Ms:         ms(st.hist.quantile(0.5)),
				P95Ms:         ms(st.hist.quantile(0.95)),
				FasterPercent: math.Round(float64(st.hist.below(t.duration))/float64(st.count)*1000) / 10,
			}
		}
	}
	for i := 1; i < len(t.spans); i++ {
		name := t.spans[i].name
		if _, ok := out.Operations[name]; ok {
			continue
		}
		os := s.operations[name]
		if os == nil {
			continue
		}
		st := os.sum(from, to)
		if st.count == 0 {
			continue
		}
		out.Operations[name] = TraceOperationStats{
			Calls: st.count,
			P50Ms: ms(st.hist.quantile(0.5)),
			P95Ms: ms(st.hist.quantile(0.95)),
		}
	}
	return out, true
}
