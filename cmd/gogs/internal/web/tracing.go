package web

import (
	"context"
	"net/http"
	"os"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// initTracing exports spans, e.g., of requests and Git operations, over OTLP
// when an endpoint is set through the standard OpenTelemetry environment
// variables, i.e., OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT. It returns false when no endpoint is set.
func initTracing(ctx context.Context) (bool, error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return false, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return false, errors.Wrap(err, "create exporter")
	}
	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the service name.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "gogs")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return false, errors.Wrap(err, "create resource")
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	))
	return true, nil
}

// newTracingHandler returns a handler that records a span for every request, so
// that the spans of what the request runs, e.g., Git operations, are grouped
// under it.
func newTracingHandler(h http.Handler) http.Handler {
	return otelhttp.NewHandler(h, "", otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
		if r.Pattern != "" {
			return r.Pattern
		}
		return r.Method + " " + r.URL.Path
	}))
}
