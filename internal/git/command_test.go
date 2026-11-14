package git

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// slowCommand is a git command whose child process runs for longer than the
// tests wait, so they only pass when the child is killed along with Git.
var slowCommand = []string{"-c", "alias.slow=!sleep 30", "slow"}

func TestExec_ContextTimeout(t *testing.T) {
	t.Run("context already expired before start", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
		defer cancel()
		time.Sleep(time.Millisecond) // ensure deadline has passed
		_, err := exec(ctx, "", []string{"version"}, nil)
		assert.Equal(t, ErrExecTimeout, err)
	})

	t.Run("context deadline fires mid-execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		start := time.Now()
		_, err := exec(ctx, "", slowCommand, nil)
		assert.Equal(t, ErrExecTimeout, err)
		assert.Less(t, time.Since(start), 5*time.Second)
	})
}

func TestExec_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := exec(ctx, "", slowCommand, nil)
	assert.ErrorIs(t, err, context.Canceled)
	assert.Less(t, time.Since(start), 5*time.Second)
	// Must NOT be ErrExecTimeout, cancellation is distinct from deadline.
	assert.NotEqual(t, ErrExecTimeout, err)
}

func TestExec_BackgroundJobKeepsOutputOpen(t *testing.T) {
	old := waitDelay
	waitDelay = 200 * time.Millisecond
	defer func() { waitDelay = old }()

	start := time.Now()
	_, err := exec(context.Background(), "", []string{"-c", "alias.bg=!sleep 3 &", "bg"}, nil)
	assert.NoError(t, err)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestPipe_SlowWriterGetsAllOutput(t *testing.T) {
	old := waitDelay
	waitDelay = 200 * time.Millisecond
	defer func() { waitDelay = old }()

	// Git exits long before the writer takes the rest of its output, and the
	// output must not be cut off at the wait delay.
	w := &slowWriter{delay: 300 * time.Millisecond}
	err := pipe(context.Background(), "", []string{"-c", "alias.big=!head -c 300000 /dev/zero", "big"}, nil, w)
	require.NoError(t, err)
	assert.Equal(t, 300000, w.n)
}

type slowWriter struct {
	delay time.Duration
	n     int
}

func (w *slowWriter) Write(p []byte) (int, error) {
	time.Sleep(w.delay)
	w.n += len(p)
	return len(p), nil
}

func TestWithTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		ctx, cancel := WithTimeout(context.Background(), timeout)
		_, err := exec(ctx, "", []string{"version"}, nil)
		cancel()
		assert.NoError(t, err, "timeout %v", timeout)
	}

	ctx, cancel := WithTimeout(context.Background(), time.Minute)
	defer cancel()
	deadline, ok := ctx.Deadline()
	assert.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(time.Minute), deadline, 5*time.Second)
}

func TestTracing(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	old := otel.GetTracerProvider()
	otel.SetTracerProvider(sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder)))
	defer otel.SetTracerProvider(old)

	ctx := context.Background()
	_, err := testrepo.RevParse(ctx, "master")
	require.NoError(t, err)
	_, err = LsRemote(ctx, "http://alice:secret@127.0.0.1:1/repo.git")
	require.Error(t, err)

	spans := recorder.Ended()
	byName := make(map[string]sdktrace.ReadOnlySpan, len(spans))
	for _, span := range spans {
		byName[span.Name()] = span
	}
	for _, name := range []string{"git.Repository.RevParse", "git rev-parse", "git.LsRemote", "git ls-remote"} {
		require.Contains(t, byName, name)
	}

	// The spans of Git processes are children of the spans of the methods that
	// run them.
	assert.Equal(t, byName["git.Repository.RevParse"].SpanContext().SpanID(), byName["git rev-parse"].Parent().SpanID())
	assert.Equal(t, byName["git.LsRemote"].SpanContext().SpanID(), byName["git ls-remote"].Parent().SpanID())

	// Arguments are left out, because they may contain credentials.
	lsRemote := byName["git ls-remote"]
	assert.Equal(t, "Error", lsRemote.Status().Code.String())
	assert.NotContains(t, fmt.Sprint(lsRemote.Attributes(), lsRemote.Events(), lsRemote.Status()), "secret")

	t.Run("passed on to Git", func(t *testing.T) {
		t.Setenv("GOGS_TEST_INHERITED", "kept")
		stdout, err := exec(ctx, "", []string{"-c", "alias.env=!printenv TRACEPARENT && printenv GOGS_TEST_INHERITED", "env"}, nil)
		require.NoError(t, err)

		var span sdktrace.ReadOnlySpan
		for _, s := range recorder.Ended() {
			if s.Name() == "git env" {
				span = s
			}
		}
		require.NotNil(t, span)
		// The rest of the environment is still inherited.
		want := fmt.Sprintf("00-%s-%s-01\nkept\n", span.SpanContext().TraceID(), span.SpanContext().SpanID())
		assert.Equal(t, want, string(stdout))
	})
}
