package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"gogs.io/gogs/internal/conf"
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

func TestOverrideHTTPPort(t *testing.T) {
	tests := []struct {
		name        string
		externalURL string
		httpPort    int
		want        string
	}{
		{name: "served directly", externalURL: "http://localhost:3000/", httpPort: 3000, want: "http://localhost:3001/"},
		{name: "served directly under a subpath", externalURL: "http://[::1]:3000/gogs/", httpPort: 3000, want: "http://[::1]:3001/gogs/"},
		{name: "served directly on the default port", externalURL: "http://localhost/", httpPort: 80, want: "http://localhost:3001/"},
		{name: "behind a proxy on another port", externalURL: "https://git.example.com:1443/", httpPort: 3000, want: "https://git.example.com:1443/"},
		{name: "behind a proxy on the default port", externalURL: "https://git.example.com/", httpPort: 3000, want: "https://git.example.com/"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			u, err := url.Parse(test.externalURL)
			require.NoError(t, err)
			conf.SetMockServer(t, conf.ServerOpts{ExternalURL: test.externalURL, URL: u, HTTPPort: test.httpPort})

			overrideHTTPPort(3001)
			assert.Equal(t, test.want, conf.Server.ExternalURL)
			assert.Equal(t, test.want, conf.Server.URL.String())
			assert.Equal(t, 3001, conf.Server.HTTPPort)
		})
	}
}
