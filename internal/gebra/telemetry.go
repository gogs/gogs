package gebra

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const instrumentationName = "gogs.io/gogs/internal/gebra"

// durationBuckets suit operations that take from a fraction of a millisecond
// to seconds, which OpenTelemetry's default buckets do not resolve.
var durationBuckets = []float64{0.0005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10, 30}

// telemetry holds the engine's tracer and metric instruments. Metrics carry no
// store, object, or user IDs, which would make every series unique; spans
// carry the store ID, and types and relations rather than object or user IDs,
// which can identify people.
type telemetry struct {
	tracer trace.Tracer

	queryDuration    metric.Float64Histogram
	viewFallbacks    metric.Int64Counter
	writeDuration    metric.Float64Histogram
	writeTuples      metric.Int64Counter
	writeRetries     metric.Int64Counter
	viewRows         metric.Int64Counter
	viewReachWrites  metric.Int64Histogram
	applyDuration    metric.Float64Histogram
	applyChanges     metric.Int64Counter
	applyBatchWrites metric.Int64Histogram
	applyLag         metric.Float64Histogram
	leadership       metric.Int64Counter
	applyFailures    metric.Int64Counter
	buildDuration    metric.Float64Histogram

	gauges metric.Registration
}

func newTelemetry(e *Engine, tp trace.TracerProvider, mp metric.MeterProvider) (*telemetry, error) {
	if tp == nil {
		tp = otel.GetTracerProvider()
	}
	if mp == nil {
		mp = otel.GetMeterProvider()
	}
	meter := mp.Meter(instrumentationName)
	t := &telemetry{tracer: tp.Tracer(instrumentationName)}
	seconds := func(name, desc string) metric.Float64Histogram {
		h, err := meter.Float64Histogram(name, metric.WithUnit("s"), metric.WithDescription(desc),
			metric.WithExplicitBucketBoundaries(durationBuckets...))
		if err != nil {
			otel.Handle(err)
		}
		return h
	}
	counter := func(name, unit, desc string) metric.Int64Counter {
		c, err := meter.Int64Counter(name, metric.WithUnit(unit), metric.WithDescription(desc))
		if err != nil {
			otel.Handle(err)
		}
		return c
	}
	t.queryDuration = seconds("gebra.query.duration", "Duration of queries, by operation and whether the view answered them.")
	t.viewFallbacks = counter("gebra.view.fallbacks", "{query}", "Queries that evaluated the graph because the view could not answer them, by reason, and queries that the view answered although it lacked writes they ask for, because none of those writes affects them (view_irrelevant_pending).")
	t.writeDuration = seconds("gebra.write.duration", "Duration of tuple writes, including retries, and waiting for the applier when the write asks for it.")
	t.writeTuples = counter("gebra.write.tuples", "{tuple}", "Tuples written and deleted.")
	t.writeRetries = counter("gebra.write.retries", "{retry}", "Write attempts that lost a race with another writer and ran again, by reason.")
	t.viewRows = counter("gebra.view.rows.changed", "{row}", "View rows inserted and deleted, by table.")
	var err error
	t.viewReachWrites, err = meter.Int64Histogram("gebra.view.reach.changed_per_change", metric.WithUnit("{row}"),
		metric.WithDescription("Reach rows inserted or deleted by one applier batch, which shows the fan-out of a change."),
		metric.WithExplicitBucketBoundaries(0, 1, 10, 100, 1000, 10_000, 100_000, 1_000_000))
	if err != nil {
		otel.Handle(err)
	}
	t.applyDuration = seconds("gebra.view.apply.duration", "Duration of applier batches that bring a view up to date.")
	t.applyChanges = counter("gebra.view.apply.changes", "{change}", "Tuple changes that appliers applied to views.")
	t.applyBatchWrites, err = meter.Int64Histogram("gebra.applier.batch.writes", metric.WithUnit("{write}"),
		metric.WithDescription("Writes that one applier batch applied."),
		metric.WithExplicitBucketBoundaries(1, 2, 4, 8, 16, 32, 64, 128, 256, 512, 1000))
	if err != nil {
		otel.Handle(err)
	}
	t.applyLag = seconds("gebra.applier.lag", "Time from a write to the commit of the applier batch that applied it.")
	t.leadership = counter("gebra.applier.leadership.changes", "{store}", "Stores whose applier this engine became (acquired), lost with its connection (lost), or gave up on close (released).")
	t.applyFailures = counter("gebra.applier.failures", "{batch}", "Applier batches that failed: race when another applier applied the same writes, error otherwise, after which the store's applier backs off.")
	t.buildDuration = seconds("gebra.view.build.duration", "Duration of view builds after model writes, from their start until their view serves queries.")

	rows, err := meter.Int64ObservableGauge("gebra.view.rows", metric.WithUnit("{row}"),
		metric.WithDescription("Rows in each store's view tables."))
	if err != nil {
		return nil, errors.Wrap(err, "create view rows gauge")
	}
	pending, err := meter.Int64ObservableGauge("gebra.view.pending_changes", metric.WithUnit("{change}"),
		metric.WithDescription("Tuple changes that each store's view does not include yet."))
	if err != nil {
		return nil, errors.Wrap(err, "create pending changes gauge")
	}
	inbox, err := meter.Int64ObservableGauge("gebra.inbox.pending_writes", metric.WithUnit("{write}"),
		metric.WithDescription("Committed writes, including model writes, that each store's applier has not applied yet."))
	if err != nil {
		return nil, errors.Wrap(err, "create inbox depth gauge")
	}
	lag, err := meter.Float64ObservableGauge("gebra.view.lag", metric.WithUnit("s"),
		metric.WithDescription("Age of the oldest write that each store's view does not include, or 0."))
	if err != nil {
		return nil, errors.Wrap(err, "create view lag gauge")
	}
	t.gauges, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		return e.observeViews(ctx, o, rows, pending, inbox, lag)
	}, rows, pending, inbox, lag)
	if err != nil {
		return nil, errors.Wrap(err, "register view gauges")
	}
	return t, nil
}

// observeViews reports the size and lag of every store's view. It reads one
// row per store, plus the oldest pending write of stores whose view lags.
func (e *Engine) observeViews(ctx context.Context, o metric.Observer, rows, pending, inbox metric.Int64Observable, lag metric.Float64Observable) error {
	db := e.db.handle()
	type viewRows struct {
		StoreID                         string
		MemberRows, EdgeRows, ReachRows int64
		Writes, Pending                 int64
		Oldest                          int64
	}
	states, err := queryStructs[viewRows](ctx, db, `SELECT v.store_id, v.member_rows, v.edge_rows, v.reach_rows,
			COUNT(i.id) AS writes, COALESCE(SUM(i.tuples), 0) AS pending, COALESCE(MIN(i.id), 0) AS oldest
		FROM gebra_view_state v
		JOIN gebra_store s ON s.id = v.store_id AND s.deleted_at IS NULL
		LEFT JOIN gebra_inbox i ON i.store_id = v.store_id
		GROUP BY v.store_id, v.member_rows, v.edge_rows, v.reach_rows`)
	if err != nil {
		return errors.Wrap(err, "read view states")
	}
	now := time.Now()
	for _, st := range states {
		store := attribute.String("gebra.store.id", st.StoreID)
		o.ObserveInt64(rows, st.MemberRows, metric.WithAttributes(store, attribute.String("gebra.view.table", "member")))
		o.ObserveInt64(rows, st.EdgeRows, metric.WithAttributes(store, attribute.String("gebra.view.table", "edge")))
		o.ObserveInt64(rows, st.ReachRows, metric.WithAttributes(store, attribute.String("gebra.view.table", "reach")))
		o.ObserveInt64(pending, st.Pending, metric.WithAttributes(store))
		o.ObserveInt64(inbox, st.Writes, metric.WithAttributes(store))
		age := 0.0
		if st.Oldest > 0 {
			// The applier may have applied the write since.
			oldest, err := queryStructs[inboxRow](ctx, db, "SELECT created_at FROM gebra_inbox WHERE id = ?", st.Oldest)
			if err != nil {
				return errors.Wrap(err, "read oldest pending write")
			}
			if len(oldest) > 0 {
				age = max(0, now.Sub(oldest[0].CreatedAt).Seconds())
			}
		}
		o.ObserveFloat64(lag, age, metric.WithAttributes(store))
	}
	return nil
}

// op is an engine operation in progress: a span, and the attributes its
// duration is recorded with.
type op struct {
	tel   *telemetry
	name  string
	span  trace.Span
	start time.Time
	attrs []attribute.KeyValue
	// path is "view" when the materialized view answered the operation and
	// "graph" when graph evaluation did. The checks of a batch may set it
	// concurrently, which mu guards.
	mu   sync.Mutex
	path string
}

func (o *op) setPath(path string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.path = path
}

type opKey struct{}

// startOp starts a span named after the operation.
func (e *Engine) startOp(ctx context.Context, name, storeID string) (context.Context, *op) {
	ctx, span := e.tel.tracer.Start(ctx, "gebra."+name, trace.WithAttributes(attribute.String("gebra.store.id", storeID)))
	o := &op{tel: e.tel, name: name, span: span, start: time.Now()}
	return context.WithValue(ctx, opKey{}, o), o
}

// target adds the object type and relation that a query is about, once the
// model has validated them, so that arbitrary input cannot add metric series.
func (o *op) target(objectType, relation string) {
	kv := []attribute.KeyValue{attribute.String("gebra.object_type", objectType), attribute.String("gebra.relation", relation)}
	o.attrs = append(o.attrs, kv...)
	o.span.SetAttributes(kv...)
}

func opFromContext(ctx context.Context) *op {
	o, _ := ctx.Value(opKey{}).(*op)
	return o
}

// end ends the span and records the duration of a query. The attributes go on
// the span only.
func (o *op) end(err error, spanAttrs ...attribute.KeyValue) {
	attrs := append([]attribute.KeyValue{attribute.String("gebra.operation", o.name)}, o.attrs...)
	if o.path != "" {
		attrs = append(attrs, attribute.String("gebra.path", o.path))
	}
	o.finish(err, &attrs, spanAttrs)
	o.tel.queryDuration.Record(context.Background(), time.Since(o.start).Seconds(), metric.WithAttributes(attrs...))
}

// finish ends the span with the error, if any, and adds error.type to the
// metric attributes.
func (o *op) finish(err error, metricAttrs *[]attribute.KeyValue, spanAttrs []attribute.KeyValue) {
	o.span.SetAttributes(spanAttrs...)
	if o.path != "" {
		o.span.SetAttributes(attribute.String("gebra.path", o.path))
	}
	if err != nil {
		kind := errorType(err)
		*metricAttrs = append(*metricAttrs, attribute.String("error.type", kind))
		o.span.SetAttributes(attribute.String("error.type", kind))
		o.span.RecordError(err)
		o.span.SetStatus(codes.Error, err.Error())
	}
	o.span.End()
}

func errorType(err error) string {
	switch {
	case errors.Is(err, ErrInvalidArgument):
		return "invalid_argument"
	case errors.Is(err, ErrNotFound):
		return "not_found"
	case errors.Is(err, ErrNotApplied):
		return "not_applied"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	default:
		return "internal"
	}
}

// recordViewChange records the view rows an applier batch changed.
func (t *telemetry) recordViewChange(ctx context.Context, v *viewMaintainer, source string) {
	if v == nil {
		return
	}
	src := attribute.String("gebra.view.source", source)
	for _, c := range []struct {
		table, operation string
		n                int64
	}{
		{"member", "insert", v.added.member}, {"member", "delete", v.removed.member},
		{"edge", "insert", v.added.edge}, {"edge", "delete", v.removed.edge},
		{"reach", "insert", v.added.reach}, {"reach", "delete", v.removed.reach},
	} {
		if c.n > 0 {
			t.viewRows.Add(ctx, c.n, metric.WithAttributes(src,
				attribute.String("gebra.view.table", c.table), attribute.String("gebra.view.operation", c.operation)))
		}
	}
	reachChanged := v.added.reach + v.removed.reach
	t.viewReachWrites.Record(ctx, reachChanged, metric.WithAttributes(src))
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int64("gebra.view.member_rows_inserted", v.added.member),
		attribute.Int64("gebra.view.member_rows_deleted", v.removed.member),
		attribute.Int64("gebra.view.edge_rows_inserted", v.added.edge),
		attribute.Int64("gebra.view.edge_rows_deleted", v.removed.edge),
		attribute.Int64("gebra.view.reach_rows_inserted", v.added.reach),
		attribute.Int64("gebra.view.reach_rows_deleted", v.removed.reach),
	)
}
