package planner

import (
	"fmt"
	"maps"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/model"
)

// files serves workflow files by path in the repository org/app at the commit
// abc, or by full name "{owner}/{repo}/{path}@{ref}". The workflow being
// planned is ".github/workflows/ci.yml".
type files map[string]string

func (f files) ReadFile(name string) (string, error) {
	if src, ok := f[name]; ok {
		return src, nil
	}
	if path, ok := strings.CutPrefix(name, "org/app/"); ok {
		if src, ok := f[strings.TrimSuffix(path, "@abc")]; ok {
			return src, nil
		}
	}
	return "", errors.Newf("%s not found", name)
}

func (f files) ResolveRef(_, ref string) (string, error) {
	return "org/app/" + strings.TrimPrefix(ref, "./") + "@abc", nil
}

const workflowPath = ".github/workflows/ci.yml"

func load(src string, f files) (*model.Workflow, error) {
	all := files{workflowPath: src}
	maps.Copy(all, f)
	return model.Load(all, workflowPath, model.DefaultOptions())
}

func plan(t *testing.T, src string, opts Options, f files) *Run {
	t.Helper()
	r, err := tryPlan(src, opts, f)
	require.NoError(t, err)
	return r
}

func tryPlan(src string, opts Options, f files) (*Run, error) {
	w, err := load(src, f)
	if err != nil {
		return nil, err
	}
	opts.Workflow = w
	return Plan(opts)
}

func statuses(r *Run) map[string]Status {
	m := map[string]Status{}
	for _, j := range r.Jobs {
		m[j.Key] = j.Status
	}
	return m
}

func complete(t *testing.T, r *Run, key string, status Status, outputs map[string]string) Changes {
	t.Helper()
	c, err := r.Complete(key, status, outputs)
	require.NoError(t, err)
	return c
}

func keys(jobs []*Job) []string {
	out := make([]string, len(jobs))
	for i, j := range jobs {
		out[i] = j.Key
	}
	return out
}

func TestNeedsAndStatusFunctions(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  a:
    runs-on: x
    steps: []
  b:
    needs: a
    runs-on: x
    steps: []
  c:
    needs: b
    runs-on: x
    steps: []
  report:
    needs: c
    if: failure()
    runs-on: x
    steps: []
  cleanup:
    needs: c
    if: always()
    runs-on: x
    steps: []
  deploy:
    needs: c
    if: ${{ !failure() && !cancelled() }}
    runs-on: x
    steps: []
  unrelated:
    if: github.ref == 'refs/heads/main'
    runs-on: x
    steps: []
`, Options{GitHub: githubContext("push", "refs/heads/dev")}, nil)

	assert.Equal(t, map[string]Status{
		"a": StatusQueued, "b": StatusWaiting, "c": StatusWaiting, "report": StatusWaiting,
		"cleanup": StatusWaiting, "deploy": StatusWaiting, "unrelated": StatusSkipped,
	}, statuses(r))

	changes := complete(t, r, "a", StatusFailure, nil)
	// A failure skips the chain, and failure() sees the failed ancestor
	// through the skipped jobs.
	assert.Equal(t, map[string]Status{
		"a": StatusFailure, "b": StatusSkipped, "c": StatusSkipped, "report": StatusQueued,
		"cleanup": StatusQueued, "deploy": StatusSkipped, "unrelated": StatusSkipped,
	}, statuses(r))
	assert.ElementsMatch(t, []string{"a", "b", "c", "report", "cleanup", "deploy"}, keys(changes.Jobs))
	assert.False(t, r.Done())

	complete(t, r, "report", StatusSuccess, nil)
	complete(t, r, "cleanup", StatusSuccess, nil)
	assert.True(t, r.Done())
	assert.Equal(t, StatusFailure, r.Result())

	_, err := r.Complete("b", StatusSuccess, nil)
	assert.Error(t, err)
}

func TestSkippedJobSkipsDependents(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  test:
    if: github.event_name == 'workflow_dispatch'
    runs-on: x
    steps: []
  deploy:
    needs: test
    runs-on: x
    steps: []
  deploy-anyway:
    needs: test
    if: ${{ !failure() && !cancelled() }}
    runs-on: x
    steps: []
  smoke:
    needs: deploy-anyway
    runs-on: x
    steps: []
`, Options{GitHub: githubContext("push", "refs/heads/main")}, nil)

	// A skipped job counts as done for the jobs that need it, but the implicit
	// success() is false for every job below it.
	assert.Equal(t, map[string]Status{
		"test": StatusSkipped, "deploy": StatusSkipped, "deploy-anyway": StatusQueued, "smoke": StatusWaiting,
	}, statuses(r))
	complete(t, r, "deploy-anyway", StatusSuccess, nil)
	assert.Equal(t, StatusSkipped, r.Job("smoke").Status)
	assert.Equal(t, StatusSuccess, r.Result())
}

func githubContext(event, ref string) *expr.Object {
	github := expr.NewObject()
	github.Set("event_name", event)
	github.Set("ref", ref)
	github.Set("event", expr.NewObject())
	return github
}

func TestLazyMatrix(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  setup:
    runs-on: x
    steps: []
  build:
    runs-on: ${{ matrix.os }}
    strategy:
      matrix:
        os: [linux, windows]
        go: ['1.27']
    steps: []
  test:
    needs: setup
    name: test ${{ matrix.pkg }}
    runs-on: ${{ needs.setup.outputs.runner }}
    strategy:
      matrix:
        pkg: ${{ fromJSON(needs.setup.outputs.packages) }}
    steps: []
`, Options{}, nil)

	// A static matrix expands at once, and one that reads needs waits.
	assert.Equal(t, []string{"build[0]", "build[1]", "setup"}, sortedKeys(r))
	assert.Equal(t, "build (linux, 1.27)", r.Job("build[0]").Name)
	assert.Equal(t, []string{"windows"}, r.Job("build[1]").RunsOn)

	changes := complete(t, r, "setup", StatusSuccess, map[string]string{
		"packages": `["api", "web", "cli"]`,
		"runner":   "big",
	})
	assert.Equal(t, []string{"test[0]", "test[1]", "test[2]"}, keys(changes.Jobs)[1:])
	test := r.Job("test[2]")
	assert.Equal(t, StatusQueued, test.Status)
	assert.Equal(t, "test cli", test.Name, "a name that uses the matrix gets no suffix")
	assert.Equal(t, []string{"big"}, test.RunsOn)
	strategy, _ := test.Contexts.Get("strategy")
	jobIndex, _ := strategy.(*expr.Object).Get("job-index")
	assert.Equal(t, 2.0, jobIndex)
}

func TestLazyMatrixError(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  setup:
    runs-on: x
    steps: []
  test:
    needs: setup
    runs-on: x
    strategy:
      matrix:
        pkg: ${{ fromJSON(needs.setup.outputs.packages) }}
    steps: []
`, Options{}, nil)
	complete(t, r, "setup", StatusSuccess, map[string]string{"packages": "[]"})
	test := r.Job("test")
	require.NotNil(t, test)
	assert.Equal(t, StatusFailure, test.Status)
	assert.Contains(t, test.Error, "Error when evaluating 'strategy' for job 'test'. .github/workflows/ci.yml (Line: 12, Col: 14): Matrix vector 'pkg' does not contain any values")
	assert.Equal(t, StatusFailure, r.Result())
}

func sortedKeys(r *Run) []string {
	out := keys(r.Jobs)
	for i := range out {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func TestFailFast(t *testing.T) {
	const src = `
on: push
jobs:
  test:
    runs-on: x
    continue-on-error: ${{ matrix.experimental }}
    strategy:
      fail-fast: %s
      matrix:
        v: [1, 2, 3]
        experimental: [false]
        include:
          - v: 4
            experimental: true
    steps: []
  after:
    needs: test
    runs-on: x
    steps: []
`
	t.Run("cancels the other matrix jobs", func(t *testing.T) {
		r := plan(t, fmt.Sprintf(src, "true"), Options{}, nil)
		changes := complete(t, r, "test[1]", StatusFailure, nil)
		assert.Equal(t, []string{"test[0]", "test[2]", "test[3]"}, keys(changes.Cancel))
		assert.True(t, r.Job("test[0]").CancelRequested)
		complete(t, r, "test[0]", StatusCancelled, nil)
		complete(t, r, "test[2]", StatusSuccess, nil)
		complete(t, r, "test[3]", StatusCancelled, nil)
		assert.Equal(t, StatusSkipped, r.Job("after").Status)
		assert.Equal(t, StatusFailure, r.Result())
	})

	t.Run("continue-on-error does not fail the job", func(t *testing.T) {
		r := plan(t, fmt.Sprintf(src, "true"), Options{}, nil)
		changes := complete(t, r, "test[3]", StatusFailure, nil)
		assert.Empty(t, changes.Cancel)
		for _, key := range []string{"test[0]", "test[1]", "test[2]"} {
			complete(t, r, key, StatusSuccess, nil)
		}
		assert.Equal(t, StatusQueued, r.Job("after").Status)
	})

	t.Run("disabled", func(t *testing.T) {
		r := plan(t, fmt.Sprintf(src, "false"), Options{}, nil)
		changes := complete(t, r, "test[0]", StatusFailure, nil)
		assert.Empty(t, changes.Cancel)
	})
}

// As github.com/orgs/community/discussions/189172 reproduces, a job skipped
// inside a called workflow makes success() false for the jobs that need the
// calling job, although the call succeeds.
func TestSkippedJobInCalledWorkflow(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  pipeline:
    uses: ./.github/workflows/reusable.yml
    with:
      run_tests: false
  deploy:
    needs: pipeline
    runs-on: x
    steps: []
  deploy-anyway:
    needs: pipeline
    if: ${{ !cancelled() && !failure() }}
    runs-on: x
    steps: []
`, Options{}, files{".github/workflows/reusable.yml": `
on:
  workflow_call:
    inputs:
      run_tests:
        type: boolean
jobs:
  build:
    runs-on: x
    steps: []
  optional-test:
    if: inputs.run_tests == true
    needs: build
    runs-on: x
    steps: []
`})
	complete(t, r, "pipeline/build", StatusSuccess, nil)
	assert.Equal(t, map[string]Status{
		"pipeline/build": StatusSuccess, "pipeline/optional-test": StatusSkipped,
		"deploy": StatusSkipped, "deploy-anyway": StatusQueued,
	}, statuses(r))
}

func TestCancel(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  a:
    runs-on: x
    steps: []
  b:
    needs: a
    runs-on: x
    steps: []
  notify:
    needs: a
    if: always()
    runs-on: x
    steps: []
  report:
    needs: a
    if: ${{ !cancelled() }}
    runs-on: x
    steps: []
  notify-each:
    needs: a
    if: always()
    strategy:
      matrix:
        to: [chat, mail]
    runs-on: x
    steps: []
`, Options{}, nil)
	changes := r.Cancel()
	assert.Equal(t, []string{"a"}, keys(changes.Cancel))
	assert.Equal(t, StatusWaiting, r.Job("notify-each[0]").Status,
		"a job whose if is not evaluated yet is not cancelled, because always() runs it")
	assert.Empty(t, r.Cancel().Cancel, "cancelling twice changes nothing")

	complete(t, r, "a", StatusCancelled, nil)
	assert.Equal(t, map[string]Status{
		"a": StatusCancelled, "b": StatusCancelled, "notify": StatusQueued, "report": StatusCancelled,
		"notify-each[0]": StatusQueued, "notify-each[1]": StatusQueued,
	}, statuses(r))
}

const deployWorkflow = `
on:
  workflow_call:
    inputs:
      environment:
        type: string
        required: true
      version:
        type: string
        required: true
      dry-run:
        type: boolean
        default: false
    secrets:
      token:
        required: true
    outputs:
      url:
        value: ${{ jobs.release.outputs.url }}
jobs:
  check:
    runs-on: ubuntu-latest
    steps: []
  release:
    needs: check
    name: release ${{ inputs.environment }}
    runs-on: ${{ inputs.environment }}-runner
    permissions:
      contents: read
    steps: []
`

const callerWorkflow = `
on: push
jobs:
  build:
    runs-on: x
    steps: []
  deploy:
    needs: build
    permissions:
      contents: read
    strategy:
      matrix:
        env: [staging, prod]
    uses: ./.github/workflows/deploy.yml
    with:
      environment: ${{ matrix.env }}
      version: ${{ needs.build.outputs.version }}
    secrets:
      token: ${{ secrets.DEPLOY_TOKEN }}
  notify:
    needs: deploy
    runs-on: x
    steps: []
`

func TestReusableWorkflow(t *testing.T) {
	loader := files{".github/workflows/deploy.yml": deployWorkflow}
	r := plan(t, callerWorkflow, Options{}, loader)
	assert.Equal(t, []string{"build", "notify"}, sortedKeys(r))

	complete(t, r, "build", StatusSuccess, map[string]string{"version": "1.2"})
	assert.Equal(t, map[string]Status{
		"build": StatusSuccess, "deploy[0]/check": StatusQueued, "deploy[0]/release": StatusWaiting,
		"deploy[1]/check": StatusQueued, "deploy[1]/release": StatusWaiting, "notify": StatusWaiting,
	}, statuses(r))
	assert.Equal(t, "deploy (staging) / check", r.Job("deploy[0]/check").Name)

	complete(t, r, "deploy[0]/check", StatusSuccess, nil)
	release := r.Job("deploy[0]/release")
	assert.Equal(t, StatusQueued, release.Status)
	assert.Equal(t, "deploy (staging) / release staging", release.Name)
	assert.Equal(t, []string{"staging-runner"}, release.RunsOn)
	assert.Equal(t, "org/app/.github/workflows/deploy.yml@abc", release.Workflow)
	inputs, _ := release.Contexts.Get("inputs")
	want, err := expr.FromJSON(`{"environment": "staging", "version": "1.2", "dry-run": false}`)
	require.NoError(t, err)
	assert.Equal(t, want, inputs)

	secrets, err := release.Secrets(map[string]string{"DEPLOY_TOKEN": "t0k3n", "OTHER": "x"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"token": "t0k3n"}, secrets)

	complete(t, r, "deploy[1]/check", StatusSuccess, nil)
	complete(t, r, "deploy[1]/release", StatusSuccess, map[string]string{"url": "https://prod"})
	assert.Equal(t, StatusWaiting, r.Job("notify").Status)
	complete(t, r, "deploy[0]/release", StatusSuccess, map[string]string{"url": "https://staging"})

	notify := r.Job("notify")
	assert.Equal(t, StatusQueued, notify.Status)
	needs, _ := notify.Contexts.Get("needs")
	want, err = expr.FromJSON(`{"deploy": {"result": "success", "outputs": {"url": "https://staging"}}}`)
	require.NoError(t, err)
	assert.Equal(t, want, needs, "outputs of matrix jobs are merged in completion order")
}

func TestReplayIsDeterministic(t *testing.T) {
	loader := files{".github/workflows/deploy.yml": deployWorkflow}
	steps := []struct {
		key     string
		status  Status
		outputs map[string]string
	}{
		{"build", StatusSuccess, map[string]string{"version": "1"}},
		{"deploy[1]/check", StatusSuccess, nil},
		{"deploy[0]/check", StatusFailure, nil},
		{"deploy[1]/release", StatusCancelled, nil},
	}
	snapshot := func() []string {
		r := plan(t, callerWorkflow, Options{}, loader)
		for _, s := range steps {
			complete(t, r, s.key, s.status, s.outputs)
		}
		var out []string
		for _, j := range r.Jobs {
			out = append(out, fmt.Sprintf("%s %s %q %v", j.Key, j.Status, j.Name, j.CancelRequested))
		}
		return out
	}
	first := snapshot()
	for range 5 {
		assert.Equal(t, first, snapshot())
	}
}

func TestReusableWorkflowErrors(t *testing.T) {
	t.Run("load errors fail the plan", func(t *testing.T) {
		_, err := tryPlan("on: push\njobs:\n  a:\n    uses: ./.github/workflows/x.yml\n", Options{}, files{
			".github/workflows/x.yml": "on:\n  workflow_call:\n    inputs:\n      v:\n        type: string\n        required: true\njobs:\n  b:\n    runs-on: x\n    steps: []\n",
		})
		assert.ErrorContains(t, err, "Input v is required, but not provided while calling.")
	})

	t.Run("input of the wrong type", func(t *testing.T) {
		r := plan(t, `
on: push
jobs:
  a:
    uses: ./.github/workflows/x.yml
    with:
      dry-run: ${{ github.event_name }}
`, Options{GitHub: githubContext("push", "refs/heads/main")}, files{
			".github/workflows/x.yml": "on:\n  workflow_call:\n    inputs:\n      dry-run:\n        type: boolean\njobs:\n  b:\n    runs-on: x\n    steps: []\n",
		})
		a := r.Job("a")
		require.NotNil(t, a)
		assert.Equal(t, StatusFailure, a.Status)
		assert.Equal(t, "The template is not valid. .github/workflows/ci.yml (Line: 7, Col: 16): Unexpected value 'push'", a.Error)
		assert.Equal(t, StatusFailure, r.Result())
	})
}

func TestReuse(t *testing.T) {
	const src = `
on: push
jobs:
  a:
    runs-on: x
    steps: []
  b:
    needs: a
    runs-on: ${{ needs.a.outputs.runner }}
    steps: []
`
	r := plan(t, src, Options{
		Attempt: 2,
		Reuse:   map[string]Completion{"a": {Status: StatusSuccess, Outputs: map[string]string{"runner": "kept"}}},
	}, nil)
	a := r.Job("a")
	assert.Equal(t, StatusSuccess, a.Status)
	assert.True(t, a.Reused)
	assert.Equal(t, StatusQueued, r.Job("b").Status)
	assert.Equal(t, []string{"kept"}, r.Job("b").RunsOn)
}

func TestPlanRunSettings(t *testing.T) {
	const src = `
run-name: Deploy ${{ inputs.target }} (${{ inputs.force }})
on:
  workflow_dispatch:
    inputs:
      target:
        type: choice
        options: [staging, prod]
        required: true
      force:
        type: boolean
      retries:
        type: number
        default: 3
jobs:
  a:
    runs-on: x
    steps: []
`
	r := plan(t, src, Options{Inputs: map[string]any{"target": "prod", "force": "true"}}, nil)
	assert.Equal(t, "Deploy prod (true)", r.Name)
	inputs, _ := r.Job("a").Contexts.Get("inputs")
	retries, _ := inputs.(*expr.Object).Get("retries")
	assert.Equal(t, 3.0, retries)

	_, err := tryPlan(src, Options{Inputs: map[string]any{"target": "dev"}}, nil)
	assert.ErrorContains(t, err, `input "target" must be one of staging, prod, not "dev"`)
	_, err = tryPlan(src, Options{Inputs: map[string]any{"target": "prod", "force": "maybe"}}, nil)
	assert.ErrorContains(t, err, `input "force" must be a boolean`)

	github := githubContext("pull_request", "refs/pull/7/merge")
	event := expr.NewObject()
	event.Set("number", 7.0)
	github.Set("event", event)
	r = plan(t, "on: pull_request\njobs:\n  a:\n    runs-on: x\n    steps: []\n", Options{GitHub: github, CancelSupersededPullRequestRuns: true}, nil)
	assert.Equal(t, &Concurrency{Group: ".github/workflows/ci.yml/pull/7", CancelInProgress: true}, r.Concurrency)

	r = plan(t, "on: pull_request\nconcurrency: ci-${{ github.ref }}\njobs:\n  a:\n    runs-on: x\n    steps: []\n", Options{GitHub: github, CancelSupersededPullRequestRuns: true}, nil)
	assert.Equal(t, &Concurrency{Group: "ci-refs/pull/7/merge"}, r.Concurrency)

	r = plan(t, "on: push\njobs:\n  a:\n    runs-on: x\n    steps: []\n", Options{}, nil)
	assert.Equal(t, model.Permissions{"contents": model.LevelRead, "packages": model.LevelRead}, r.Job("a").Permissions)
	assert.Equal(t, 360, r.Job("a").TimeoutMinutes)
}

func TestJobSettings(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  deploy:
    runs-on:
      group: org/deployers
      labels: [linux, '${{ matrix.arch }}']
    strategy:
      matrix:
        arch: [x64]
    timeout-minutes: ${{ matrix.arch == 'x64' && 30 || 60 }}
    cancel-timeout-minutes: 2
    continue-on-error: ${{ github.ref != 'refs/heads/main' }}
    concurrency:
      group: deploy-${{ matrix.arch }}
      queue: max
    environment:
      name: prod-${{ matrix.arch }}
      url: ${{ steps.deploy.outputs.url }}
      deployment: false
    steps:
      - id: deploy
        run: ./deploy
  empty:
    runs-on: ${{ github.event_name == 'never' && 'x' || '' }}
    steps:
      - run: true
`, Options{GitHub: githubContext("push", "refs/heads/dev")}, nil)

	j := r.Job("deploy[0]")
	require.NotNil(t, j)
	assert.Equal(t, StatusQueued, j.Status, j.Error)
	assert.Equal(t, "deploy (x64)", j.Name)
	assert.Equal(t, []string{"linux", "x64"}, j.RunsOn)
	assert.Equal(t, "org/deployers", j.RunnerGroup)
	assert.Equal(t, 30, j.TimeoutMinutes)
	assert.Equal(t, 2, j.CancelTimeoutMinutes)
	assert.True(t, j.ContinueOnError)
	assert.Equal(t, &Concurrency{Group: "deploy-x64", Queue: "max"}, j.Concurrency)
	require.NotNil(t, j.Environment)
	assert.Equal(t, "prod-x64", j.Environment.Name)
	assert.False(t, j.Environment.Deployment)
	require.NotNil(t, j.Environment.URL)
	assert.Equal(t, "steps.deploy.outputs.url", j.Environment.URL.Expr, "the agent evaluates the URL")

	empty := r.Job("empty")
	assert.Equal(t, StatusFailure, empty.Status)
	assert.Equal(t, "Error when evaluating 'runs-on' for job 'empty'. .github/workflows/ci.yml (Line: 25, Col: 14): Unexpected value ''", empty.Error)
}

func TestCallSettings(t *testing.T) {
	r := plan(t, `
on: push
jobs:
  call:
    concurrency: caller
    uses: ./.github/workflows/x.yml
    with:
      count: ${{ fromJSON('2') }}
    secrets:
      token: ${{ secrets.TOKEN }}
      missing: ${{ secrets.MISSING }}
`, Options{}, files{".github/workflows/x.yml": `
on:
  workflow_call:
    inputs:
      count:
        type: number
      label:
        type: string
        default: ${{ github.event_name }}
    secrets:
      token:
      missing:
concurrency: callee-${{ inputs.count }}
jobs:
  b:
    runs-on: x
    steps:
      - run: true
`})
	b := r.Job("call/b")
	require.NotNil(t, b)
	assert.Equal(t, StatusQueued, b.Status, b.Error)
	assert.Equal(t, []CallConcurrency{{"call/", &Concurrency{Group: "caller"}}, {"call/", &Concurrency{Group: "callee-2"}}}, b.CallConcurrency)
	inputs, _ := b.Contexts.Get("inputs")
	want, err := expr.FromJSON(`{"count": 2, "label": ""}`)
	require.NoError(t, err)
	assert.Equal(t, want, inputs)

	secrets, err := b.Secrets(map[string]string{"TOKEN": "t"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"token": "t"}, secrets, "an empty secret is not passed")
}
