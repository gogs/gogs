// Package tracing sets up exporting spans for Gogs processes.
package tracing

import (
	"context"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"unknwon.dev/x/logx"
)

// Init records spans, e.g., of requests and Git operations. It exports them
// over OTLP when an endpoint is set through the standard OpenTelemetry
// environment variables, i.e., OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, and passes them to the given processors,
// e.g., the store of the admin panel. It returns a function that ends the
// processors and exports the spans that are left, which wait in a batch for up
// to 5 seconds, giving up after the timeout, or nil when there is neither an
// endpoint nor a processor.
func Init(ctx context.Context, logger *logx.Logger, processors ...sdktrace.SpanProcessor) (shutdown func(timeout time.Duration), err error) {
	export := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != ""
	if !export && len(processors) == 0 {
		return nil, nil
	}

	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the service name.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "gogs")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create resource")
	}
	opts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if export {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "create exporter")
		}
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}
	for _, p := range processors {
		opts = append(opts, sdktrace.WithSpanProcessor(p))
	}
	provider := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)
	// Requests that Git hooks make to Gogs carry the trace they belong to, and the
	// web server continues the trace of any request that carries one.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return func(timeout time.Duration) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			logger.Warn("Failed to export the remaining spans", "error", err)
		}
	}, nil
}
