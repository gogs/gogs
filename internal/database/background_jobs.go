package database

import (
	"context"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"
)

// BackgroundJobKind is the kind of work a job does, which decides the worker pool that
// runs it.
type BackgroundJobKind string

// BackgroundJobPriority orders jobs of the same kind that are due. Lower
// values run first.
type BackgroundJobPriority int

const (
	BackgroundJobPriorityHigh   BackgroundJobPriority = 50
	BackgroundJobPriorityNormal BackgroundJobPriority = 100
	BackgroundJobPriorityLow    BackgroundJobPriority = 150
)

// BackgroundJobState is the state of a job.
type BackgroundJobState string

const (
	// BackgroundJobStatePending is a job waiting to run, either for the first time or for
	// a retry after its backoff.
	BackgroundJobStatePending BackgroundJobState = "pending"
	// BackgroundJobStateRunning is a job a worker holds until its lease ends.
	BackgroundJobStateRunning BackgroundJobState = "running"
	// BackgroundJobStateDone is a job that succeeded.
	BackgroundJobStateDone BackgroundJobState = "done"
	// BackgroundJobStateFailed is a job that failed permanently or ran out of attempts.
	BackgroundJobStateFailed BackgroundJobState = "failed"
)

// BackgroundJob is a unit of background work that is kept in the database until it
// succeeds or runs out of attempts, so it survives restarts and can be retried.
type BackgroundJob struct {
	ID   int64             `gorm:"primaryKey"`
	Kind BackgroundJobKind `gorm:"type:VARCHAR(32);not null;index:idx_background_jobs_kind_state_priority_run_at;index:idx_background_jobs_kind_unique_key"`
	// UniqueKey identifies the work, e.g., the ID of the mirror to sync.
	// Enqueueing does nothing while a pending job of the same kind has the same
	// non-empty key.
	UniqueKey string             `gorm:"type:VARCHAR(255);not null;default:'';index:idx_background_jobs_kind_unique_key"`
	Payload   []byte             `gorm:"not null"`
	State     BackgroundJobState `gorm:"type:VARCHAR(16);not null;index:idx_background_jobs_kind_state_priority_run_at"`
	// Priority orders the jobs that are due. It never makes a job run before
	// RunAt.
	Priority    BackgroundJobPriority `gorm:"type:SMALLINT;not null;default:100;index:idx_background_jobs_kind_state_priority_run_at"`
	Attempts    int                   `gorm:"not null;default:0"`
	MaxAttempts int                   `gorm:"not null"`
	// RunAt is the earliest time the job may run.
	RunAt time.Time `gorm:"not null;index:idx_background_jobs_kind_state_priority_run_at"`
	// LeaseExpiresAt is when the lease of the worker running the job expires,
	// and NULL when no worker runs it. Another worker may take the job over
	// once the lease expires.
	LeaseExpiresAt *time.Time
	LastError      string    `gorm:"type:TEXT"`
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

// TableName implements the GORM table name interface. New tables use plural
// names.
func (*BackgroundJob) TableName() string {
	return "background_jobs"
}

// ErrBackgroundJobLost is returned when a worker no longer holds the job, because its
// lease ended and another worker took the job over.
var ErrBackgroundJobLost = errors.New("the job is no longer held by this worker")

// BackgroundJobsTable is the table of the jobs that Gogs runs in the
// background, e.g., sending emails.
const BackgroundJobsTable = "background_jobs"

// BackgroundJobsStore is the storage layer for background jobs in one table.
type BackgroundJobsStore struct {
	db    *gorm.DB
	table string
}

// NewBackgroundJobsStore returns the storage layer for background jobs in the
// given table. Systems that keep their jobs apart from the others, e.g., to
// keep them for longer, use a table of their own with every column of
// BackgroundJob.
func NewBackgroundJobsStore(db *gorm.DB, table string) *BackgroundJobsStore {
	return &BackgroundJobsStore{db: db, table: table}
}

// query returns a query on the table of the store, which can be reused for
// several statements.
func (s *BackgroundJobsStore) query(ctx context.Context) *gorm.DB {
	return s.db.WithContext(ctx).Table(s.table).Session(&gorm.Session{})
}

// now returns the current time in UTC, which keeps times comparable on SQLite,
// where they are stored as text.
func (s *BackgroundJobsStore) now() time.Time {
	return s.db.NowFunc().UTC()
}

func (db *DB) BackgroundJobs() *BackgroundJobsStore {
	return NewBackgroundJobsStore(db.db, BackgroundJobsTable)
}

type jobWakeupKey struct {
	table string
	kind  BackgroundJobKind
}

// jobWakeups has one channel per table and job kind that is signaled when a
// job of the kind is enqueued in this process, so idle workers start without
// waiting for their next poll.
var jobWakeups sync.Map // map[jobWakeupKey]chan struct{}

// Wakeup returns the channel that is signaled when a job of the given kind is
// enqueued in the table of the store in this process.
func (s *BackgroundJobsStore) Wakeup(kind BackgroundJobKind) chan struct{} {
	ch, _ := jobWakeups.LoadOrStore(jobWakeupKey{table: s.table, kind: kind}, make(chan struct{}, 1))
	return ch.(chan struct{})
}

func (s *BackgroundJobsStore) wake(kind BackgroundJobKind) {
	select {
	case s.Wakeup(kind) <- struct{}{}:
	default:
	}
}

// EnqueueBackgroundJobOptions contains the options for enqueueing a job.
type EnqueueBackgroundJobOptions struct {
	// UniqueKey identifies the work. When a pending job of the same kind has the
	// same key, enqueueing adds no job and gives the pending one the higher of
	// the two priorities and the earlier of the two times to run.
	UniqueKey string
	// Priority defaults to BackgroundJobPriorityNormal.
	Priority BackgroundJobPriority
	// RunAt is the earliest time the job may run. It defaults to now.
	RunAt time.Time
	// MaxAttempts is the number of times the job runs before it is marked as
	// failed. It is at least 1.
	MaxAttempts int
}

// Enqueue adds a job of the given kind. Pass a store of a transaction to
// enqueue the job only when the transaction commits.
func (s *BackgroundJobsStore) Enqueue(ctx context.Context, kind BackgroundJobKind, payload []byte, opts EnqueueBackgroundJobOptions) error {
	now := s.now()
	job := &BackgroundJob{
		Kind:        kind,
		UniqueKey:   opts.UniqueKey,
		Payload:     payload,
		State:       BackgroundJobStatePending,
		Priority:    opts.Priority,
		MaxAttempts: max(opts.MaxAttempts, 1),
		RunAt:       opts.RunAt.UTC(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if job.Payload == nil {
		job.Payload = []byte{}
	}
	if job.Priority == 0 {
		job.Priority = BackgroundJobPriorityNormal
	}
	if opts.RunAt.IsZero() {
		job.RunAt = now
	}

	// The check is not atomic with the insert, so two concurrent enqueues of
	// the same key may both add a job. Jobs with a unique key must be safe to
	// run twice.
	if opts.UniqueKey != "" {
		merged, err := s.mergeIntoPending(ctx, kind, opts.UniqueKey, job.Priority, job.RunAt)
		if err != nil {
			return errors.Wrap(err, "merge into pending job with the same key")
		}
		if merged {
			s.wake(kind)
			return nil
		}
	}

	err := s.query(ctx).Create(job).Error
	if err != nil {
		return errors.Wrap(err, "create")
	}
	s.wake(kind)
	return nil
}

// mergeIntoPending gives the pending job of the given kind and unique key the
// higher of its own and the given priority, and the earlier of its own and the
// given time to run. It returns false when there is no such job.
func (s *BackgroundJobsStore) mergeIntoPending(ctx context.Context, kind BackgroundJobKind, uniqueKey string, priority BackgroundJobPriority, runAt time.Time) (bool, error) {
	var pending BackgroundJob
	err := s.query(ctx).
		Select("id").
		Where("kind = ? AND unique_key = ? AND state = ?", kind, uniqueKey, BackgroundJobStatePending).
		Limit(1).
		Find(&pending).Error
	if err != nil {
		return false, errors.Wrap(err, "find pending job")
	}
	if pending.ID == 0 {
		return false, nil
	}

	// The state condition leaves a job alone that a worker claimed meanwhile.
	err = s.query(ctx).Model(&BackgroundJob{}).
		Where("id = ? AND state = ?", pending.ID, BackgroundJobStatePending).
		Updates(map[string]any{
			"priority":   gorm.Expr("CASE WHEN priority > ? THEN ? ELSE priority END", priority, priority),
			"run_at":     gorm.Expr("CASE WHEN run_at > ? THEN ? ELSE run_at END", runAt, runAt),
			"updated_at": s.now(),
		}).Error
	if err != nil {
		return false, errors.Wrap(err, "update pending job")
	}
	return true, nil
}

// Claim takes the next job of the given kind that is due, or a running job
// whose lease has ended, and holds it for the given lease. It returns nil when
// no job is due.
func (s *BackgroundJobsStore) Claim(ctx context.Context, kind BackgroundJobKind, lease time.Duration) (*BackgroundJob, error) {
	db := s.query(ctx)
	now := s.now()

	// A job whose worker stopped during its last attempt has no attempts left.
	err := db.Model(&BackgroundJob{}).
		Where("kind = ? AND state = ? AND lease_expires_at < ? AND attempts >= max_attempts", kind, BackgroundJobStateRunning, now).
		Updates(map[string]any{
			"state":            BackgroundJobStateFailed,
			"lease_expires_at": nil,
			"last_error":       "the worker stopped during the last attempt",
			"updated_at":       now,
		}).Error
	if err != nil {
		return nil, errors.Wrap(err, "fail abandoned jobs")
	}

	claimable := db.Where(
		"kind = ? AND ((state = ? AND run_at <= ?) OR (state = ? AND lease_expires_at < ?))",
		kind, BackgroundJobStatePending, now, BackgroundJobStateRunning, now,
	)

	// Another worker may claim the candidate first, then try the next one.
	for range 3 {
		var candidate BackgroundJob
		err = claimable.Session(&gorm.Session{}).
			Select("id").
			Order("priority, run_at, id").
			Limit(1).
			Find(&candidate).Error
		if err != nil {
			return nil, errors.Wrap(err, "find candidate")
		}
		if candidate.ID == 0 {
			return nil, nil
		}

		// The conditional update is the claim, which works the same on every
		// database without row locks.
		result := claimable.Session(&gorm.Session{}).
			Model(&BackgroundJob{}).
			Where("id = ?", candidate.ID).
			Updates(map[string]any{
				"state":            BackgroundJobStateRunning,
				"attempts":         gorm.Expr("attempts + 1"),
				"lease_expires_at": now.Add(lease),
				"updated_at":       now,
			})
		if result.Error != nil {
			return nil, errors.Wrap(result.Error, "claim")
		}
		if result.RowsAffected == 0 {
			continue
		}

		var job BackgroundJob
		err = db.Where("id = ?", candidate.ID).First(&job).Error
		if err != nil {
			return nil, errors.Wrap(err, "get claimed job")
		}
		return &job, nil
	}
	return nil, nil
}

// held returns the query for the given job while this worker holds it. The
// attempt number tells this worker's claim apart from a later one by another
// worker after the lease ended.
func (s *BackgroundJobsStore) held(ctx context.Context, job *BackgroundJob) *gorm.DB {
	return s.query(ctx).Model(&BackgroundJob{}).
		Where("id = ? AND state = ? AND attempts = ?", job.ID, BackgroundJobStateRunning, job.Attempts)
}

func updatedOrLost(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrBackgroundJobLost
	}
	return nil
}

// Extend extends the lease of a job held by this worker.
func (s *BackgroundJobsStore) Extend(ctx context.Context, job *BackgroundJob, lease time.Duration) error {
	now := s.now()
	result := s.held(ctx, job).Updates(map[string]any{
		"lease_expires_at": now.Add(lease),
		"updated_at":       now,
	})
	if result.Error != nil || result.RowsAffected > 0 {
		return result.Error
	}

	// MySQL reports no affected rows when the values did not change within the
	// same second.
	var count int64
	err := s.held(ctx, job).Count(&count).Error
	if err != nil {
		return errors.Wrap(err, "count held job")
	}
	if count == 0 {
		return ErrBackgroundJobLost
	}
	return nil
}

// Complete marks a job held by this worker as done.
func (s *BackgroundJobsStore) Complete(ctx context.Context, job *BackgroundJob) error {
	return updatedOrLost(s.held(ctx, job).Updates(map[string]any{
		"state":            BackgroundJobStateDone,
		"lease_expires_at": nil,
		"last_error":       "",
		"updated_at":       s.now(),
	}))
}

// Fail marks a job held by this worker as failed for good.
func (s *BackgroundJobsStore) Fail(ctx context.Context, job *BackgroundJob, jobErr error) error {
	return updatedOrLost(s.held(ctx, job).Updates(map[string]any{
		"state":            BackgroundJobStateFailed,
		"lease_expires_at": nil,
		"last_error":       jobErr.Error(),
		"updated_at":       s.now(),
	}))
}

// Retry makes a job held by this worker pending again after the given delay.
// When the work has been enqueued again in the meantime, the job is deleted
// and its priority and time to run are merged into the newer job, which will
// do the work.
func (s *BackgroundJobsStore) Retry(ctx context.Context, job *BackgroundJob, jobErr error, delay time.Duration) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txStore := NewBackgroundJobsStore(tx, s.table)
		now := s.now()
		if job.UniqueKey != "" {
			merged, err := txStore.mergeIntoPending(ctx, job.Kind, job.UniqueKey, job.Priority, now.Add(delay))
			if err != nil {
				return errors.Wrap(err, "merge into pending job with the same key")
			}
			if merged {
				return updatedOrLost(txStore.held(ctx, job).Delete(&BackgroundJob{}))
			}
		}

		return updatedOrLost(txStore.held(ctx, job).Updates(map[string]any{
			"state":            BackgroundJobStatePending,
			"run_at":           now.Add(delay),
			"lease_expires_at": nil,
			"last_error":       jobErr.Error(),
			"updated_at":       now,
		}))
	})
}

// DeleteFinished deletes jobs that were done before doneBefore, and jobs that
// failed before failedBefore.
func (s *BackgroundJobsStore) DeleteFinished(ctx context.Context, doneBefore, failedBefore time.Time) error {
	return s.query(ctx).
		Where("(state = ? AND updated_at < ?) OR (state = ? AND updated_at < ?)",
			BackgroundJobStateDone, doneBefore, BackgroundJobStateFailed, failedBefore).
		Delete(&BackgroundJob{}).Error
}
