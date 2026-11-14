package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
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
