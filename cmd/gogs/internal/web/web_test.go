package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestTracingHandler(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	before := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(before) })

	// The admin panel groups requests by their route, which is only known once
	// the request was routed.
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/web/{owner}/{repo}/header", func(http.ResponseWriter, *http.Request) {})
	handler := newTracingHandler(mux)

	for _, path := range []string{"/api/web/unknwon/gogs/header", "/unknwon/gogs"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}

	spans := recorder.Ended()
	require.Len(t, spans, 2)
	route := func(span sdktrace.ReadOnlySpan) string {
		for _, kv := range span.Attributes() {
			if kv.Key == "http.route" {
				return kv.Value.AsString()
			}
		}
		return ""
	}
	assert.Equal(t, "GET /api/web/{owner}/{repo}/header", spans[0].Name())
	assert.Equal(t, "/api/web/{owner}/{repo}/header", route(spans[0]))
	assert.Equal(t, "GET /unknwon/gogs", spans[1].Name())
	assert.Equal(t, "", route(spans[1]))
}

func TestTracingHandler_TraceOfRequest(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	beforeProvider, beforePropagator := otel.GetTracerProvider(), otel.GetTextMapPropagator()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(beforeProvider)
		otel.SetTextMapPropagator(beforePropagator)
	})
	handler := newTracingHandler(http.NewServeMux())

	const (
		carriedTraceID = "0af7651916cd43dd8448eb211c80319c"
		carriedSpanID  = "b7ad6b7169203331"
	)
	for _, traceparent := range []string{"00-" + carriedTraceID + "-" + carriedSpanID + "-01", "00-" + carriedTraceID + "-" + carriedSpanID + "-01", ""} {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		if traceparent != "" {
			r.Header.Set("Traceparent", traceparent)
		}
		handler.ServeHTTP(httptest.NewRecorder(), r)
	}

	spans := recorder.Ended()
	require.Len(t, spans, 3)
	traceIDs := make(map[string]bool)
	for _, span := range spans {
		assert.False(t, span.Parent().IsValid(), "the span has a parent")
		traceIDs[span.SpanContext().TraceID().String()] = true
	}
	assert.Len(t, traceIDs, 3, "requests share a trace")
	assert.NotContains(t, traceIDs, carriedTraceID)

	for _, span := range spans[:2] {
		require.Len(t, span.Links(), 1)
		assert.Equal(t, carriedTraceID, span.Links()[0].SpanContext.TraceID().String())
		assert.Equal(t, carriedSpanID, span.Links()[0].SpanContext.SpanID().String())
	}
	assert.Empty(t, spans[2].Links())
}
