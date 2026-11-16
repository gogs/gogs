package queue

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbtest"
)

func newTestStore(t *testing.T, suite string) (*database.BackgroundJobsStore, func() []*database.BackgroundJob) {
	db := dbtest.NewDB(t, suite)
	require.NoError(t, migrations.Migrate(db))
	// Workers wait for backoffs, which needs a clock that moves.
	db = db.Session(&gorm.Session{NowFunc: time.Now})
	list := func() []*database.BackgroundJob {
		var jobs []*database.BackgroundJob
		require.NoError(t, db.Order("id").Find(&jobs).Error)
		return jobs
	}
	return database.NewBackgroundJobsStore(db), list
}

func testOptions(workers int) Options {
	return Options{
		Workers:      workers,
		Lease:        time.Minute,
		PollInterval: 50 * time.Millisecond,
		MinBackoff:   time.Millisecond,
		MaxBackoff:   time.Millisecond,
	}
}

// runUntil runs the workers until done returns true.
func runUntil(t *testing.T, store *database.BackgroundJobsStore, kind database.BackgroundJobKind, opts Options, handle Handler, done func() bool) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		Run(ctx, store, kind, opts, handle)
		close(stopped)
	}()
	assert.Eventually(t, done, 30*time.Second, 10*time.Millisecond)
	cancel()
	select {
	case <-stopped:
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not return after the context was canceled")
	}
}

func TestRun(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	t.Run("every job runs once", func(t *testing.T) {
		store, list := newTestStore(t, "queueRunOnce")
		const kind = "run_once"
		const total = 40
		ctx := context.Background()
		for i := range total {
			require.NoError(t, store.Enqueue(ctx, kind, []byte(strconv.Itoa(i)), database.EnqueueBackgroundJobOptions{}))
		}

		var mu sync.Mutex
		runs := make(map[string]int)
		running, maxRunning := 0, 0
		handle := func(ctx context.Context, job *database.BackgroundJob) error {
			mu.Lock()
			runs[string(job.Payload)]++
			running++
			maxRunning = max(maxRunning, running)
			mu.Unlock()
			time.Sleep(50 * time.Millisecond)
			mu.Lock()
			running--
			mu.Unlock()
			return nil
		}
		runUntil(t, store, kind, testOptions(4), handle, func() bool {
			for _, job := range list() {
				if job.State != database.BackgroundJobStateDone {
					return false
				}
			}
			return true
		})

		mu.Lock()
		defer mu.Unlock()
		assert.Len(t, runs, total)
		for payload, n := range runs {
			assert.Equal(t, 1, n, "job %s", payload)
		}
		assert.Greater(t, maxRunning, 1, "jobs should run at the same time")
		assert.LessOrEqual(t, maxRunning, 4)
	})

	t.Run("retries until success", func(t *testing.T) {
		store, list := newTestStore(t, "queueRetry")
		const kind = "retry"
		require.NoError(t, store.Enqueue(context.Background(), kind, nil, database.EnqueueBackgroundJobOptions{MaxAttempts: 5}))

		handle := func(ctx context.Context, job *database.BackgroundJob) error {
			if job.Attempts < 3 {
				return errors.Newf("attempt %d", job.Attempts)
			}
			return nil
		}
		runUntil(t, store, kind, testOptions(1), handle, func() bool {
			return list()[0].State == database.BackgroundJobStateDone
		})
		assert.Equal(t, 3, list()[0].Attempts)
	})

	t.Run("fails after the last attempt", func(t *testing.T) {
		store, list := newTestStore(t, "queueMaxAttempts")
		const kind = "max_attempts"
		require.NoError(t, store.Enqueue(context.Background(), kind, nil, database.EnqueueBackgroundJobOptions{MaxAttempts: 3}))

		handle := func(ctx context.Context, job *database.BackgroundJob) error {
			return errors.Newf("attempt %d", job.Attempts)
		}
		runUntil(t, store, kind, testOptions(1), handle, func() bool {
			return list()[0].State == database.BackgroundJobStateFailed
		})
		job := list()[0]
		assert.Equal(t, 3, job.Attempts)
		assert.Equal(t, "attempt 3", job.LastError)
	})

	t.Run("permanent errors and panics", func(t *testing.T) {
		store, list := newTestStore(t, "queuePermanent")
		const kind = "permanent"
		ctx := context.Background()
		require.NoError(t, store.Enqueue(ctx, kind, []byte("permanent"), database.EnqueueBackgroundJobOptions{MaxAttempts: 5}))
		require.NoError(t, store.Enqueue(ctx, kind, []byte("panic"), database.EnqueueBackgroundJobOptions{MaxAttempts: 1}))

		handle := func(ctx context.Context, job *database.BackgroundJob) error {
			if string(job.Payload) == "panic" {
				panic("boom")
			}
			return Permanent(errors.New("mailbox does not exist"))
		}
		runUntil(t, store, kind, testOptions(1), handle, func() bool {
			jobs := list()
			return jobs[0].State == database.BackgroundJobStateFailed && jobs[1].State == database.BackgroundJobStateFailed
		})
		jobs := list()
		assert.Equal(t, 1, jobs[0].Attempts)
		assert.Equal(t, "mailbox does not exist", jobs[0].LastError)
		assert.Equal(t, "panic: boom", jobs[1].LastError)
	})

	t.Run("jobs enqueued while idle start without waiting for a poll", func(t *testing.T) {
		store, list := newTestStore(t, "queueWakeup")
		const kind = "wakeup"
		opts := testOptions(1)
		opts.PollInterval = time.Hour

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go Run(ctx, store, kind, opts, func(context.Context, *database.BackgroundJob) error { return nil })

		// Let the worker find nothing and go idle first.
		time.Sleep(100 * time.Millisecond)
		require.NoError(t, store.Enqueue(ctx, kind, nil, database.EnqueueBackgroundJobOptions{}))
		assert.Eventually(t, func() bool {
			return list()[0].State == database.BackgroundJobStateDone
		}, 10*time.Second, 10*time.Millisecond)
	})

	t.Run("a stopping server records the result of running jobs", func(t *testing.T) {
		store, list := newTestStore(t, "queueShutdown")
		const kind = "shutdown"
		require.NoError(t, store.Enqueue(context.Background(), kind, nil, database.EnqueueBackgroundJobOptions{MaxAttempts: 3}))

		started := make(chan struct{})
		handle := func(ctx context.Context, _ *database.BackgroundJob) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		}
		ctx, cancel := context.WithCancel(context.Background())
		stopped := make(chan struct{})
		go func() {
			Run(ctx, store, kind, testOptions(1), handle)
			close(stopped)
		}()
		<-started
		cancel()
		<-stopped

		// The canceled attempt retries on the next start instead of waiting for
		// its lease to end.
		job := list()[0]
		assert.Equal(t, database.BackgroundJobStatePending, job.State)
		assert.Equal(t, "context canceled", job.LastError)
	})
}

func TestBackoff(t *testing.T) {
	w := &worker{opts: Options{MinBackoff: 10 * time.Second, MaxBackoff: time.Minute}}
	for _, tc := range []struct {
		attempt  int
		min, max time.Duration
	}{
		{1, 5 * time.Second, 10 * time.Second},
		{2, 10 * time.Second, 20 * time.Second},
		{3, 20 * time.Second, 40 * time.Second},
		{4, 30 * time.Second, time.Minute},
		{100, 30 * time.Second, time.Minute},
	} {
		for range 20 {
			got := w.backoff(tc.attempt)
			assert.GreaterOrEqual(t, got, tc.min, "attempt %d", tc.attempt)
			assert.LessOrEqual(t, got, tc.max, "attempt %d", tc.attempt)
		}
	}
}
