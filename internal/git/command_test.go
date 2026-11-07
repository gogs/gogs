package git

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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

		// Use cmd directly with a blocking stdin so the command starts successfully and
		// blocks reading until the context deadline fires.
		c, timeoutCancel := cmd(ctx, "", []string{"hash-object", "--stdin"}, nil)
		defer timeoutCancel()

		err := c.Input(blockingReader{cancel: ctx.Done()}).StdOut().Run().Stream(io.Discard)
		err = mapContextError(err, ctx)
		assert.Equal(t, ErrExecTimeout, err)
	})
}

// blockingReader is an io.Reader that blocks until its cancel channel is closed,
// simulating a stdin that never provides data. When canceled it returns io.EOF
// so that the stdin copy goroutine can exit cleanly, allowing cmd.Wait() to
// return.
type blockingReader struct {
	cancel <-chan struct{}
}

func (r blockingReader) Read(p []byte) (int, error) {
	<-r.cancel
	return 0, io.EOF
}

func TestCmd_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel in the background after a short delay so the command is already running
	// when cancellation arrives. Close done to unblock the reader.
	done := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
		close(done)
	}()

	c, timeoutCancel := cmd(ctx, "", []string{"hash-object", "--stdin"}, nil)
	defer timeoutCancel()

	err := c.Input(blockingReader{cancel: done}).StdOut().Run().Stream(io.Discard)
	err = mapContextError(err, ctx)
	assert.ErrorIs(t, err, context.Canceled)
	// Must NOT be ErrExecTimeout — cancellation is distinct from deadline.
	assert.NotEqual(t, ErrExecTimeout, err)
}

func TestExec_DefaultTimeoutApplied(t *testing.T) {
	// A plain context.Background() has no deadline. The command should still succeed
	// because DefaultTimeout is applied automatically and "git version" completes
	// well within that.
	ctx := context.Background()
	stdout, err := exec(ctx, "", []string{"version"}, nil)
	assert.NoError(t, err)
	assert.Contains(t, string(stdout), "git version")
}
