package web

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/cockroachdb/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	log "unknwon.dev/clog/v2"
)

// initTracing exports spans, e.g., of requests and Git operations, over OTLP
// when an endpoint is set through the standard OpenTelemetry environment
// variables, i.e., OTEL_EXPORTER_OTLP_ENDPOINT or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT. It returns a function that exports the
// spans that are left, or nil when no endpoint is set.
//
// Spans wait in a batch for up to 5 seconds before they are exported, so the
// spans that are left are also exported when Gogs is interrupted or terminated,
// before it exits.
func initTracing(ctx context.Context) (shutdown func(), err error) {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		return nil, nil
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "create exporter")
	}
	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the service name.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "gogs")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create resource")
	}
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)

	shutdown = func() {
		// Stopping Gogs must not wait long on a collector that is down.
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			log.Warn("Failed to export the remaining spans: %v", err)
		}
	}
	go func() {
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		<-signals
		shutdown()
		os.Exit(0)
	}()
	return shutdown, nil
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
