package main

import (
	"context"
	"net/http"
	"os"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	otelprom "go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"unknwon.dev/x/logx"
)

// setUpTelemetry installs global OpenTelemetry providers. Metrics are always
// served in the Prometheus format by the returned handler, and spans and
// metrics are also exported over OTLP when an endpoint is set through the
// standard environment variables, i.e., OTEL_EXPORTER_OTLP_ENDPOINT, or
// OTEL_EXPORTER_OTLP_TRACES_ENDPOINT and OTEL_EXPORTER_OTLP_METRICS_ENDPOINT.
// The returned function flushes what is left, giving up after the timeout.
func setUpTelemetry(ctx context.Context, logger *logx.Logger) (metrics http.Handler, shutdown func(timeout time.Duration), err error) {
	// OTEL_SERVICE_NAME and OTEL_RESOURCE_ATTRIBUTES override the service name.
	res, err := resource.New(ctx,
		resource.WithAttributes(attribute.String("service.name", "gebra")),
		resource.WithFromEnv(),
	)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create resource")
	}

	traceOpts := []sdktrace.TracerProviderOption{sdktrace.WithResource(res)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") != "" {
		exporter, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "create trace exporter")
		}
		traceOpts = append(traceOpts, sdktrace.WithBatcher(exporter))
	}
	tracerProvider := sdktrace.NewTracerProvider(traceOpts...)

	registry := prometheus.NewRegistry()
	promExporter, err := otelprom.New(otelprom.WithRegisterer(registry))
	if err != nil {
		return nil, nil, errors.Wrap(err, "create Prometheus exporter")
	}
	meterOpts := []sdkmetric.Option{sdkmetric.WithResource(res), sdkmetric.WithReader(promExporter)}
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") != "" || os.Getenv("OTEL_EXPORTER_OTLP_METRICS_ENDPOINT") != "" {
		exporter, err := otlpmetrichttp.New(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "create metric exporter")
		}
		meterOpts = append(meterOpts, sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter)))
	}
	meterProvider := sdkmetric.NewMeterProvider(meterOpts...)

	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	// Callers that pass a traceparent header get Gebra's spans in their trace.
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{}), func(timeout time.Duration) {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := errors.Join(tracerProvider.Shutdown(ctx), meterProvider.Shutdown(ctx)); err != nil {
			logger.Warn("Failed to flush telemetry", "error", err)
		}
	}, nil
}
