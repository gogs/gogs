package planner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/model"
)

// repoFiles serves the workflow files of a repository from a directory.
type repoFiles struct{ repo, dir string }

func (f repoFiles) ReadFile(name string) (string, error) {
	path, _ := strings.CutPrefix(name, f.repo+"/")
	path, _, _ = strings.Cut(path, "@")
	b, err := os.ReadFile(filepath.Join(f.dir, path))
	return string(b), err
}

func (f repoFiles) ResolveRef(_, ref string) (string, error) {
	return f.repo + "/" + ref[len("./"):] + "@main", nil
}

// planReal plans a workflow of the real repositories in testdata/real, with
// the github context and the workflow_dispatch inputs given.
func planReal(t *testing.T, repo, dir, path, github string, inputs map[string]any) *Run {
	t.Helper()
	w, err := model.Load(repoFiles{repo, dir}, repo+"/"+path+"@main", model.DefaultOptions())
	require.NoError(t, err)
	require.Empty(t, w.Errors)
	ctx, err := expr.FromJSON(github)
	require.NoError(t, err)
	r, err := Plan(Options{Workflow: w, GitHub: ctx.(*expr.Object), Inputs: inputs})
	require.NoError(t, err)
	return r
}

// runQueued completes the queued jobs with the status result returns and
// the outputs of their key, or as cancelled when the planner asked to stop
// them, until none is queued. It returns the keys of the jobs of each wave.
func runQueued(t *testing.T, r *Run, result func(j *Job) Status, outputs map[string]map[string]string) [][]string {
	t.Helper()
	var waves [][]string
	for {
		var queued []*Job
		for _, j := range r.Jobs {
			if j.Status == StatusQueued {
				queued = append(queued, j)
			}
		}
		if len(queued) == 0 {
			return waves
		}
		waves = append(waves, keys(queued))
		for _, j := range queued {
			if j.Status != StatusQueued {
				continue
			}
			status := result(j)
			if j.CancelRequested {
				status = StatusCancelled
			}
			complete(t, r, j.Key, status, outputs[j.Key])
		}
	}
}

// stages returns the stage of each node of the graph, the length of the
// longest chain of needs that ends at it, as GitHub lays out the columns of
// the graph of a run.
func stages(g *Graph) map[string]int {
	m := map[string]int{}
	var stage func(n *GraphNode) int
	stage = func(n *GraphNode) int {
		if s, ok := m[n.Key]; ok {
			return s
		}
		s := 1
		for _, dep := range n.Needs {
			s = max(s, stage(graphNode(g, dep))+1)
		}
		m[n.Key] = s
		return s
	}
	for _, n := range g.Nodes {
		stage(n)
	}
	return m
}

func nodeStatuses(g *Graph) map[string]Status {
	m := map[string]Status{}
	for _, n := range g.Nodes {
		m[n.Key] = n.Status
	}
	return m
}

func graphNode(g *Graph, key string) *GraphNode {
	for _, n := range g.Nodes {
		if n.Key == key {
			return n
		}
	}
	return nil
}

func TestGraphCPython(t *testing.T) {
	r := planReal(t, "python/cpython", "testdata/real/cpython", ".github/workflows/build.yml",
		`{"event_name": "pull_request", "ref": "refs/pull/1/merge", "repository": "python/cpython", "base_ref": "main", "event": {"number": 1}}`, nil)

	g := r.Graph()
	var keys []string
	for _, n := range g.Nodes {
		keys = append(keys, n.Key)
	}
	assert.Equal(t, []string{
		"build-context", "check-docs", "check-autoconf-regen", "check-generated-files", "check-c-api-docs",
		"build-windows", "build-macos", "build-ubuntu", "build-ubuntu-installed", "build-ubuntu-ssltests",
		"build-android", "build-ios", "build-emscripten", "build-wasi", "test-hypothesis",
		"test-lazy-imports-all", "build-san", "cross-build-linux", "cifuzz", "all-required-green",
	}, keys, "every job of the file is in the graph, in file order")
	assert.Equal(t, StatusQueued, g.Status)

	context := graphNode(g, "build-context")
	assert.Equal(t, StatusQueued, context.Status)
	assert.True(t, context.Reusable)
	require.Len(t, context.Calls, 1)
	assert.Equal(t, "Change detection", context.Calls[0].Name)
	assert.Equal(t, "build-context/compute-changes", context.Calls[0].Nodes[0].Jobs[0].Key)
	assert.Equal(t, StatusQueued, context.Calls[0].Nodes[0].Status)

	windows := graphNode(g, "build-windows")
	assert.Equal(t, StatusWaiting, windows.Status)
	assert.True(t, windows.Matrix && windows.Reusable)
	assert.Equal(t, []string{"build-context"}, windows.Needs)
	assert.Empty(t, windows.Calls, "a call waits for its needs before it is expanded")
	assert.Len(t, graphNode(g, "build-ubuntu-ssltests").Jobs, 6, "a matrix that reads no needs is expanded at once")
	assert.Len(t, graphNode(g, "all-required-green").Needs, 18)

	complete(t, r, "build-context/compute-changes", StatusSuccess, map[string]string{
		"run-android": "false", "run-ci-fuzz": "false", "run-ci-fuzz-stdlib": "false", "run-docs": "true",
		"run-ios": "false", "run-macos": "true", "run-tests": "true", "run-ubuntu": "true",
		"run-emscripten": "false", "run-wasi": "false", "run-windows-tests": "true",
	})
	g = r.Graph()
	statuses := nodeStatuses(g)
	for _, key := range []string{"build-android", "build-ios", "build-emscripten", "build-wasi", "cifuzz"} {
		assert.Equal(t, StatusSkipped, statuses[key], key)
	}
	assert.Equal(t, StatusWaiting, statuses["all-required-green"])
	ubuntu := graphNode(g, "build-ubuntu")
	assert.Equal(t, StatusQueued, ubuntu.Status)
	require.Len(t, ubuntu.Calls, 5)
	assert.Equal(t, "Ubuntu (free-threading) ", ubuntu.Calls[2].Name)
	assert.Equal(t, "build and test (ubuntu-26.04)", ubuntu.Calls[2].Nodes[0].Jobs[0].Name,
		"names inside a call leave out the caller's name")

	runQueued(t, r, func(j *Job) Status {
		if strings.HasPrefix(j.Key, "build-ubuntu[") && strings.Contains(j.Name, "free-threading") {
			return StatusFailure
		}
		return StatusSuccess
	}, nil)
	g = r.Graph()
	assert.Equal(t, StatusFailure, g.Status)
	ubuntu = graphNode(g, "build-ubuntu")
	assert.Equal(t, StatusFailure, ubuntu.Status)
	var calls []Status
	for _, c := range ubuntu.Calls {
		calls = append(calls, c.Status)
	}
	assert.Equal(t, []Status{StatusSuccess, StatusSuccess, StatusFailure, StatusFailure, StatusSuccess}, calls,
		"fail-fast is off, so the other calls finish")
	assert.Equal(t, StatusSuccess, graphNode(g, "all-required-green").Status, "if: always() runs after a failure")

	data, err := json.Marshal(g)
	require.NoError(t, err)
	assert.Contains(t, string(data), `{"key":"check-docs","jobId":"check-docs","name":"Docs","needs":["build-context"],"status":"success","reusable":true,"calls":[`)
}

func TestGraphRust(t *testing.T) {
	r := planReal(t, "rust-lang/rust", "testdata/real/rust", ".github/workflows/ci.yml",
		`{"event_name": "push", "ref": "refs/heads/automation/bors/auto", "sha": "abc", "repository": "rust-lang/rust", "workflow": "CI", "event": {}}`, nil)

	g := r.Graph()
	job := graphNode(g, "job")
	assert.Equal(t, StatusWaiting, job.Status)
	assert.True(t, job.Matrix)
	assert.Empty(t, job.Jobs, "the matrix comes from the outputs of calculate_matrix")
	assert.Equal(t, []string{"calculate_matrix", "job"}, graphNode(g, "outcome").Needs)

	complete(t, r, "calculate_matrix", StatusSuccess, map[string]string{
		"run_type": "auto",
		"jobs": `[{"name": "x86_64-gnu", "full_name": "auto - x86_64-gnu", "os": "ubuntu-24.04"},
			{"name": "aarch64-gnu", "full_name": "auto - aarch64-gnu", "os": "ubuntu-24.04-arm"},
			{"name": "x86_64-msvc", "full_name": "auto - x86_64-msvc", "os": "windows-2025"}]`,
	})
	job = graphNode(r.Graph(), "job")
	assert.Equal(t, StatusQueued, job.Status)
	var names []string
	for _, j := range job.Jobs {
		names = append(names, j.Name)
	}
	assert.Equal(t, []string{"auto - x86_64-gnu", "auto - aarch64-gnu", "auto - x86_64-msvc"}, names)

	complete(t, r, "job[2]", StatusFailure, nil)
	job = graphNode(r.Graph(), "job")
	assert.True(t, job.Jobs[0].CancelRequested, "fail-fast is true for auto builds")
	assert.True(t, job.Jobs[1].CancelRequested)

	runQueued(t, r, func(*Job) Status { return StatusSuccess }, nil)
	g = r.Graph()
	assert.Equal(t, StatusFailure, g.Status)
	assert.Equal(t, map[string]Status{
		"calculate_matrix": StatusSuccess, "job": StatusFailure, "outcome": StatusSkipped,
	}, nodeStatuses(g))
}

const uvRelease = `{"event_name": "workflow_dispatch", "ref": "refs/heads/main", "repository": "astral-sh/uv",
	"repository_owner": "astral-sh", "workflow_ref": "astral-sh/uv/.github/workflows/release.yml@refs/heads/main",
	"event": {"inputs": {"dry-run": false}}}`

// uvOutputs are the outputs of the jobs of the uv release whose outputs
// decide the jobs after them.
var uvOutputs = map[string]map[string]string{
	"plan": {
		"val":        `{"announcement_tag": "0.9.0", "announcement_tag_is_implicit": false, "announcement_is_prerelease": false}`,
		"publishing": "true",
	},
	"build-docker/docker-plan": {
		"login": "true", "push": "true", "save": "true", "tag": "0.9.0",
		"extra-images": `{"include": [{"image-mapping": "alpine:3.22,alpine3.22"},
			{"image-mapping": "debian:trixie-slim,trixie-slim"}, {"image-mapping": "python:3.13-bookworm,python3.13-bookworm"}]}`,
	},
	"assemble-release-artifacts/prepare": {
		"macos": `[{"target": "x86_64-apple-darwin", "runner": "macos-15-intel"}, {"target": "aarch64-apple-darwin", "runner": "macos-15"}]`,
		"windows": `[{"target": "x86_64-pc-windows-msvc", "runner": "windows-2025"},
			{"target": "i686-pc-windows-msvc", "runner": "windows-2025"}, {"target": "aarch64-pc-windows-msvc", "runner": "windows-11-arm"}]`,
	},
	"publish-docker/plan": {
		"base-image":   `{"name": "uv"}`,
		"extra-images": `{"include": [{"name": "alpine3.22"}, {"name": "trixie-slim"}, {"name": "python3.13-bookworm"}]}`,
	},
}

func TestGraphUVRelease(t *testing.T) {
	plan := func(t *testing.T) *Run {
		return planReal(t, "astral-sh/uv", "testdata/real/uv", ".github/workflows/release.yml", uvRelease,
			map[string]any{"dry-run": false})
	}

	t.Run("success", func(t *testing.T) {
		r := plan(t)
		g := r.Graph()
		assert.Equal(t, map[string]int{
			"release-gate": 1, "plan": 1, "build-release-artifacts": 2, "build-docker": 2,
			"assemble-release-artifacts": 3, "generate-local-dist-manifest": 4, "build-global-dist-artifacts": 5,
			"finalize-dist-manifest": 6, "publish-docker": 7, "publish-pypi": 7, "publish-crates": 8,
			"publish-github": 8, "publish-docs": 9, "publish-versions": 9, "publish-mirror": 9,
		}, stages(g))
		assert.Len(t, graphNode(g, "publish-github").Needs, 6)
		assert.Empty(t, graphNode(g, "publish-docker").Calls, "a call waits for its needs before it is expanded")

		waves := runQueued(t, r, func(*Job) Status { return StatusSuccess }, uvOutputs)
		wave := map[string]int{}
		for i, keys := range waves {
			for _, key := range keys {
				wave[key] = i + 1
			}
		}
		// The longest chain of the run goes through two levels of called
		// workflows, and each job of it waits for the one before.
		chain := []string{
			"plan",
			"build-release-artifacts/sdist",
			"build-release-artifacts/check-wheels",
			"assemble-release-artifacts/prepare",
			"assemble-release-artifacts/sign-macos",
			"assemble-release-artifacts/assemble",
			"assemble-release-artifacts/verify-macos[0]",
			"generate-local-dist-manifest",
			"build-global-dist-artifacts",
			"finalize-dist-manifest",
			"publish-docker/plan",
			"publish-docker/publish-extra[2]/publish",
			"publish-docker/publish-base/publish",
			"publish-github",
			"publish-docs/mkdocs",
		}
		for i, key := range chain {
			assert.Equal(t, i+1, wave[key], key)
		}
		assert.Len(t, waves, len(chain))

		g = r.Graph()
		assert.Equal(t, StatusSuccess, g.Status)
		for key, status := range nodeStatuses(g) {
			assert.Equal(t, StatusSuccess, status, key)
		}

		docker := graphNode(g, "publish-docker")
		require.Len(t, docker.Calls, 1)
		assert.Equal(t, map[string]int{
			"publish-docker/plan": 1, "publish-docker/publish-extra": 2, "publish-docker/publish-base": 3,
		}, stages(docker.Calls[0]))
		extra := graphNode(docker.Calls[0], "publish-docker/publish-extra")
		assert.True(t, extra.Matrix && extra.Reusable)
		var names []string
		for _, c := range extra.Calls {
			names = append(names, c.Name+": "+c.Nodes[0].Jobs[0].Name)
		}
		assert.Equal(t, []string{
			"publish alpine3.22: publish alpine3.22",
			"publish trixie-slim: publish trixie-slim",
			"publish python3.13-bookworm: publish python3.13-bookworm",
		}, names, "the matrix of a call inside a call comes from the outputs of a job of the outer call")
		base := graphNode(docker.Calls[0], "publish-docker/publish-base")
		require.Len(t, base.Calls, 1)
		assert.Equal(t, "publish-docker/publish-base/publish", base.Calls[0].Nodes[0].Jobs[0].Key)

		sign := graphNode(g, "assemble-release-artifacts").Calls[0]
		assert.Len(t, graphNode(sign, "assemble-release-artifacts/verify-macos").Jobs, 2)
		assert.Len(t, graphNode(sign, "assemble-release-artifacts/verify-windows").Jobs, 3)
		assert.Len(t, graphNode(graphNode(g, "build-docker").Calls[0], "build-docker/docker-build-extra").Jobs, 3)
	})

	t.Run("docker build fails", func(t *testing.T) {
		r := plan(t)
		runQueued(t, r, func(j *Job) Status {
			if j.Key == "build-docker/docker-build-extra[1]" {
				return StatusFailure
			}
			return StatusSuccess
		}, uvOutputs)
		g := r.Graph()
		assert.Equal(t, StatusFailure, g.Status)
		assert.Equal(t, map[string]Status{
			"release-gate": StatusSuccess, "plan": StatusSuccess, "build-release-artifacts": StatusSuccess,
			"build-docker": StatusFailure, "assemble-release-artifacts": StatusSuccess,
			"generate-local-dist-manifest": StatusSuccess, "build-global-dist-artifacts": StatusSuccess,
			// The if of finalize-dist-manifest runs with always(), and is
			// false because build-docker failed.
			"finalize-dist-manifest": StatusSkipped, "publish-docker": StatusSkipped,
			"publish-pypi": StatusSkipped, "publish-crates": StatusSkipped, "publish-github": StatusSkipped,
			"publish-docs": StatusSkipped, "publish-versions": StatusSkipped, "publish-mirror": StatusSkipped,
		}, nodeStatuses(g))
		docker := graphNode(g, "build-docker").Calls[0]
		assert.Equal(t, map[string]Status{
			"build-docker/docker-plan": StatusSuccess, "build-docker/docker-build-base": StatusSuccess,
			"build-docker/docker-build-extra": StatusFailure, "build-docker/docker-manifest": StatusSkipped,
		}, nodeStatuses(docker))
	})
}

func TestGraphHomeAssistant(t *testing.T) {
	r := planReal(t, "home-assistant/core", "testdata/real/home-assistant", ".github/workflows/ci.yaml",
		`{"event_name": "push", "ref": "refs/heads/dev", "repository": "home-assistant/core", "event": {}}`, nil)
	g := r.Graph()
	assert.Equal(t, map[string]int{
		"info": 1, "prek": 2, "lint-hadolint": 2, "base": 2, "hassfest": 3, "gen-requirements-all": 3,
		"build-pylint-package": 2, "gen-recorder-db-versions": 2, "dependency-review": 3, "audit-licenses": 3,
		"pylint": 3, "pylint-tests": 3, "mypy": 3, "benchmarks": 3, "build-hassfest-package": 2,
		"build-pytest-package": 2, "prepare-pytest-full": 3, "pytest-full": 4, "pytest-mariadb": 4,
		"pytest-postgres": 4, "pytest-sqlite": 4, "coverage-full": 5, "pytest-partial": 3, "coverage-partial": 4,
		"upload-test-results": 5,
	}, stages(g))
	edges := 0
	for _, n := range g.Nodes {
		edges += len(n.Needs)
	}
	assert.Equal(t, 63, edges)

	runQueued(t, r, func(*Job) Status { return StatusSuccess }, map[string]map[string]string{"info": {
		"core": "[]", "integrations_glob": "", "integrations": "[]", "requirements": "false",
		"python_versions": `["3.14.2"]`, "default_python": "3.14.2", "test_full_suite": "true",
		"test_groups": "[1, 2, 3, 4]", "mariadb_groups": `["mariadb:10.6"]`,
		"postgresql_groups": `["postgres:15", "postgres:17"]`, "sqlite_groups": `["sqlite"]`,
		"lint_only": "false", "skip_coverage": "false",
	}})
	g = r.Graph()
	assert.Equal(t, StatusSuccess, g.Status)
	assert.Len(t, graphNode(g, "pytest-full").Jobs, 4, "one Python version by four groups")
	assert.Len(t, graphNode(g, "pytest-postgres").Jobs, 2)
	assert.Equal(t, StatusSkipped, graphNode(g, "pytest-partial").Status, "the full suite runs instead")
}

func TestGraphNextJS(t *testing.T) {
	r := planReal(t, "vercel/next.js", "testdata/real/next.js", ".github/workflows/build_and_test.yml",
		`{"event_name": "push", "ref": "refs/heads/canary", "ref_name": "canary", "repository": "vercel/next.js",
			"workflow": "build-and-test", "sha": "abc", "event": {}}`, nil)
	g := r.Graph()
	s := stages(g)
	assert.Len(t, s, 41)
	assert.Equal(t, 1, s["optimize-ci"])
	assert.Equal(t, 5, s["tests-pass"])
	assert.Len(t, graphNode(g, "tests-pass").Needs, 31)
	edges, reusable := 0, 0
	for _, n := range g.Nodes {
		edges += len(n.Needs)
		if n.Reusable {
			reusable++
		}
	}
	assert.Equal(t, 148, edges)
	assert.Equal(t, 35, reusable)

	waves := runQueued(t, r, func(*Job) Status { return StatusSuccess }, map[string]map[string]string{
		"optimize-ci/optimize-ci": {"skip": "false"},
		"changes":                 {"docs-only": "false", "is-release": "false", "rspack": "false", "turbopack-only": "false"},
	})
	assert.Len(t, waves, 5, "each stage runs once the one before finishes")
	g = r.Graph()
	assert.Equal(t, StatusSuccess, g.Status)
	for key, status := range nodeStatuses(g) {
		want := StatusSuccess
		if strings.HasPrefix(key, "test-rspack-") {
			want = StatusSkipped
		}
		assert.Equal(t, want, status, key)
	}
}

// The stress workflows in testdata/complex are made up. A chain runs through
// four levels of called workflows, and the matrix of integration comes from an
// output set by one of three calls of a reusable workflow.
const stressGitHub = `{"event_name": "push", "ref": "refs/heads/main", "repository": "org/stress", "event": {}}`

func planStress(t *testing.T, opts Options) *Run {
	t.Helper()
	w, err := model.Load(repoFiles{"org/stress", "testdata/complex"}, "org/stress/.github/workflows/ci.yml@main", model.DefaultOptions())
	require.NoError(t, err)
	require.Empty(t, w.Errors)
	ctx, err := expr.FromJSON(stressGitHub)
	require.NoError(t, err)
	opts.Workflow, opts.GitHub = w, ctx.(*expr.Object)
	r, err := Plan(opts)
	require.NoError(t, err)
	return r
}

func stressOutputs() map[string]map[string]string {
	out := map[string]map[string]string{
		"changes": {"services": `["api", "web", "worker"]`, "docs": "true"},
		// Only the api service reports the shards of the integration tests.
		"build[0]/scan": {"shards": "[1, 2, 3]"},
	}
	for i := range 3 {
		out[fmt.Sprintf("build[%d]/sign/prepare", i)] = map[string]string{"keys": `["k1", "k2"]`}
		out[fmt.Sprintf("build[%d]/sign/notarize/staple/attest", i)] = map[string]string{"id": fmt.Sprint("attestation-", i)}
	}
	return out
}

func failJobs(keys ...string) func(*Job) Status {
	return func(j *Job) Status {
		if slices.Contains(keys, j.Key) {
			return StatusFailure
		}
		return StatusSuccess
	}
}

func jobNames(r *Run, prefix string) []string {
	var names []string
	for _, j := range r.Jobs {
		if strings.HasPrefix(j.Key, prefix) {
			names = append(names, j.Name)
		}
	}
	return names
}

func TestGraphStress(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		r := planStress(t, Options{})
		waves := runQueued(t, r, failJobs(), stressOutputs())
		perService := func(format string) []string {
			return []string{fmt.Sprintf(format, 0), fmt.Sprintf(format, 1), fmt.Sprintf(format, 2)}
		}
		perArch := func(format string) []string {
			var keys []string
			for i := range 3 {
				keys = append(keys, fmt.Sprintf(format, i, 0), fmt.Sprintf(format, i, 1))
			}
			return keys
		}
		assert.Equal(t, [][]string{
			{"changes"},
			{"lint", "unit[0]", "unit[1]", "unit[2]", "unit[3]", "unit[4]", "unit[5]", "docs"},
			append([]string{"publish-docs"}, perService("build[%d]/compile")...),
			perArch("build[%d]/image[%d]"),
			perService("build[%d]/sign/prepare"),
			perArch("build[%d]/sign/sign[%d]"),
			perService("build[%d]/sign/notarize/submit"),
			perService("build[%d]/sign/notarize/staple/attest"),
			perService("build[%d]/scan"),
			{"integration[0]", "integration[1]", "integration[2]"},
			{"e2e"},
			// max-parallel: 1 runs the called workflows one after the other.
			{"deploy[0]/plan"}, {"deploy[0]/apply"}, {"deploy[0]/smoke"},
			{"deploy[1]/plan"}, {"deploy[1]/apply"}, {"deploy[1]/smoke"},
			{"summary"},
		}, waves)
		assert.Equal(t, []string{"integration 1", "integration 2", "integration 3"}, jobNames(r, "integration"),
			"the output of a matrix of calls is the last non-empty value of a call that succeeded")
		assert.Equal(t, []string{"summary (green)"}, jobNames(r, "summary"))
		assert.Equal(t, StatusSuccess, r.Result())
	})

	t.Run("deep failure and re-run", func(t *testing.T) {
		r := planStress(t, Options{})
		runQueued(t, r, failJobs("build[1]/sign/notarize/staple/attest"), stressOutputs())
		g := r.Graph()
		assert.Equal(t, map[string]Status{
			"changes": StatusSuccess, "lint": StatusSuccess, "unit": StatusSuccess, "build": StatusFailure,
			"integration": StatusSkipped, "e2e": StatusSkipped, "docs": StatusSuccess, "publish-docs": StatusSuccess,
			"deploy": StatusSkipped, "notify-failure": StatusSuccess, "summary": StatusSuccess, "cleanup": StatusSkipped,
		}, nodeStatuses(g))
		web := graphNode(g, "build").Calls[1]
		assert.Equal(t, map[string]Status{
			"build[1]/compile": StatusSuccess, "build[1]/image": StatusSuccess, "build[1]/sign": StatusFailure,
			"build[1]/scan": StatusSkipped,
		}, nodeStatuses(web), "the failure four calls down fails each call above it")
		assert.Equal(t, []string{"summary (red)"}, jobNames(r, "summary"))

		reuse := map[string]Completion{}
		for _, j := range r.Jobs {
			if j.Status == StatusSuccess {
				reuse[j.Key] = Completion{Status: j.Status, Outputs: j.Outputs}
			}
		}
		r = planStress(t, Options{Attempt: 2, Reuse: reuse})
		waves := runQueued(t, r, failJobs(), stressOutputs())
		assert.Equal(t, []string{"build[1]/sign/notarize/staple/attest"}, waves[0])
		assert.Equal(t, []string{"summary"}, waves[len(waves)-1],
			"a job that needs a job that runs again runs again, although it succeeded")
		assert.Equal(t, []string{"summary (green)"}, jobNames(r, "summary"))
		assert.True(t, r.Job("build[0]/scan").Reused)
		assert.Equal(t, StatusSuccess, r.Result())
	})

	t.Run("fail-fast stops the held call", func(t *testing.T) {
		r := planStress(t, Options{})
		runQueued(t, r, failJobs("deploy[0]/smoke"), stressOutputs())
		g := r.Graph()
		deploy := graphNode(g, "deploy")
		assert.Equal(t, map[string]Status{
			"deploy[0]/plan": StatusSuccess, "deploy[0]/apply": StatusSuccess, "deploy[0]/smoke": StatusFailure,
			"deploy[0]/rollback": StatusSuccess,
		}, nodeStatuses(deploy.Calls[0]))
		assert.Equal(t, StatusCancelled, deploy.Calls[1].Status, "production never started")
		assert.Equal(t, StatusSuccess, graphNode(g, "notify-failure").Status)
		assert.Equal(t, StatusFailure, r.Result())
	})

	t.Run("cancelled", func(t *testing.T) {
		r := planStress(t, Options{})
		runQueued(t, r, func(j *Job) Status {
			if j.Key == "integration[1]" {
				r.Cancel()
			}
			return StatusSuccess
		}, stressOutputs())
		statuses := nodeStatuses(r.Graph())
		for key, want := range map[string]Status{
			"integration": StatusCancelled, "e2e": StatusCancelled, "deploy": StatusCancelled,
			"notify-failure": StatusCancelled, "summary": StatusSuccess, "cleanup": StatusSuccess,
		} {
			assert.Equal(t, want, statuses[key], key)
		}
		assert.Equal(t, StatusCancelled, r.Result())
	})
}
