package planner

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const supersededReason = "Canceling since a higher priority waiting request for '%s' exists"

// A new push cancels the run in progress in its group. The run's always()
// job still runs, and the new run starts once the old one is done.
func TestConcurrencyCancelInProgress(t *testing.T) {
	src := `
on: push
concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true
jobs:
  build:
    runs-on: x
    steps: []
  test:
    needs: build
    runs-on: x
    steps: []
  cleanup:
    needs: test
    if: always()
    runs-on: x
    steps: []
`
	var groups Groups
	old := plan(t, src, Options{GitHub: githubContext("push", "refs/heads/Main")}, nil)
	start, cancel := groups.Request(Claim{Run: 1}, "run 1", old.Concurrency)
	require.True(t, start)
	require.Empty(t, cancel)

	// Group names are case-insensitive.
	next := plan(t, src, Options{GitHub: githubContext("push", "refs/heads/main")}, nil)
	start, cancel = groups.Request(Claim{Run: 2}, "run 2", next.Concurrency)
	assert.False(t, start, "the new run waits until the old one is cancelled")
	assert.Equal(t, []Cancellation{{Claim: Claim{Run: 1}, Started: true,
		Reason: fmt.Sprintf(supersededReason, "ci-refs/heads/main")}}, cancel)

	changes := old.CancelClaim("")
	assert.Equal(t, []string{"build"}, keys(changes.Cancel), "the scheduler stops the running job")
	complete(t, old, "build", StatusCancelled, nil)
	assert.Equal(t, map[string]Status{"build": StatusCancelled, "test": StatusCancelled, "cleanup": StatusQueued}, statuses(old))
	complete(t, old, "cleanup", StatusSuccess, nil)
	require.True(t, old.Done())
	assert.Equal(t, StatusCancelled, old.Result())

	assert.Equal(t, []Claim{{Run: 2}}, groups.Release(Claim{Run: 1}))
	assert.Empty(t, groups.Groups["ci-refs/heads/main"].Pending)
}

// Without cancel-in-progress, a new run waits, and replaces the run that was
// already waiting. That run never starts, so not even its always() jobs run.
func TestConcurrencyPendingIsReplaced(t *testing.T) {
	src := `
on: push
concurrency: deploy
jobs:
  deploy:
    runs-on: x
    steps: []
  notify:
    needs: deploy
    if: always()
    runs-on: x
    steps: []
`
	var groups Groups
	c := plan(t, src, Options{}, nil).Concurrency
	start, _ := groups.Request(Claim{Run: 1}, "run 1", c)
	require.True(t, start)
	start, cancel := groups.Request(Claim{Run: 2}, "run 2", c)
	assert.False(t, start)
	assert.Empty(t, cancel)

	waiting := plan(t, src, Options{}, nil)
	start, cancel = groups.Request(Claim{Run: 3}, "run 3", c)
	assert.False(t, start)
	assert.Equal(t, []Cancellation{{Claim: Claim{Run: 2}, Reason: fmt.Sprintf(supersededReason, "deploy")}}, cancel)

	changes := waiting.Discard("")
	assert.Empty(t, changes.Cancel, "nothing of a waiting run was running")
	assert.Equal(t, map[string]Status{"deploy": StatusCancelled, "notify": StatusCancelled}, statuses(waiting))
	assert.True(t, waiting.Done())
	assert.Empty(t, groups.Release(Claim{Run: 2}), "run 2 held nothing")

	assert.Equal(t, []Claim{{Run: 3}}, groups.Release(Claim{Run: 1}))
}

// With queue: max, runs wait in FIFO order, up to 100 of them.
func TestConcurrencyQueueMax(t *testing.T) {
	c := &Concurrency{Group: "prod", Queue: "max"}
	var groups Groups
	for run := int64(1); run <= maxPending+1; run++ {
		start, cancel := groups.Request(Claim{Run: run}, "", c)
		assert.Equal(t, run == 1, start)
		assert.Empty(t, cancel)
	}
	_, cancel := groups.Request(Claim{Run: 999}, "", c)
	assert.Equal(t, []Cancellation{{Claim: Claim{Run: 999}, Reason: "Canceling since the queue for concurrency group 'prod' is full"}}, cancel)

	assert.Equal(t, []Claim{{Run: 2}}, groups.Release(Claim{Run: 1}))
	assert.Equal(t, []Claim{{Run: 3}}, groups.Release(Claim{Run: 2}))

	_, err := tryPlan("on: push\nconcurrency:\n  group: g\n  queue: max\n  cancel-in-progress: true\njobs:\n  a:\n    runs-on: x\n    steps: []\n", Options{}, nil)
	assert.ErrorContains(t, err, "Concurrency queue 'max' cannot be combined with 'cancel-in-progress: true'")
}

// A job that asks for the group its own run holds would wait forever, so it
// is cancelled instead, as GitHub does.
func TestConcurrencyDeadlock(t *testing.T) {
	var groups Groups
	start, _ := groups.Request(Claim{Run: 1}, "run", &Concurrency{Group: "release"})
	require.True(t, start)
	start, cancel := groups.Request(Claim{Run: 1, Key: "publish"}, "publish", &Concurrency{Group: "Release"})
	assert.False(t, start)
	assert.Equal(t, []Cancellation{{Claim: Claim{Run: 1, Key: "publish"},
		Reason: "Canceling since a deadlock for concurrency group 'Release' was detected between 'top level workflow' and 'publish'"}}, cancel)

	start, _ = groups.Request(Claim{Run: 2, Key: "publish"}, "publish", &Concurrency{Group: "release"})
	assert.False(t, start, "a job of another run waits")
}

// A call of a reusable workflow holds its concurrency group for all of its
// jobs, and a newer claim cancels the call alone, not the run.
func TestConcurrencyCall(t *testing.T) {
	reusable := files{".github/workflows/deploy.yml": `
on: workflow_call
jobs:
  plan:
    runs-on: x
    steps: []
  apply:
    needs: plan
    runs-on: x
    steps: []
  unlock:
    needs: apply
    if: always()
    runs-on: x
    steps: []
`}
	src := `
on: push
jobs:
  deploy:
    concurrency:
      group: production
      cancel-in-progress: true
    uses: ./.github/workflows/deploy.yml
  report:
    needs: deploy
    if: always()
    runs-on: x
    steps: []
`
	r := plan(t, src, Options{}, reusable)
	job := r.Job("deploy/plan")
	require.Len(t, job.CallConcurrency, 1)
	assert.Equal(t, CallConcurrency{"deploy/", &Concurrency{Group: "production", CancelInProgress: true}}, job.CallConcurrency[0])

	complete(t, r, "deploy/plan", StatusSuccess, nil)
	changes := r.CancelClaim("deploy/")
	assert.Equal(t, []string{"deploy/apply"}, keys(changes.Cancel))
	changes = complete(t, r, "deploy/apply", StatusCancelled, nil)
	assert.Empty(t, changes.Calls, "unlock still runs in the call")
	changes = complete(t, r, "deploy/unlock", StatusSuccess, nil)
	assert.Equal(t, []string{"deploy/"}, changes.Calls, "the call releases its group")
	assert.Equal(t, StatusQueued, r.Job("report").Status, "the run goes on")

	r = plan(t, src, Options{}, reusable)
	r.Discard("deploy/")
	assert.Equal(t, map[string]Status{
		"deploy/plan": StatusCancelled, "deploy/apply": StatusCancelled, "deploy/unlock": StatusCancelled,
		"report": StatusQueued,
	}, statuses(r), "a call that never started runs none of its jobs")
}
