package git

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
