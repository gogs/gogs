package telemetry

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const (
	// maxPendingTraces bounds the traces whose spans are still being recorded.
	maxPendingTraces = 10_000
	// pendingMaxAge is how long a trace waits for its root span to end before
	// it is dropped, e.g., when the root never ends.
	pendingMaxAge        = 5 * time.Minute
	pendingSweepInterval = time.Minute
	maxSpansPerTrace     = 1000
	maxAttributes        = 32
	maxValueBytes        = 1024
	maxRoutes            = 500
	maxOperations        = 1000
	// slowForRouteWindow and slowForRouteMinRequests decide when a request is
	// slow for its route: slower than the p99 of the route in the window, once
	// the route had enough requests in it.
	slowForRouteWindow      = 15
	slowForRouteMinRequests = 100
)

// SpanKind is what a span measures.
type SpanKind string

const (
	SpanKindRequest SpanKind = "request"
	SpanKindGit     SpanKind = "git"
	SpanKindOther   SpanKind = "other"
)

// RouteKind tells what a route is.
type RouteKind string

const (
	// RouteKindRoute is a matched route pattern, e.g.,
	// "GET /api/web/{owner}/{repo}/header".
	RouteKindRoute RouteKind = "route"
	// RouteKindOther groups the requests whose route is not known, e.g., pages
	// served by Macaron, under their method.
	RouteKindOther RouteKind = "other"
	// RouteKindBackground is work that no request started, e.g., a mirror sync,
	// named after what it ran.
	RouteKindBackground RouteKind = "background"
)

// Route is what the traces of a route are grouped under.
type Route struct {
	Name   string    `json:"name"`
	Kind   RouteKind `json:"kind"`
	Method string    `json:"method"`
}

// KeptReason tells why a trace was kept.
type KeptReason string

const (
	KeptFailed  KeptReason = "failed"
	KeptSlow    KeptReason = "slow"
	KeptSampled KeptReason = "sampled"
)

// Options configure a Store.
type Options struct {
	// MaxMemory is about how many bytes the kept traces take: three quarters
	// for failed and slow ones, the rest for samples of the others.
	MaxMemory int64
	// MaxAge is how long a trace is kept at most.
	MaxAge time.Duration
	// SlowThreshold is the duration from which a request is always kept.
	SlowThreshold time.Duration
	// SamplesPerRoutePerMinute is how many of the other requests of a route are
	// kept every minute.
	SamplesPerRoutePerMinute int
}

type Attribute struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type span struct {
	id       string
	parentID string
	name     string
	kind     SpanKind
	start    time.Time
	duration time.Duration
	err      string
	attrs    []Attribute
}

func (s *span) attr(key string) string {
	for _, a := range s.attrs {
		if a.Key == key {
			return a.Value
		}
	}
	return ""
}

type storedTrace struct {
	id       string
	route    Route
	method   string
	path     string
	status   int
	start    time.Time
	duration time.Duration
	failed   bool
	kept     KeptReason
	// spans are ordered by start, the root first.
	spans   []span
	dropped int
	size    int64
}

func (t *storedTrace) end() time.Time {
	return t.start.Add(t.duration)
}

type pendingTrace struct {
	spans     []span
	dropped   int
	openRoots int
	started   time.Time
	// root is the last local root span that ended, i.e., the outermost.
	root *span
}

// ring keeps traces in the order they ended, within a budget of bytes.
type ring struct {
	budget int64
	size   int64
	traces []*storedTrace
}

type routeSeries struct {
	route Route
	series
}

type operationSeries struct {
	kind SpanKind
	series
}

type sampleCount struct {
	minute int64
	n      int
}

// Store is a span processor that keeps the traces of failed requests, slow
// requests and a sample of the others in memory, and rolls up every span into
// per-minute stats. All its methods are safe to call on a nil Store, which
// keeps nothing.
type Store struct {
	opts Options
	now  func() time.Time

	mu      sync.Mutex
	pending map[trace.TraceID]*pendingTrace
	// pendingSweptAt is when expired pending traces were last dropped.
	pendingSweptAt time.Time
	kept           ring
	sampled        ring
	byID           map[string]*storedTrace
	requests       series
	routes         map[string]*routeSeries
	operations     map[string]*operationSeries
	samples        map[string]sampleCount
	// interval collects the requests since the last call to TakeInterval.
	interval      histogram
	intervalStart time.Time
}

var _ sdktrace.SpanProcessor = (*Store)(nil)

// NewStore returns a Store with the given options.
func NewStore(opts Options) *Store {
	now := time.Now()
	return &Store{
		opts:          opts,
		now:           time.Now,
		pending:       make(map[trace.TraceID]*pendingTrace),
		kept:          ring{budget: opts.MaxMemory * 3 / 4},
		sampled:       ring{budget: opts.MaxMemory / 4},
		byID:          make(map[string]*storedTrace),
		routes:        make(map[string]*routeSeries),
		operations:    make(map[string]*operationSeries),
		samples:       make(map[string]sampleCount),
		interval:      histogram{},
		intervalStart: now,
	}
}

func isLocalRoot(parent trace.SpanContext) bool {
	return !parent.IsValid() || parent.IsRemote()
}

func (s *Store) OnStart(_ context.Context, sp sdktrace.ReadWriteSpan) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	id := sp.SpanContext().TraceID()
	p := s.pending[id]
	if p == nil {
		now := s.now()
		if now.Sub(s.pendingSweptAt) >= pendingSweepInterval {
			s.dropExpiredPending(now)
		}
		if len(s.pending) >= maxPendingTraces {
			return
		}
		p = &pendingTrace{started: now}
		s.pending[id] = p
	}
	if isLocalRoot(sp.Parent()) {
		p.openRoots++
	}
}

// dropExpiredPending drops the traces without an open root that wait longer
// than pendingMaxAge, e.g., of a span that started after its root ended. A
// trace with an open root waits for it, e.g., of a long clone.
func (s *Store) dropExpiredPending(now time.Time) {
	s.pendingSweptAt = now
	cutoff := now.Add(-pendingMaxAge)
	for id, p := range s.pending {
		if p.openRoots <= 0 && p.started.Before(cutoff) {
			delete(s.pending, id)
		}
	}
}

func copySpan(sp sdktrace.ReadOnlySpan) span {
	out := span{
		id:       sp.SpanContext().SpanID().String(),
		name:     sp.Name(),
		start:    sp.StartTime(),
		duration: sp.EndTime().Sub(sp.StartTime()),
	}
	if sp.Parent().IsValid() {
		out.parentID = sp.Parent().SpanID().String()
	}
	if status := sp.Status(); status.Code == codes.Error {
		out.err = status.Description
		if out.err == "" {
			out.err = "error"
		}
	}
	switch {
	case sp.SpanKind() == trace.SpanKindServer:
		out.kind = SpanKindRequest
	case strings.HasPrefix(out.name, "git ") || strings.HasPrefix(out.name, "git."):
		out.kind = SpanKindGit
	default:
		out.kind = SpanKindOther
	}
	attrs := sp.Attributes()
	if len(attrs) > maxAttributes {
		attrs = attrs[:maxAttributes]
	}
	out.attrs = make([]Attribute, 0, len(attrs))
	for _, kv := range attrs {
		value := kv.Value.Emit()
		if len(value) > maxValueBytes {
			value = value[:maxValueBytes] + "…"
		}
		out.attrs = append(out.attrs, Attribute{Key: string(kv.Key), Value: value})
	}
	return out
}

func (s *Store) OnEnd(sp sdktrace.ReadOnlySpan) {
	if s == nil {
		return
	}
	copied := copySpan(sp)

	s.mu.Lock()
	defer s.mu.Unlock()

	id := sp.SpanContext().TraceID()
	p := s.pending[id]
	if p == nil {
		return
	}
	localRoot := isLocalRoot(sp.Parent())
	// The root that ends a trace ends after its children, so it is kept past
	// the limit. Other roots are not, e.g., of requests that a client sent with
	// the same trace ID.
	if len(p.spans) < maxSpansPerTrace || (localRoot && p.openRoots == 1) {
		p.spans = append(p.spans, copied)
	} else {
		p.dropped++
	}
	if !localRoot {
		return
	}
	p.root = &copied
	p.openRoots--
	if p.openRoots <= 0 {
		delete(s.pending, id)
		s.finish(id.String(), p)
	}
}

func (s *Store) Shutdown(context.Context) error   { return nil }
func (s *Store) ForceFlush(context.Context) error { return nil }

// routeOf returns what the trace rooted at root is grouped under.
func routeOf(root *span) Route {
	if root.kind != SpanKindRequest {
		return Route{Name: root.name, Kind: RouteKindBackground}
	}
	method := root.attr("http.request.method")
	if method == "" {
		method, _, _ = strings.Cut(root.name, " ")
	}
	if route := root.attr("http.route"); route != "" {
		return Route{Name: method + " " + route, Kind: RouteKindRoute, Method: method}
	}
	return Route{Name: method, Kind: RouteKindOther, Method: method}
}

func spanSize(sp *span) int64 {
	size := int64(200 + len(sp.name) + len(sp.err))
	for _, a := range sp.attrs {
		size += int64(32 + len(a.Key) + len(a.Value))
	}
	return size
}

func (s *Store) finish(id string, p *pendingTrace) {
	root := p.root
	spans := p.spans
	// Put the root first, then the others by start.
	slices.SortStableFunc(spans, func(a, b span) int {
		switch {
		case a.id == root.id:
			return -1
		case b.id == root.id:
			return 1
		}
		return a.start.Compare(b.start)
	})

	childTime := make(map[string]time.Duration, len(spans))
	for i := range spans {
		if spans[i].parentID != "" {
			childTime[spans[i].parentID] += spans[i].duration
		}
	}
	selfTime := func(sp *span) time.Duration {
		return max(sp.duration-childTime[sp.id], 0)
	}

	t := &storedTrace{
		id:       id,
		route:    routeOf(root),
		method:   root.attr("http.request.method"),
		path:     root.attr("url.path"),
		start:    root.start,
		duration: root.duration,
		spans:    spans,
		dropped:  p.dropped,
		size:     300,
	}
	t.status, _ = strconv.Atoi(root.attr("http.response.status_code"))
	t.failed = root.err != "" || t.status >= 500
	for i := range spans {
		t.size += spanSize(&spans[i])
	}
	end := t.end()

	rs := s.routes[t.route.Name]
	if rs == nil {
		if len(s.routes) >= maxRoutes {
			if t.route.Kind == RouteKindBackground {
				t.route = Route{Name: "other", Kind: RouteKindBackground}
			} else {
				t.route = Route{Name: t.route.Method, Kind: RouteKindOther, Method: t.route.Method}
			}
			rs = s.routes[t.route.Name]
		}
		if rs == nil {
			rs = &routeSeries{route: t.route}
			s.routes[t.route.Name] = rs
		}
	}

	t.kept = s.keepReason(t, rs)

	rs.add(end, t.duration, selfTime(root), t.failed)
	if t.route.Kind != RouteKindBackground {
		s.requests.add(end, t.duration, selfTime(root), t.failed)
		s.interval.add(t.duration)
	}
	for i := 1; i < len(spans); i++ {
		sp := &spans[i]
		os := s.operations[sp.name]
		if os == nil {
			if len(s.operations) >= maxOperations {
				continue
			}
			os = &operationSeries{kind: sp.kind}
			s.operations[sp.name] = os
		}
		os.add(end, sp.duration, selfTime(sp), sp.err != "")
	}

	var r *ring
	switch t.kept {
	case KeptFailed, KeptSlow:
		r = &s.kept
	case KeptSampled:
		r = &s.sampled
	default:
		return
	}
	// A trace larger than its whole budget would push out every other one.
	if t.size > r.budget {
		return
	}
	r.push(t)
	s.byID[id] = t
	s.evict()
}

// keepReason returns why t is kept, or "" when it is dropped. It runs before t
// is added to the rollups, so that t does not count towards the p99 of its
// route.
func (s *Store) keepReason(t *storedTrace, rs *routeSeries) KeptReason {
	if t.failed {
		return KeptFailed
	}
	if s.opts.SlowThreshold > 0 && t.duration >= s.opts.SlowThreshold {
		return KeptSlow
	}
	minute := t.end().Unix() / 60
	recent := rs.sum(minute-slowForRouteWindow+1, minute)
	// The p99 is the middle of its bucket, so compare buckets: most requests of a
	// route with steady latency fall in the bucket of its p99, half of them above
	// its middle.
	if recent.count >= slowForRouteMinRequests && histogramBucket(t.duration) > histogramBucket(recent.hist.quantile(0.99)) {
		return KeptSlow
	}
	count := s.samples[t.route.Name]
	if count.minute != minute {
		count = sampleCount{minute: minute}
	}
	if count.n >= s.opts.SamplesPerRoutePerMinute {
		return ""
	}
	count.n++
	s.samples[t.route.Name] = count
	return KeptSampled
}

func (r *ring) push(t *storedTrace) {
	r.traces = append(r.traces, t)
	r.size += t.size
}

func (r *ring) evict(cutoff time.Time, byID map[string]*storedTrace) {
	n := 0
	for n < len(r.traces) && (r.size > r.budget || r.traces[n].end().Before(cutoff)) {
		t := r.traces[n]
		r.size -= t.size
		// A later trace with the same ID may have replaced this one.
		if byID[t.id] == t {
			delete(byID, t.id)
		}
		r.traces[n] = nil
		n++
	}
	r.traces = r.traces[n:]
}

func (s *Store) evict() {
	cutoff := s.now().Add(-s.opts.MaxAge)
	s.kept.evict(cutoff, s.byID)
	s.sampled.evict(cutoff, s.byID)
}

// TakeInterval returns how many requests finished since the last call, how
// long ago that was, and their p95.
func (s *Store) TakeInterval() (requests int, elapsed, p95 time.Duration) {
	if s == nil {
		return 0, 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	requests, elapsed, p95 = s.interval.count(), now.Sub(s.intervalStart), s.interval.quantile(0.95)
	s.interval = histogram{}
	s.intervalStart = now
	return requests, elapsed, p95
}
