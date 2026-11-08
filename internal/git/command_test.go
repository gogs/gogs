package git

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// slowCommand is a git command that runs for longer than the tests wait.
var slowCommand = []string{"-c", "alias.slow=!sleep 2", "slow"}

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
		_, err := exec(ctx, "", slowCommand, nil)
		assert.Equal(t, ErrExecTimeout, err)
	})
}

func TestExec_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := exec(ctx, "", slowCommand, nil)
	assert.ErrorIs(t, err, context.Canceled)
	// Must NOT be ErrExecTimeout, cancellation is distinct from deadline.
	assert.NotEqual(t, ErrExecTimeout, err)
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
