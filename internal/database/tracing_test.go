package database

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"gorm.io/gorm"
)

func TestTracingPlugin(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	before := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(before) })

	db := newTestDB(t, "TestTracingPlugin")
	require.NoError(t, db.Use(tracingPlugin{}))
	// Suites running at the same time record spans of their own, e.g., of Git,
	// through the same global provider, so only the spans that match count.
	ended := func(match func(sdktrace.ReadOnlySpan) bool) []sdktrace.ReadOnlySpan {
		var spans []sdktrace.ReadOnlySpan
		for _, span := range recorder.Ended() {
			if match(span) {
				spans = append(spans, span)
			}
		}
		return spans
	}

	// Queries outside of any span, e.g., of startup, record nothing.
	err := db.WithContext(context.Background()).Where("name = ?", "outside").First(&AccessToken{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	assert.Empty(t, ended(func(span sdktrace.ReadOnlySpan) bool {
		return span.InstrumentationScope().Name == "gogs.io/gogs/internal/database"
	}))

	ctx, parent := provider.Tracer("test").Start(context.Background(), "GET /")
	err = db.WithContext(ctx).Where("name = ?", "secret-value").First(&AccessToken{}).Error
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	parent.End()

	spans := ended(func(span sdktrace.ReadOnlySpan) bool {
		return span.SpanContext().TraceID() == parent.SpanContext().TraceID()
	})
	require.Len(t, spans, 2)
	query := spans[0]
	assert.Equal(t, "db SELECT access_token", query.Name())
	assert.Equal(t, parent.SpanContext().SpanID(), query.Parent().SpanID())
	// Not finding a record is an answer, not a failure.
	assert.Equal(t, codes.Unset, query.Status().Code)
	var text string
	for _, kv := range query.Attributes() {
		if kv.Key == "db.query.text" {
			text = kv.Value.AsString()
		}
	}
	assert.Contains(t, text, "access_token")
	assert.NotContains(t, text, "secret-value")
}
