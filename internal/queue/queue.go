// Package queue runs background jobs that are kept in the database, with a
// pool of workers per job kind, retries with backoff and leases that hand the
// jobs of a stopped worker to another one.
package queue

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/database"
)

// Handler does the work of a job. A returned error makes the job retry after a
// backoff until it runs out of attempts, unless the error is permanent.
type Handler func(ctx context.Context, job *database.BackgroundJob) error

type permanentError struct {
	error
}

func (e permanentError) Unwrap() error {
	return e.error
}

// Permanent marks an error of a handler as one that a retry would not fix, so
// the job fails right away.
func Permanent(err error) error {
	return permanentError{err}
}

// Options contains the options for running the workers of a job kind.
type Options struct {
	// Workers is the number of jobs of the kind that run at the same time.
	Workers int
	// Lease is how long a worker holds a job before another worker may take it
	// over. Workers extend the lease while the job runs.
	Lease time.Duration
	// PollInterval is how often idle workers look for jobs that were enqueued
	// by another process or whose backoff ended.
	PollInterval time.Duration
	// MinBackoff and MaxBackoff bound the delay before a retry, which doubles
	// with each attempt.
	MinBackoff time.Duration
	MaxBackoff time.Duration
}

// DefaultOptions returns the options with the given number of workers and
// defaults for everything else.
func DefaultOptions(workers int) Options {
	return Options{
		Workers:      workers,
		Lease:        5 * time.Minute,
		PollInterval: 5 * time.Second,
		MinBackoff:   30 * time.Second,
		MaxBackoff:   time.Hour,
	}
}

// Run runs the workers of the given job kind until the context is canceled,
// then waits for the jobs they are running to return.
func Run(ctx context.Context, store *database.BackgroundJobsStore, kind database.BackgroundJobKind, opts Options, handle Handler) {
	var wg sync.WaitGroup
	for range max(opts.Workers, 1) {
		wg.Go(func() {
			w := &worker{store: store, kind: kind, opts: opts, handle: handle}
			w.run(ctx)
		})
	}
	wg.Wait()
}

type worker struct {
	store  *database.BackgroundJobsStore
	kind   database.BackgroundJobKind
	opts   Options
	handle Handler
}

func (w *worker) run(ctx context.Context) {
	wakeup := w.store.Wakeup(w.kind)
	ticker := time.NewTicker(w.opts.PollInterval)
	defer ticker.Stop()
	for {
		job, err := w.store.Claim(ctx, w.kind, w.opts.Lease)
		if err != nil && ctx.Err() == nil {
			log.Error("Failed to claim a %q job: %v", w.kind, err)
		}
		if job != nil {
			// Wake another idle worker in case more jobs are due.
			select {
			case wakeup <- struct{}{}:
			default:
			}
			w.process(ctx, job)
			continue
		}

		select {
		case <-ctx.Done():
			return
		case <-wakeup:
		case <-ticker.C:
		}
	}
}

func (w *worker) process(ctx context.Context, job *database.BackgroundJob) {
	jobCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Keep the lease while the job runs, and stop the job once another worker
	// has taken it over.
	heartbeatDone := make(chan struct{})
	go func() {
		defer close(heartbeatDone)
		ticker := time.NewTicker(w.opts.Lease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-jobCtx.Done():
				return
			case <-ticker.C:
				err := w.store.Extend(jobCtx, job, w.opts.Lease)
				if errors.Is(err, database.ErrBackgroundJobLost) {
					log.Warn("Lost the lease of the %q job %d", w.kind, job.ID)
					cancel()
					return
				} else if err != nil && jobCtx.Err() == nil {
					log.Error("Failed to extend the lease of the %q job %d: %v", w.kind, job.ID, err)
				}
			}
		}
	}()

	jobErr := w.call(jobCtx, job)
	cancel()
	<-heartbeatDone

	// Record the result even when the server is stopping, so that the job does
	// not run again.
	ctx = context.WithoutCancel(ctx)
	var err error
	switch {
	case jobErr == nil:
		err = w.store.Complete(ctx, job)
	case errors.As(jobErr, new(permanentError)) || job.Attempts >= job.MaxAttempts:
		log.Error("The %q job %d failed after %d attempts: %v", w.kind, job.ID, job.Attempts, jobErr)
		err = w.store.Fail(ctx, job, jobErr)
	default:
		delay := w.backoff(job.Attempts)
		log.Warn("The %q job %d failed at attempt %d and retries in %s: %v", w.kind, job.ID, job.Attempts, delay, jobErr)
		err = w.store.Retry(ctx, job, jobErr, delay)
	}
	if err != nil && !errors.Is(err, database.ErrBackgroundJobLost) {
		log.Error("Failed to record the result of the %q job %d: %v", w.kind, job.ID, err)
	}
}

func (w *worker) call(ctx context.Context, job *database.BackgroundJob) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf("panic: %v", r)
		}
	}()
	return w.handle(ctx, job)
}

// backoff returns the delay before the retry after the given attempt: the
// minimum backoff doubled for each earlier attempt up to the maximum, with up
// to half of it random so that jobs that failed together do not retry
// together.
func (w *worker) backoff(attempt int) time.Duration {
	delay := w.opts.MinBackoff
	for i := 1; i < attempt && delay < w.opts.MaxBackoff; i++ {
		delay *= 2
	}
	delay = min(delay, w.opts.MaxBackoff)
	return delay/2 + rand.N(delay/2+1)
}
