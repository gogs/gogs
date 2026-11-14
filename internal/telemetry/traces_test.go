package telemetry

import (
	"context"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

var testNow = time.Date(2026, 11, 14, 12, 30, 0, 0, time.UTC)

func newTestStore(t *testing.T, opts Options) (*Store, trace.Tracer) {
	t.Helper()
	store := NewStore(opts)
	store.now = func() time.Time { return testNow }
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(store))
	t.Cleanup(func() { _ = provider.Shutdown(context.Background()) })
	return store, provider.Tracer("test")
}

type testRequest struct {
	route    string
	status   int
	start    time.Time
	duration time.Duration
	// git is how long a Git command in the middle of the request takes.
	git time.Duration
}

// record records a request as the HTTP handler does, and returns its trace ID.
func record(tracer trace.Tracer, r testRequest) string {
	if r.start.IsZero() {
		r.start = testNow.Add(-time.Second)
	}
	attrs := []attribute.KeyValue{
		attribute.String("http.request.method", "GET"),
		attribute.String("url.path", "/some/path"),
		attribute.Int("http.response.status_code", r.status),
	}
	if r.route != "" {
		attrs = append(attrs, attribute.String("http.route", r.route))
	}
	ctx, root := tracer.Start(context.Background(), "GET "+r.route,
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(r.start),
		trace.WithAttributes(attrs...),
	)
	if r.git > 0 {
		gitStart := r.start.Add((r.duration - r.git) / 2)
		_, git := tracer.Start(ctx, "git log", trace.WithTimestamp(gitStart))
		git.End(trace.WithTimestamp(gitStart.Add(r.git)))
	}
	if r.status >= 500 {
		root.SetStatus(codes.Error, "")
	}
	root.End(trace.WithTimestamp(r.start.Add(r.duration)))
	return root.SpanContext().TraceID().String()
}

func defaultOptions() Options {
	return Options{
		MaxMemory:                1 << 20,
		MaxAge:                   time.Hour,
		SlowThreshold:            time.Second,
		SamplesPerRoutePerMinute: 1,
	}
}

func TestStore_KeepsFailedTraceThroughFlood(t *testing.T) {
	opts := defaultOptions()
	opts.MaxMemory = 64 << 10
	store, tracer := newTestStore(t, opts)

	failed := record(tracer, testRequest{route: "/{owner}/{repo}", status: 500, duration: 30 * time.Millisecond})
	// Fast requests on many routes, each of which is sampled once.
	for i := range 2000 {
		record(tracer, testRequest{route: "/route/" + string(rune('a'+i%26)) + string(rune('a'+i/26%26)), status: 200, duration: time.Millisecond})
	}

	got, ok := store.Get(failed)
	require.True(t, ok, "the failed trace was evicted by sampled ones")
	assert.Equal(t, KeptFailed, got.Kept)
	assert.Equal(t, 500, got.Status)
	assert.LessOrEqual(t, store.sampled.size, store.sampled.budget)
	assert.LessOrEqual(t, store.kept.size, store.kept.budget)
}

func TestStore_KeepRules(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	first := record(tracer, testRequest{route: "/a", status: 200, duration: 5 * time.Millisecond})
	second := record(tracer, testRequest{route: "/a", status: 200, duration: 5 * time.Millisecond})
	slow := record(tracer, testRequest{route: "/a", status: 200, duration: 1500 * time.Millisecond})
	notFound := record(tracer, testRequest{route: "/a", status: 404, duration: 5 * time.Millisecond})

	got, ok := store.Get(first)
	require.True(t, ok)
	assert.Equal(t, KeptSampled, got.Kept)

	_, ok = store.Get(second)
	assert.False(t, ok, "only one request of a route is sampled per minute")

	got, ok = store.Get(slow)
	require.True(t, ok)
	assert.Equal(t, KeptSlow, got.Kept)

	_, ok = store.Get(notFound)
	assert.False(t, ok, "a 4xx response is not a failure of the server")

	// The rollups count every request, not only the kept ones.
	overview := store.Overview(15, "15m")
	assert.Equal(t, 4, overview.Requests)
	assert.Equal(t, 0, overview.Errors)
}

func TestStore_KeepsSlowForRoute(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	for range 200 {
		record(tracer, testRequest{route: "/a", status: 200, duration: 10 * time.Millisecond})
	}
	// Well below the threshold of a second, but far slower than the route.
	outlier := record(tracer, testRequest{route: "/a", status: 200, duration: 80 * time.Millisecond})
	usual := record(tracer, testRequest{route: "/a", status: 200, duration: 10 * time.Millisecond})

	got, ok := store.Get(outlier)
	require.True(t, ok)
	assert.Equal(t, KeptSlow, got.Kept)
	_, ok = store.Get(usual)
	assert.False(t, ok)
}

func TestStore_Routes(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	route := record(tracer, testRequest{route: "/api/web/{owner}/{repo}/header", status: 200, duration: time.Millisecond})
	other := record(tracer, testRequest{status: 200, duration: time.Millisecond})

	ctx, background := tracer.Start(context.Background(), "mirror sync", trace.WithTimestamp(testNow.Add(-time.Second)))
	_, git := tracer.Start(ctx, "git remote update", trace.WithTimestamp(testNow.Add(-time.Second)))
	git.End(trace.WithTimestamp(testNow))
	background.End(trace.WithTimestamp(testNow))

	got, ok := store.Get(route)
	require.True(t, ok)
	assert.Equal(t, Route{Name: "GET /api/web/{owner}/{repo}/header", Kind: RouteKindRoute, Method: "GET"}, got.Route)

	got, ok = store.Get(other)
	require.True(t, ok)
	assert.Equal(t, Route{Name: "GET", Kind: RouteKindOther, Method: "GET"}, got.Route)

	got, ok = store.Get(background.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Equal(t, Route{Name: "mirror sync", Kind: RouteKindBackground}, got.Route)
	require.Len(t, got.Spans, 2)
	assert.Equal(t, SpanKindGit, got.Spans[1].Kind)

	// Background work is not a request.
	assert.Equal(t, 2, store.Overview(15, "15m").Requests)
}

func TestStore_SelfTime(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	id := record(tracer, testRequest{route: "/a", status: 200, duration: 100 * time.Millisecond, git: 70 * time.Millisecond})

	overview := store.Overview(15, "15m")
	// The request itself took 30 ms besides the 70 ms of Git.
	assert.Equal(t, 100.0, overview.SpanSelfMs)
	require.Len(t, overview.Operations, 1)
	assert.Equal(t, OperationStat{Name: "git log", Kind: SpanKindGit, Calls: 1, P50Ms: overview.Operations[0].P50Ms, P95Ms: overview.Operations[0].P95Ms, SelfMs: 70}, overview.Operations[0])

	got, ok := store.Get(id)
	require.True(t, ok)
	require.Len(t, got.Spans, 2)
	assert.Equal(t, "", got.Spans[0].ParentID)
	assert.Equal(t, got.Spans[0].ID, got.Spans[1].ParentID)
	assert.Equal(t, 15.0, got.Spans[1].StartMs)
	assert.Equal(t, 70.0, got.Spans[1].DurationMs)
}

func TestStore_Search(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	var ids []string
	for i := range 300 {
		ids = append(ids, record(tracer, testRequest{route: "/a", status: 500, duration: time.Duration(i+1) * time.Millisecond}))
	}
	got := store.Search(SearchQuery{Q: "  " + strings.ToUpper(ids[5]) + " "})
	require.Equal(t, 1, got.Total)
	assert.Equal(t, ids[5], got.Traces[0].ID)

	got = store.Search(SearchQuery{Q: ids[5][:MinTraceIDPrefix]})
	require.Equal(t, 1, got.Total)
	assert.Equal(t, ids[5], got.Traces[0].ID)

	got = store.Search(SearchQuery{Q: ids[5][:MinTraceIDPrefix-1]})
	assert.Equal(t, 0, got.Total, "a prefix shorter than the minimum matches nothing")

	got = store.Search(SearchQuery{Status: "5xx", Slowest: true})
	assert.Equal(t, 300, got.Total)
	assert.Len(t, got.Traces, maxSearchResults)
	assert.Equal(t, 300.0, got.Traces[0].DurationMs)
}

func TestStore_SearchAmbiguousPrefix(t *testing.T) {
	store, _ := newTestStore(t, defaultOptions())
	for _, id := range []string{
		"0123456789abcdef0000000000000001",
		"0123456789abcdef0000000000000002",
		"fedcba98765432100000000000000003",
	} {
		tr := &storedTrace{id: id, route: Route{Name: "/a"}, start: testNow, kept: KeptFailed, failed: true, spans: []span{{}}, size: 1}
		store.kept.push(tr)
		store.byID[id] = tr
	}

	got := store.Search(SearchQuery{Q: "0123456789ABCDEF"})
	assert.Equal(t, 2, got.Total)

	got = store.Search(SearchQuery{Q: "fedcba98"})
	assert.Equal(t, 1, got.Total)
}

func TestStore_MaxAge(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	old := record(tracer, testRequest{route: "/a", status: 500, start: testNow.Add(-2 * time.Hour), duration: time.Millisecond})
	recent := record(tracer, testRequest{route: "/a", status: 500, duration: time.Millisecond})

	_, ok := store.Get(old)
	assert.False(t, ok)
	_, ok = store.Get(recent)
	assert.True(t, ok)
}

func TestStore_MemoryCap(t *testing.T) {
	opts := defaultOptions()
	opts.MaxMemory = 256 << 10
	store, tracer := newTestStore(t, opts)

	var first, last string
	for i := range 5000 {
		last = record(tracer, testRequest{route: "/a", status: 500, duration: time.Duration(i) * time.Microsecond, git: time.Duration(i) * time.Microsecond / 2})
		if i == 0 {
			first = last
		}
	}
	assert.LessOrEqual(t, store.kept.size, store.kept.budget)
	assert.Greater(t, store.kept.size, store.kept.budget*3/4, "the budget is used")
	_, ok := store.Get(first)
	assert.False(t, ok, "the oldest traces are evicted")
	_, ok = store.Get(last)
	assert.True(t, ok, "the most recent traces are kept")
	assert.Len(t, store.byID, len(store.kept.traces)+len(store.sampled.traces))
}

func TestHistogram_Quantile(t *testing.T) {
	// A skewed distribution: mostly fast, with a long tail.
	r := rand.New(rand.NewPCG(1, 2))
	var values []time.Duration
	h := histogram{}
	for range 10000 {
		d := time.Duration(r.ExpFloat64() * float64(5*time.Millisecond))
		if r.IntN(100) == 0 {
			d += time.Duration(r.IntN(5000)) * time.Millisecond
		}
		values = append(values, d)
		h.add(d)
	}
	slices.Sort(values)
	for _, q := range []float64{0.5, 0.95, 0.99} {
		want := values[int(q*float64(len(values)))-1]
		got := h.quantile(q)
		assert.InEpsilon(t, float64(want), float64(got), 0.05, "q=%v want %v got %v", q, want, got)
	}
	assert.Equal(t, time.Duration(0), histogram{}.quantile(0.5))
}

func TestNilStore(t *testing.T) {
	var store *Store
	assert.False(t, store.Overview(15, "15m").Enabled)
	assert.False(t, store.Search(SearchQuery{}).Enabled)
	_, ok := store.Get("0123456789abcdef")
	assert.False(t, ok)
}
