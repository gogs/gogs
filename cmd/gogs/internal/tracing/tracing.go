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

// recordUnsampled records the spans that its sampler drops, e.g., of a request
// whose caller does not sample its trace, so that processors see every request
// while only sampled spans are exported.
type recordUnsampled struct {
	sdktrace.Sampler
}

func (s recordUnsampled) ShouldSample(p sdktrace.SamplingParameters) sdktrace.SamplingResult {
	result := s.Sampler.ShouldSample(p)
	if result.Decision == sdktrace.Drop {
		result.Decision = sdktrace.RecordOnly
	}
	return result
}

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
	// A sampler set through OTEL_TRACES_SAMPLER decides for the processors too,
	// since the SDK has no way to wrap it.
	if len(processors) > 0 && os.Getenv("OTEL_TRACES_SAMPLER") == "" {
		opts = append(opts, sdktrace.WithSampler(recordUnsampled{sdktrace.ParentBased(sdktrace.AlwaysSample())}))
	}
	provider := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(provider)
	// The web server reads the trace that a request carries to link the trace of
	// the request to it.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return func(timeout time.Duration) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := provider.Shutdown(ctx); err != nil {
			logger.Warn("Failed to export the remaining spans", "error", err)
		}
	}, nil
}
