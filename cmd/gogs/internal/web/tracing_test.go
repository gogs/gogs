package web

import (
	"net/http/httptest"
	"testing"

	"github.com/flamego/flamego"
	"github.com/go-macaron/i18n"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gopkg.in/macaron.v1"

	"gogs.io/gogs/internal/context"
)

func TestTracingHandler_spanName(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	t.Cleanup(func() { otel.SetTracerProvider(old) })

	f := flamego.New()
	f.Get("/api/web/{owner}/{repo}/header", func() {})

	m := macaron.New()
	m.Use(func(c *macaron.Context) {
		c.Map(&context.Context{Context: c})
		c.Map(i18n.Locale{})
	})
	m.Any("/api/web/*", flamegoBridger(f))
	handler := newTracingHandler(m)

	tests := []struct {
		path string
		want string
	}{
		{path: "/api/web/alice/alice/header", want: "GET /api/web/{owner}/{repo}/header"},
		{path: "/api/web/no/such/route", want: "GET /api/web/no/such/route"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", test.path, nil))
			spans := recorder.Ended()
			require.NotEmpty(t, spans)
			assert.Equal(t, test.want, spans[len(spans)-1].Name())
		})
	}
}
