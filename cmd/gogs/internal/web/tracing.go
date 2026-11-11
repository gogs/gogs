package web

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// initTracing exports spans, e.g., of Git operations, over OTLP when an
// endpoint is set through the standard OpenTelemetry environment variables,
// i.e., OTEL_EXPORTER_OTLP_ENDPOINT or OTEL_EXPORTER_OTLP_TRACES_ENDPOINT.
func initTracing(ctx context.Context) error {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return errors.Wrap(err, "create exporter")
	}
	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the service name.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "gogs")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return errors.Wrap(err, "create resource")
	}
	otel.SetTracerProvider(sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	))
	return nil
}
