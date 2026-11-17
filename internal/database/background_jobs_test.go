package database

import (
	"context"
	"testing"
	"time"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestBackgroundJobs(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	ctx := context.Background()
	db := newTestDB(t, "BackgroundJobsStore")

	// Another system's table with the same columns, but without the indexes,
	// whose names are taken by background_jobs.
	type otherJob struct {
		ID             int64
		Kind           string `gorm:"not null"`
		UniqueKey      string `gorm:"not null;default:''"`
		Payload        []byte `gorm:"not null"`
		State          string `gorm:"not null"`
		Priority       int    `gorm:"not null;default:100"`
		Attempts       int    `gorm:"not null;default:0"`
		MaxAttempts    int    `gorm:"not null"`
		RunAt          time.Time
		LeaseExpiresAt *time.Time
		LastError      string
		CreatedAt      time.Time
		UpdatedAt      time.Time
	}
	const otherTable = "other_jobs"
	require.NoError(t, db.Table(otherTable).Migrator().CreateTable(&otherJob{}))

	countJobs := func(table string) int64 {
		var count int64
		require.NoError(t, db.Table(table).Count(&count).Error)
		return count
	}

	type testCase struct {
		name string
		test func(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time)
	}
	testCases := []testCase{
		{"Enqueue", jobsEnqueue},
		{"Claim", jobsClaim},
		{"Priority", jobsPriority},
		{"Merge", jobsMerge},
		{"Lease", jobsLease},
		{"Retry", jobsRetry},
		{"DeleteFinished", jobsDeleteFinished},
	}
	for _, tables := range [][2]string{{BackgroundJobsTable, otherTable}, {otherTable, BackgroundJobsTable}} {
		table, untouched := tables[0], tables[1]
		for _, tc := range testCases {
			t.Run(table+"/"+tc.name, func(t *testing.T) {
				t.Cleanup(func() {
					require.NoError(t, clearTables(t, db))
					require.NoError(t, db.Table(otherTable).Where("TRUE").Delete(&otherJob{}).Error)
				})
				clock := time.Unix(1700000000, 0).UTC()
				s := NewBackgroundJobsStore(db.Session(&gorm.Session{NowFunc: func() time.Time { return clock }}), table)
				tc.test(t, ctx, s, &clock)
				assert.Zero(t, countJobs(untouched), "jobs in %q", untouched)
			})
			if t.Failed() {
				return
			}
		}
	}
}

func listJobs(t *testing.T, s *BackgroundJobsStore) []*BackgroundJob {
	var jobs []*BackgroundJob
	require.NoError(t, s.query(context.Background()).Order("id").Find(&jobs).Error)
	return jobs
}

func jobsEnqueue(t *testing.T, ctx context.Context, s *BackgroundJobsStore, _ *time.Time) {
	opts := EnqueueBackgroundJobOptions{UniqueKey: "1", MaxAttempts: 3}
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("a"), opts))
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("b"), opts))
	// The same key of another kind is another job.
	require.NoError(t, s.Enqueue(ctx, "other", []byte("c"), opts))
	// Jobs without a key never conflict.
	require.NoError(t, s.Enqueue(ctx, "sync", nil, EnqueueBackgroundJobOptions{}))
	require.NoError(t, s.Enqueue(ctx, "sync", nil, EnqueueBackgroundJobOptions{}))

	jobs := listJobs(t, s)
	require.Len(t, jobs, 4)
	assert.Equal(t, []byte("a"), jobs[0].Payload)
	assert.Equal(t, 1, jobs[2].MaxAttempts)

	// Once a worker has started the job, the same work can be enqueued again
	// for the changes made since.
	job, err := s.Claim(ctx, "other", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	require.NoError(t, s.Enqueue(ctx, "other", []byte("d"), opts))
	require.NoError(t, s.Enqueue(ctx, "other", []byte("e"), opts))
	assert.Len(t, listJobs(t, s), 5)

	// Enqueueing in a transaction that rolls back adds nothing.
	err = s.db.Transaction(func(tx *gorm.DB) error {
		require.NoError(t, NewBackgroundJobsStore(tx, s.table).Enqueue(ctx, "sync", []byte("f"), EnqueueBackgroundJobOptions{}))
		return errors.New("roll back")
	})
	require.Error(t, err)
	assert.Len(t, listJobs(t, s), 5)
}

func jobsClaim(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("first"), EnqueueBackgroundJobOptions{}))
	*clock = clock.Add(time.Second)
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("second"), EnqueueBackgroundJobOptions{}))
	require.NoError(t, s.Enqueue(ctx, "other", []byte("other"), EnqueueBackgroundJobOptions{}))

	first, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.Equal(t, []byte("first"), first.Payload)
	assert.Equal(t, BackgroundJobStateRunning, first.State)
	assert.Equal(t, 1, first.Attempts)
	require.NotNil(t, first.LeaseExpiresAt)
	assert.True(t, clock.Add(time.Minute).Equal(*first.LeaseExpiresAt))

	second, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.Equal(t, []byte("second"), second.Payload)

	// Running jobs and jobs of other kinds are not claimed.
	none, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)

	require.NoError(t, s.Complete(ctx, first))
	require.NoError(t, s.Fail(ctx, second, errors.New("broken")))
	none, err = s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)

	jobs := listJobs(t, s)
	assert.Equal(t, BackgroundJobStateDone, jobs[0].State)
	assert.Equal(t, BackgroundJobStateFailed, jobs[1].State)
	assert.Equal(t, "broken", jobs[1].LastError)
	assert.Equal(t, BackgroundJobStatePending, jobs[2].State)
}

func jobsPriority(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	enqueue := func(payload string, priority BackgroundJobPriority, runAt time.Time) {
		opts := EnqueueBackgroundJobOptions{Priority: priority, RunAt: runAt}
		require.NoError(t, s.Enqueue(ctx, "sync", []byte(payload), opts))
	}
	// Enqueued first and due earliest, but the lowest priority.
	enqueue("low", BackgroundJobPriorityLow, time.Time{})
	*clock = clock.Add(time.Second)
	enqueue("normal", 0, time.Time{})
	enqueue("high later", BackgroundJobPriorityHigh, clock.Add(time.Minute))
	enqueue("high", BackgroundJobPriorityHigh, time.Time{})

	claimAll := func() []string {
		var got []string
		for {
			job, err := s.Claim(ctx, "sync", time.Hour)
			require.NoError(t, err)
			if job == nil {
				return got
			}
			got = append(got, string(job.Payload))
		}
	}
	// A higher priority never makes a job run before its time.
	assert.Equal(t, []string{"high", "normal", "low"}, claimAll())
	*clock = clock.Add(time.Minute)
	assert.Equal(t, []string{"high later"}, claimAll())
	assert.Equal(t, BackgroundJobPriorityNormal, listJobs(t, s)[1].Priority)
}

func jobsMerge(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	later := clock.Add(time.Hour)
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("scheduled"), EnqueueBackgroundJobOptions{
		UniqueKey: "1", Priority: BackgroundJobPriorityLow, RunAt: later, MaxAttempts: 3,
	}))

	// A less urgent duplicate changes nothing.
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("dup"), EnqueueBackgroundJobOptions{
		UniqueKey: "1", Priority: BackgroundJobPriorityLow, RunAt: later.Add(time.Hour),
	}))
	jobs := listJobs(t, s)
	require.Len(t, jobs, 1)
	assert.Equal(t, BackgroundJobPriorityLow, jobs[0].Priority)
	assert.True(t, later.Equal(jobs[0].RunAt))

	// "Sync now" makes the pending job more urgent and due now, and keeps its
	// payload and attempts.
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("now"), EnqueueBackgroundJobOptions{
		UniqueKey: "1", Priority: BackgroundJobPriorityHigh,
	}))
	jobs = listJobs(t, s)
	require.Len(t, jobs, 1)
	assert.Equal(t, BackgroundJobPriorityHigh, jobs[0].Priority)
	assert.True(t, clock.Equal(jobs[0].RunAt))
	assert.Equal(t, []byte("scheduled"), jobs[0].Payload)
	assert.Equal(t, 3, jobs[0].MaxAttempts)

	job, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, []byte("scheduled"), job.Payload)
}

func jobsLease(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	require.NoError(t, s.Enqueue(ctx, "sync", nil, EnqueueBackgroundJobOptions{MaxAttempts: 2}))

	held, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, held)

	// The lease is kept while the worker extends it.
	*clock = clock.Add(50 * time.Second)
	require.NoError(t, s.Extend(ctx, held, time.Minute))
	// Extending again within the same second changes no value.
	require.NoError(t, s.Extend(ctx, held, time.Minute))
	*clock = clock.Add(50 * time.Second)
	none, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)

	// Once the lease ends, another worker takes the job over and the first
	// worker can no longer record a result.
	*clock = clock.Add(11 * time.Second)
	takenOver, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, takenOver)
	assert.Equal(t, held.ID, takenOver.ID)
	assert.Equal(t, 2, takenOver.Attempts)
	assert.ErrorIs(t, s.Extend(ctx, held, time.Minute), ErrBackgroundJobLost)
	assert.ErrorIs(t, s.Complete(ctx, held), ErrBackgroundJobLost)
	assert.ErrorIs(t, s.Fail(ctx, held, errors.New("late")), ErrBackgroundJobLost)
	assert.ErrorIs(t, s.Retry(ctx, held, errors.New("late"), 0), ErrBackgroundJobLost)

	// When the worker of the last attempt stops, the job fails instead of
	// running again.
	*clock = clock.Add(2 * time.Minute)
	none, err = s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)
	jobs := listJobs(t, s)
	assert.Equal(t, BackgroundJobStateFailed, jobs[0].State)
	assert.Equal(t, "the worker stopped during the last attempt", jobs[0].LastError)
}

func jobsRetry(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	opts := EnqueueBackgroundJobOptions{UniqueKey: "1", MaxAttempts: 3}
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("a"), opts))

	job, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NoError(t, s.Retry(ctx, job, errors.New("timeout"), time.Minute))

	jobs := listJobs(t, s)
	require.Len(t, jobs, 1)
	assert.Equal(t, BackgroundJobStatePending, jobs[0].State)
	assert.Equal(t, "timeout", jobs[0].LastError)
	assert.True(t, clock.Add(time.Minute).Equal(jobs[0].RunAt))
	assert.Nil(t, jobs[0].LeaseExpiresAt)

	// The job waits for its backoff, and is still unique while it waits.
	none, err := s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	assert.Nil(t, none)
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("b"), opts))
	assert.Len(t, listJobs(t, s), 1)

	*clock = clock.Add(time.Minute)
	job, err = s.Claim(ctx, "sync", time.Minute)
	require.NoError(t, err)
	require.NotNil(t, job)
	assert.Equal(t, 2, job.Attempts)

	// When the work is enqueued again while the job runs, the newer job does
	// the work instead of a retry.
	require.NoError(t, s.Enqueue(ctx, "sync", []byte("c"), opts))
	require.NoError(t, s.Retry(ctx, job, errors.New("timeout"), time.Minute))
	jobs = listJobs(t, s)
	require.Len(t, jobs, 1)
	assert.Equal(t, []byte("c"), jobs[0].Payload)
	assert.Equal(t, 0, jobs[0].Attempts)
}

func jobsDeleteFinished(t *testing.T, ctx context.Context, s *BackgroundJobsStore, clock *time.Time) {
	finish := func(fail bool) {
		require.NoError(t, s.Enqueue(ctx, "sync", nil, EnqueueBackgroundJobOptions{}))
		job, err := s.Claim(ctx, "sync", time.Minute)
		require.NoError(t, err)
		if fail {
			require.NoError(t, s.Fail(ctx, job, errors.New("broken")))
		} else {
			require.NoError(t, s.Complete(ctx, job))
		}
	}
	start := *clock
	finish(false)
	finish(true)
	*clock = clock.Add(time.Hour)
	finish(false)
	finish(true)
	require.NoError(t, s.Enqueue(ctx, "sync", nil, EnqueueBackgroundJobOptions{}))

	// Done jobs are kept for less time than failed ones.
	*clock = clock.Add(time.Hour)
	require.NoError(t, s.DeleteFinished(ctx, start.Add(2*time.Hour), start.Add(30*time.Minute)))

	var states []BackgroundJobState
	for _, job := range listJobs(t, s) {
		states = append(states, job.State)
	}
	assert.Equal(t, []BackgroundJobState{BackgroundJobStateFailed, BackgroundJobStatePending}, states)
}
