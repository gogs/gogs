package database

import (
	"strings"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

var tracer = otel.Tracer("gogs.io/gogs/internal/database")

const tracingSpanKey = "gogs:tracing:span"

// tracingPlugin records a span for every query that runs under another span,
// e.g., of a request, named after its statement and table, e.g., "db SELECT
// user". The span carries the SQL with placeholders, never the values.
type tracingPlugin struct{}

func (tracingPlugin) Name() string { return "gogs:tracing" }

func (tracingPlugin) Initialize(db *gorm.DB) error {
	cb := db.Callback()
	return errors.Join(
		cb.Create().Before("gorm:create").Register("gogs:tracing:before_create", startQuerySpan),
		cb.Create().After("gorm:create").Register("gogs:tracing:after_create", endQuerySpan),
		cb.Query().Before("gorm:query").Register("gogs:tracing:before_query", startQuerySpan),
		cb.Query().After("gorm:query").Register("gogs:tracing:after_query", endQuerySpan),
		cb.Update().Before("gorm:update").Register("gogs:tracing:before_update", startQuerySpan),
		cb.Update().After("gorm:update").Register("gogs:tracing:after_update", endQuerySpan),
		cb.Delete().Before("gorm:delete").Register("gogs:tracing:before_delete", startQuerySpan),
		cb.Delete().After("gorm:delete").Register("gogs:tracing:after_delete", endQuerySpan),
		cb.Row().Before("gorm:row").Register("gogs:tracing:before_row", startQuerySpan),
		cb.Row().After("gorm:row").Register("gogs:tracing:after_row", endQuerySpan),
		cb.Raw().Before("gorm:raw").Register("gogs:tracing:before_raw", startQuerySpan),
		cb.Raw().After("gorm:raw").Register("gogs:tracing:after_raw", endQuerySpan),
	)
}

func startQuerySpan(db *gorm.DB) {
	ctx := db.Statement.Context
	// Queries outside of a request or another span, e.g., of startup, would each
	// become a trace of their own.
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return
	}
	// The statement is only built by the query, so the span is named once it ends.
	_, span := tracer.Start(ctx, "db", trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", db.Name())))
	db.InstanceSet(tracingSpanKey, span)
}

func endQuerySpan(db *gorm.DB) {
	v, ok := db.InstanceGet(tracingSpanKey)
	if !ok {
		return
	}
	span := v.(trace.Span)
	defer span.End()

	query := db.Statement.SQL.String()
	verb, _, _ := strings.Cut(strings.TrimSpace(query), " ")
	if verb == "" {
		// The query failed before its statement was built.
		verb = "QUERY"
	}
	name := "db " + strings.ToUpper(verb)
	if db.Statement.Table != "" {
		name += " " + db.Statement.Table
	}
	span.SetName(name)
	span.SetAttributes(attribute.String("db.query.text", query))
	if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
		span.RecordError(db.Error)
		span.SetStatus(codes.Error, db.Error.Error())
	}
}
