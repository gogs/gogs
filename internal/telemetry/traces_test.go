package telemetry

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
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

	// Usual requests take 10 to 10.5 ms, within one bucket of the histogram.
	usualDuration := func(i int) time.Duration {
		return 10*time.Millisecond + time.Duration(i%50)*10*time.Microsecond
	}
	for i := range 200 {
		record(tracer, testRequest{route: "/a", status: 200, duration: usualDuration(i)})
	}
	// Well below the threshold of a second, but far slower than the route.
	outlier := record(tracer, testRequest{route: "/a", status: 200, duration: 80 * time.Millisecond})

	got, ok := store.Get(outlier)
	require.True(t, ok)
	assert.Equal(t, KeptSlow, got.Kept)
	for i := range 100 {
		usual := record(tracer, testRequest{route: "/a", status: 200, duration: usualDuration(i)})
		if got, ok := store.Get(usual); ok {
			assert.NotEqual(t, KeptSlow, got.Kept, "a request of %v is not slow for its route", usualDuration(i))
		}
	}
}

func TestStore_KeepsRootPastSpanLimit(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	start := testNow.Add(-time.Second)
	ctx, root := tracer.Start(context.Background(), "GET /{owner}/{repo}",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(start),
		trace.WithAttributes(attribute.Int("http.response.status_code", 500)),
	)
	for i := range maxSpansPerTrace + 1 {
		at := start.Add(time.Duration(i) * time.Millisecond)
		_, git := tracer.Start(ctx, "git log", trace.WithTimestamp(at))
		git.End(trace.WithTimestamp(at.Add(time.Millisecond)))
	}
	root.End(trace.WithTimestamp(testNow))

	got, ok := store.Get(root.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Equal(t, "GET /{owner}/{repo}", got.Spans[0].Name)
	assert.Equal(t, float64(1000), got.DurationMs)
	assert.Equal(t, 1, got.DroppedSpans)
}

func TestStore_AttributeValues(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	_, root := tracer.Start(context.Background(), "GET /{owner}/{repo}",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(testNow.Add(-time.Second)),
		trace.WithAttributes(
			attribute.Int("http.response.status_code", 500),
			attribute.StringSlice("git.args", []string{"log", "--", "<path>"}),
		),
	)
	root.End(trace.WithTimestamp(testNow))

	got, ok := store.Get(root.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Contains(t, got.Spans[0].Attributes, Attribute{Key: "git.args", Value: `["log","--","<path>"]`})
}

func TestStore_LongSpanName(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	_, root := tracer.Start(context.Background(), strings.Repeat("n", 4096), trace.WithTimestamp(testNow.Add(-2*time.Second)))
	root.SetStatus(codes.Error, "")
	root.End(trace.WithTimestamp(testNow))

	got, ok := store.Get(root.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("n", maxNameBytes)+"…", got.Spans[0].Name)
	assert.Equal(t, got.Spans[0].Name, got.Route.Name)
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		maxBytes int
		want     string
	}{
		{name: "shorter", s: "abc", maxBytes: 4, want: "abc"},
		{name: "as long", s: "abcd", maxBytes: 4, want: "abcd"},
		{name: "longer", s: "abcde", maxBytes: 4, want: "abcd…"},
		{name: "cut inside a character", s: "abc日本", maxBytes: 4, want: "abc…"},
		{name: "cut between characters", s: "abc日本", maxBytes: 6, want: "abc日…"},
		{name: "only a part of a character fits", s: "日本", maxBytes: 2, want: "…"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, truncate(test.s, test.maxBytes))
		})
	}
}

// A client can send requests with the trace ID of one that is still running,
// each with a path that matches no route and so names its span. They are
// requests of their own, counted under their method.
func TestStore_JoinedRequests(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0xce, 1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	start := testNow.Add(-2 * time.Second)
	request := func(name string) (context.Context, trace.Span) {
		return tracer.Start(ctx, name, trace.WithSpanKind(trace.SpanKindServer), trace.WithTimestamp(start))
	}
	openCtx, open := request("GET /a")
	_, query := tracer.Start(openCtx, "db SELECT user", trace.WithTimestamp(start))
	query.End(trace.WithTimestamp(start.Add(time.Millisecond)))
	for _, path := range []string{"/made-up-1", "/made-up-2"} {
		_, joined := request("PROPFIND " + path)
		joined.End(trace.WithTimestamp(start.Add(time.Millisecond)))
	}
	_, lateQuery := tracer.Start(openCtx, "db UPDATE user", trace.WithTimestamp(start.Add(time.Second)))
	lateQuery.End(trace.WithTimestamp(start.Add(time.Second + time.Millisecond)))
	open.End(trace.WithTimestamp(testNow))

	overview := store.Overview(15, "15m")
	var operations []string
	for _, op := range overview.Operations {
		operations = append(operations, op.Name)
	}
	assert.ElementsMatch(t, []string{"db SELECT user", "db UPDATE user"}, operations)

	requests := make(map[string]int)
	for _, route := range overview.Routes {
		requests[route.Name] = route.Requests
	}
	assert.Equal(t, map[string]int{"GET": 1, "PROPFIND": 2}, requests)
	assert.Equal(t, 3, overview.Requests)
}

// joinedTo returns a context whose requests all belong to one trace, as those
// of a client that sends its own trace ID do.
func joinedTo(id byte) context.Context {
	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{id, 1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithRemoteSpanContext(context.Background(), parent)
}

// A request that joined the trace of another one is only found in that trace,
// so one that failed or was slow keeps the trace, also when the request that
// ended last is neither.
func TestStore_JoinedRequestKeepsTrace(t *testing.T) {
	tests := []struct {
		name string
		// The joined request.
		status     int
		spanFailed bool
		duration   time.Duration
		// The request that ends last.
		lastStatus int
		// sampleTaken is whether the route of that request has had its sample
		// of the minute, so that the trace would be dropped.
		sampleTaken   bool
		slowThreshold time.Duration
		want          KeptReason
		wantErrors    int
	}{
		{name: "failed", status: 500, duration: time.Millisecond, lastStatus: 200, sampleTaken: true, slowThreshold: time.Second, want: KeptFailed, wantErrors: 1},
		{name: "failed by the status of its span", status: 200, spanFailed: true, duration: time.Millisecond, lastStatus: 200, sampleTaken: true, slowThreshold: time.Second, want: KeptFailed, wantErrors: 1},
		{name: "failed in a sampled trace", status: 500, duration: time.Millisecond, lastStatus: 200, slowThreshold: time.Second, want: KeptFailed, wantErrors: 1},
		{name: "slow", status: 200, duration: 2 * time.Second, lastStatus: 200, sampleTaken: true, slowThreshold: time.Second, want: KeptSlow},
		{name: "slow at the threshold", status: 200, duration: time.Second, lastStatus: 200, sampleTaken: true, slowThreshold: time.Second, want: KeptSlow},
		{name: "slow in a sampled trace", status: 200, duration: 2 * time.Second, lastStatus: 200, slowThreshold: time.Second, want: KeptSlow},
		{name: "slow in a failed trace", status: 200, duration: 2 * time.Second, lastStatus: 500, sampleTaken: true, slowThreshold: time.Second, want: KeptFailed, wantErrors: 1},
		{name: "slow without a threshold", status: 200, duration: 2 * time.Second, lastStatus: 200, sampleTaken: true},
		{name: "neither", status: 200, duration: time.Millisecond, lastStatus: 200, sampleTaken: true, slowThreshold: time.Second},
		{name: "neither in a sampled trace", status: 200, duration: time.Millisecond, lastStatus: 200, slowThreshold: time.Second, want: KeptSampled},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := defaultOptions()
			opts.SlowThreshold = test.slowThreshold
			store, tracer := newTestStore(t, opts)
			wantRequests := 2
			if test.sampleTaken {
				record(tracer, testRequest{route: "/a", status: 200, duration: time.Millisecond})
				wantRequests++
			}

			ctx := joinedTo(0xc1)
			end := testNow.Add(-500 * time.Millisecond)
			request := func(route string, status int, duration time.Duration) trace.Span {
				_, sp := tracer.Start(ctx, "GET "+route,
					trace.WithSpanKind(trace.SpanKindServer),
					trace.WithTimestamp(end.Add(-duration)),
					trace.WithAttributes(
						attribute.String("http.route", route),
						attribute.Int("http.response.status_code", status),
					),
				)
				return sp
			}
			last := request("/a", test.lastStatus, 10*time.Millisecond)
			joined := request("/b", test.status, test.duration+time.Millisecond)
			if test.spanFailed {
				joined.SetStatus(codes.Error, "")
			}
			joined.End(trace.WithTimestamp(end.Add(-time.Millisecond)))
			last.End(trace.WithTimestamp(end))

			id := last.SpanContext().TraceID().String()
			got, ok := store.Get(id)
			require.Equal(t, test.want != "", ok)
			if ok {
				assert.Equal(t, test.want, got.Kept)
				assert.Equal(t, test.lastStatus, got.Status, "the trace shows the request that ended last")
			}

			// The search for failed requests finds the trace by the one that joined it.
			failed := store.Search(SearchQuery{Status: "5xx"})
			if test.want == KeptFailed {
				require.Len(t, failed.Traces, 1)
				assert.Equal(t, id, failed.Traces[0].ID)
				assert.True(t, failed.Traces[0].Error)
				require.Len(t, failed.Points, 1)
				assert.True(t, failed.Points[0].Error)
			} else {
				assert.Empty(t, failed.Traces)
			}

			interval := store.TakeInterval()
			assert.Equal(t, wantRequests, interval.Requests)
			assert.Equal(t, test.wantErrors, interval.Errors)
			overview := store.Overview(15, "15m")
			assert.Equal(t, test.wantErrors, overview.Errors)
		})
	}
}

// A request can carry the trace ID of background work, e.g., one that a Git
// hook sends during a push. It counts as a request like any other, while the
// interval leaves it out along with the time of the work it joined.
func TestStore_RequestJoinedBackgroundWork(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	ctx := joinedTo(0xc2)
	start := testNow.Add(-time.Second)
	_, background := tracer.Start(ctx, "ssh git-receive-pack", trace.WithTimestamp(start))
	requestCtx, request := tracer.Start(ctx, "GET /a",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(start),
		trace.WithAttributes(attribute.String("http.route", "/a")),
	)
	_, query := tracer.Start(requestCtx, "db SELECT user", trace.WithTimestamp(start))
	query.End(trace.WithTimestamp(start.Add(100 * time.Millisecond)))
	request.End(trace.WithTimestamp(start.Add(300 * time.Millisecond)))
	background.End(trace.WithTimestamp(start.Add(500 * time.Millisecond)))

	overview := store.Overview(15, "15m")
	assert.Equal(t, 1, overview.Requests)
	assert.InDelta(t, 300, overview.TotalMs, 0.001)
	// 500 ms of the background work, 200 ms of the request outside its query,
	// and the 100 ms of the query.
	assert.InDelta(t, 800, overview.SpanSelfMs, 0.001)
	requests := make(map[string]int)
	for _, route := range overview.Routes {
		requests[route.Name] = route.Requests
	}
	assert.Equal(t, map[string]int{"GET /a": 1, "ssh git-receive-pack": 1}, requests)
	assert.Zero(t, store.TakeInterval().Requests)
}

// Once the store holds as many routes as it keeps, a new one counts with the
// others of its kind, and its traces are listed there too.
func TestStore_RouteLimit(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())
	for i := range maxRoutes {
		record(tracer, testRequest{route: "/" + strconv.Itoa(i), status: 200, duration: time.Millisecond})
	}

	request := record(tracer, testRequest{route: "/new", status: 500, duration: time.Millisecond})
	got, ok := store.Get(request)
	require.True(t, ok)
	assert.Equal(t, Route{Name: "GET", Kind: RouteKindOther, Method: "GET"}, got.Route)

	_, background := tracer.Start(context.Background(), "mirror sync", trace.WithTimestamp(testNow.Add(-2*time.Second)))
	background.End(trace.WithTimestamp(testNow))
	got, ok = store.Get(background.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Equal(t, Route{Name: "other", Kind: RouteKindBackground}, got.Route)

	assert.Equal(t, 1, store.Search(SearchQuery{Route: "GET"}).Total)
}

// A client can send many requests with the trace ID of one that is still
// running, and they must not grow that trace past the limit and push out the
// other kept traces when it ends.
func TestStore_ReusedTraceIDWhileOpen(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())
	failed := record(tracer, testRequest{route: "/a", status: 500, duration: time.Millisecond})

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0xcd, 1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	start := testNow.Add(-2 * time.Second)
	request := func() trace.Span {
		_, root := tracer.Start(ctx, "POST /b",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithTimestamp(start),
			trace.WithAttributes(
				attribute.String("http.route", "/b"),
				attribute.Int("http.response.status_code", 200),
				attribute.String("url.path", strings.Repeat("x", 100)),
			),
		)
		return root
	}
	open := request()
	for range 3 * maxSpansPerTrace {
		request().End(trace.WithTimestamp(start.Add(time.Millisecond)))
	}
	assert.LessOrEqual(t, len(store.pending[parent.TraceID()].spans), maxSpansPerTrace)
	open.End(trace.WithTimestamp(testNow))

	_, ok := store.Get(failed)
	assert.True(t, ok, "the merged trace pushed out a failed one")
	got, ok := store.Get(parent.TraceID().String())
	require.True(t, ok)
	assert.LessOrEqual(t, len(got.Spans), maxSpansPerTrace+1)
}

func TestStore_KeepsRequestLongerThanPendingMaxAge(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	start := testNow
	_, clone := tracer.Start(context.Background(), "POST /{owner}/{repo}/git-upload-pack",
		trace.WithSpanKind(trace.SpanKindServer),
		trace.WithTimestamp(start),
		trace.WithAttributes(attribute.Int("http.response.status_code", 200)),
	)
	later := start.Add(pendingMaxAge + time.Minute)
	store.now = func() time.Time { return later }
	// Starting another trace drops the expired pending ones.
	record(tracer, testRequest{route: "/a", status: 200, start: later.Add(-time.Millisecond), duration: time.Millisecond})
	clone.End(trace.WithTimestamp(later))

	got, ok := store.Get(clone.SpanContext().TraceID().String())
	require.True(t, ok)
	assert.Equal(t, KeptSlow, got.Kept)
}

// The rollups cover the last hour, so a trace from before it is not compared
// with the requests of that hour.
func TestStore_GetOldTraceHasNoStats(t *testing.T) {
	opts := defaultOptions()
	opts.MaxAge = 24 * time.Hour
	store, tracer := newTestStore(t, opts)

	old := record(tracer, testRequest{route: "/a", status: 500, start: testNow.Add(-3 * time.Hour), duration: 10 * time.Millisecond, git: time.Millisecond})
	recent := record(tracer, testRequest{route: "/a", status: 500, duration: 10 * time.Millisecond, git: time.Millisecond})

	got, ok := store.Get(old)
	require.True(t, ok)
	assert.Nil(t, got.RouteStats)
	assert.Empty(t, got.Operations)

	got, ok = store.Get(recent)
	require.True(t, ok)
	assert.NotNil(t, got.RouteStats)
	assert.NotEmpty(t, got.Operations)
}

func TestStore_Routes(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	route := record(tracer, testRequest{route: "/api/web/{owner}/{repo}/header", status: 200, duration: time.Millisecond})
	other := record(tracer, testRequest{status: 200, duration: time.Millisecond})

	ctx, background := tracer.Start(context.Background(), "mirror sync", trace.WithTimestamp(testNow.Add(-time.Second)))
	_, git := tracer.Start(ctx, "git remote update", trace.WithTimestamp(testNow.Add(-time.Second)))
	git.End(trace.WithTimestamp(testNow))
	_, query := tracer.Start(ctx, "db SELECT mirror", trace.WithTimestamp(testNow.Add(-time.Second)))
	query.End(trace.WithTimestamp(testNow))
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
	require.Len(t, got.Spans, 3)
	assert.Equal(t, SpanKindGit, got.Spans[1].Kind)
	assert.Equal(t, SpanKindDB, got.Spans[2].Kind)

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

// The chart of a search covers the time of every match, even when there are
// more of them than it shows.
func TestStore_SearchPointsSpreadOverTime(t *testing.T) {
	opts := defaultOptions()
	opts.MaxMemory = 64 << 20
	opts.MaxAge = 24 * time.Hour
	store, tracer := newTestStore(t, opts)

	n := 2 * maxSearchPoints
	oldest := testNow.Add(-time.Duration(n) * time.Second)
	for i := range n {
		record(tracer, testRequest{route: "/a", status: 500, start: oldest.Add(time.Duration(i) * time.Second), duration: time.Duration(i%7+1) * time.Millisecond})
	}

	for _, slowest := range []bool{false, true} {
		got := store.Search(SearchQuery{Slowest: slowest})
		require.Equal(t, n, got.Total)
		require.Len(t, got.Points, maxSearchPoints)
		assert.True(t, got.Points[0].Start.Equal(oldest), "slowest=%v: the first point starts at %v", slowest, got.Points[0].Start)
		assert.True(t, got.Points[len(got.Points)-1].Start.After(testNow.Add(-time.Minute)), "slowest=%v", slowest)
	}
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

// A client may send the same trace ID with two requests, and the trace page
// redirects to the only search result, so search lists only the trace that
// Get returns.
func TestStore_SearchReusedTraceID(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())

	parent := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    trace.TraceID{0xab, 1},
		SpanID:     trace.SpanID{1},
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	ctx := trace.ContextWithRemoteSpanContext(context.Background(), parent)
	for _, status := range []int{500, 200} {
		_, root := tracer.Start(ctx, "GET /a",
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithTimestamp(testNow.Add(-time.Second)),
			trace.WithAttributes(attribute.Int("http.response.status_code", status)),
		)
		root.End(trace.WithTimestamp(testNow))
	}

	id := parent.TraceID().String()
	got := store.Search(SearchQuery{Q: id})
	require.Equal(t, 1, got.Total)
	latest, ok := store.Get(id)
	require.True(t, ok)
	assert.Equal(t, 200, latest.Status)
	assert.Equal(t, 200, got.Traces[0].Status)
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
	kept, sampled := store.MemoryUse()
	assert.Equal(t, MemoryUse{Used: store.kept.size, Limit: store.kept.budget}, kept)
	assert.Equal(t, MemoryUse{Limit: store.sampled.budget}, sampled, "failed traces take none of the memory of sampled ones")
	assert.Equal(t, opts.MaxMemory, kept.Limit+sampled.Limit)
	_, ok := store.Get(first)
	assert.False(t, ok, "the oldest traces are evicted")
	_, ok = store.Get(last)
	assert.True(t, ok, "the most recent traces are kept")
	assert.Len(t, store.byID, len(store.kept.traces)+len(store.sampled.traces))
}

func TestStore_TakeInterval(t *testing.T) {
	store, tracer := newTestStore(t, defaultOptions())
	store.intervalStart = testNow.Add(-15 * time.Second)

	record(tracer, testRequest{route: "/a", status: 200, duration: 40 * time.Millisecond, git: 30 * time.Millisecond})
	record(tracer, testRequest{route: "/a", status: 500, duration: 20 * time.Millisecond})
	record(tracer, testRequest{route: "/b", status: 404, duration: 10 * time.Millisecond})

	got := store.TakeInterval()
	assert.Equal(t, 15*time.Second, got.Elapsed)
	assert.Equal(t, 3, got.Requests)
	assert.Equal(t, 1, got.Errors, "only the response with status 500 failed")
	assert.Equal(t, 30*time.Millisecond, got.Git)
	assert.Zero(t, got.DB)
	// The request with the Git command spent 10 ms outside of it.
	assert.Equal(t, 40*time.Millisecond, got.Other)

	got = store.TakeInterval()
	assert.Zero(t, got.Requests, "the interval starts over")
	assert.Zero(t, got.Git)
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
