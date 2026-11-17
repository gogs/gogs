package telemetry

import (
	"context"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// SetUser records the signed-in user of a request on the span of the request.
func SetUser(ctx context.Context, id int64, name string) {
	trace.SpanFromContext(ctx).SetAttributes(
		attribute.Int64("enduser.id", id),
		attribute.String("enduser.name", name),
	)
}

// SetRepository records the ID of the repository a request or a Git operation
// is about on the span of the request.
func SetRepository(ctx context.Context, id int64) {
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int64("repo.id", id))
}

// RecordFailure records the error that failed a request, e.g., with a 500
// response, on the span of the request, which the response alone does not
// explain.
func RecordFailure(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}
