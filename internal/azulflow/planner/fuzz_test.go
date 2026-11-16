package planner

import (
	"fmt"
	"hash/fnv"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/model"
)

// fuzzFiles serves a workflow and one reusable workflow, which every
// reference resolves to, so that the reusable workflow may call itself.
type fuzzFiles struct{ main, reusable string }

func (f fuzzFiles) ReadFile(name string) (string, error) {
	switch name {
	case "o/r/.github/workflows/ci.yml@sha":
		return f.main, nil
	case "o/r/.github/workflows/reusable.yml@sha":
		return f.reusable, nil
	}
	return "", errors.Newf("%s not found", name)
}

func (fuzzFiles) ResolveRef(_, _ string) (string, error) {
	return "o/r/.github/workflows/reusable.yml@sha", nil
}

// checkGraph checks the invariants of a graph: a node is queued only once
// the nodes it needs are done, a node is done only once its jobs and calls
// are, and a waiting node has no queued jobs.
func checkGraph(t *testing.T, g *Graph) {
	t.Helper()
	byKey := map[string]*GraphNode{}
	for _, n := range g.Nodes {
		byKey[n.Key] = n
	}
	for _, n := range g.Nodes {
		for _, k := range n.Needs {
			dep := byKey[k]
			require.NotNil(t, dep, "%s needs unknown node %s", n.Key, k)
			if n.Status != StatusWaiting {
				require.True(t, dep.Status.Done(), "%s is %s while %s it needs is %s", n.Key, n.Status, k, dep.Status)
			}
		}
		for _, j := range n.Jobs {
			if n.Status == StatusWaiting {
				require.Equal(t, StatusWaiting, j.Status, "%s of waiting node %s", j.Key, n.Key)
			}
			if n.Status.Done() {
				require.True(t, j.Status.Done(), "%s is %s in done node %s", j.Key, j.Status, n.Key)
			}
		}
		for _, c := range n.Calls {
			if n.Status.Done() {
				require.True(t, c.Status.Done(), "call %s is %s in done node %s", c.Key, c.Status, n.Key)
			}
			checkGraph(t, c)
		}
	}
	if g.Status.Done() {
		for _, n := range g.Nodes {
			require.True(t, n.Status.Done(), "%s is %s in done graph %s", n.Key, n.Status, g.Key)
		}
	}
}

// runToEnd completes queued jobs with the results decide picks, checking the
// graph after every change, until the run is done. It fails when jobs are
// left that can never run.
func runToEnd(t *testing.T, r *Run, decide func(j *Job, step int) Status) {
	t.Helper()
	checkGraph(t, r.Graph())
	for step := 0; ; step++ {
		var queued *Job
		for _, j := range r.Jobs {
			if j.Status == StatusQueued {
				queued = j
				break
			}
		}
		if queued == nil {
			break
		}
		require.Less(t, step, 100000, "the run does not end")
		status := decide(queued, step)
		if queued.Status != StatusQueued {
			checkGraph(t, r.Graph())
			continue // decide discarded it.
		}
		if queued.CancelRequested {
			status = StatusCancelled
		}
		_, err := r.Complete(queued.Key, status, map[string]string{"out": `["a"]`})
		require.NoError(t, err)
		checkGraph(t, r.Graph())
	}
	require.True(t, r.Done(), "no job is queued, but the run is not done")
	for _, j := range r.Jobs {
		require.True(t, j.Status.Done(), "%s is %s after the run", j.Key, j.Status)
	}
	require.True(t, r.Graph().Status.Done())
}

// FuzzLoadAndPlan loads arbitrary workflow files, plans them, and runs them to
// the end with arbitrary results. Nothing may panic, and the run must end.
func FuzzLoadAndPlan(f *testing.F) {
	corpus, err := filepath.Glob("../model/testdata/reader/*.yml")
	require.NoError(f, err)
	for _, path := range corpus {
		data, err := os.ReadFile(path)
		require.NoError(f, err)
		docs := strings.Split(string(data), "\n---\n")
		if len(docs) < 3 {
			continue
		}
		reusable := ""
		if len(docs) >= 5 {
			reusable = docs[3]
		}
		f.Add(docs[1], reusable, uint64(0))
	}
	stress, err := filepath.Glob("testdata/complex/.github/workflows/*.yml")
	require.NoError(f, err)
	for _, path := range stress {
		data, err := os.ReadFile(path)
		require.NoError(f, err)
		if strings.Contains(string(data), "workflow_call") {
			f.Add("on: push\njobs:\n  a:\n    uses: ./x.yml\n  b:\n    needs: a\n    runs-on: x\n    steps: [run: x]\n", string(data), uint64(5))
		}
	}
	github, err := expr.FromJSON(`{"event_name": "push", "ref": "refs/heads/main", "repository": "o/r", "event": {}}`)
	require.NoError(f, err)

	f.Fuzz(func(t *testing.T, main, reusable string, seed uint64) {
		w, err := model.Load(fuzzFiles{main, reusable}, "o/r/.github/workflows/ci.yml@sha", model.DefaultOptions())
		if err != nil || len(w.Errors) > 0 {
			return
		}
		r, err := Plan(Options{Workflow: w, GitHub: github.(*expr.Object)})
		if err != nil {
			return
		}
		cancelAt := int(seed>>32) % 64
		runToEnd(t, r, func(j *Job, step int) Status {
			if seed&1 == 1 && step == cancelAt {
				// Cancel the run, or what holds or waits for a concurrency
				// group: the run, the call that contains j, or j.
				call := ""
				if i := strings.LastIndex(j.Key, "/"); i >= 0 {
					call = j.Key[:i+1]
				}
				switch seed >> 8 % 5 {
				case 0:
					r.Cancel()
				case 1:
					r.Discard("")
				case 2:
					r.CancelClaim(call)
				case 3:
					r.Discard(call)
				case 4:
					r.Discard(j.Key)
					return StatusCancelled
				}
			}
			return []Status{StatusSuccess, StatusSuccess, StatusFailure, StatusCancelled}[(seed>>(step%30*2))&3]
		})
	})
}

// genJob is a job of a generated workflow.
type genJob struct {
	id          string
	needs       []int
	cond        string // The `if`, or empty for none.
	call        bool
	matrix      int // Matrix size, or 0 for no matrix.
	failFast    bool
	coe         bool
	maxParallel bool
}

// genWorkflow generates the jobs of a workflow from fuzz input. Calling jobs
// are generated only when calls is true.
func genWorkflow(next func() byte, prefix string, calls bool) []genJob {
	jobs := make([]genJob, 1+int(next())%8)
	for i := range jobs {
		j := &jobs[i]
		j.id = fmt.Sprintf("%s%d", prefix, i)
		for k := range i {
			if next()%3 == 0 {
				j.needs = append(j.needs, k)
			}
		}
		conds := []string{"", "", "always()", "failure()", "!failure()", "cancelled()", "false", "needs"}
		j.cond = conds[next()%8]
		if j.cond == "needs" && len(j.needs) == 0 {
			j.cond = ""
		}
		j.call = calls && next()%3 == 0
		j.matrix = int(next()) % 4
		if j.call {
			j.matrix %= 3
			j.maxParallel = next()%2 == 0
		} else {
			j.failFast = next()%2 == 0
			j.coe = next()%4 == 0
		}
	}
	return jobs
}

func (j genJob) ifExpr(jobs []genJob) string {
	if j.cond == "needs" {
		return fmt.Sprintf("always() && needs.%s.result == 'failure'", jobs[j.needs[0]].id)
	}
	return j.cond
}

func genYAML(on string, jobs []genJob) string {
	var b strings.Builder
	b.WriteString("on: " + on + "\njobs:\n")
	for _, j := range jobs {
		fmt.Fprintf(&b, "  %s:\n", j.id)
		if len(j.needs) > 0 {
			var ids []string
			for _, k := range j.needs {
				ids = append(ids, jobs[k].id)
			}
			fmt.Fprintf(&b, "    needs: [%s]\n", strings.Join(ids, ", "))
		}
		if j.cond != "" {
			fmt.Fprintf(&b, "    if: ${{ %s }}\n", j.ifExpr(jobs))
		}
		if j.matrix > 0 {
			var values []string
			for i := range j.matrix {
				values = append(values, fmt.Sprint(i))
			}
			fmt.Fprintf(&b, "    strategy:\n      fail-fast: %t\n      matrix:\n        i: [%s]\n", j.failFast, strings.Join(values, ", "))
			if j.maxParallel {
				b.WriteString("      max-parallel: 1\n")
			}
		}
		if j.call {
			b.WriteString("    uses: ./.github/workflows/reusable.yml\n")
			continue
		}
		if j.coe {
			b.WriteString("    continue-on-error: true\n")
		}
		b.WriteString("    runs-on: x\n    steps: [{run: x}]\n")
	}
	return b.String()
}

// outcome is the result the runner reports for a job, picked by its key.
func outcome(salt byte, key string) Status {
	h := fnv.New32a()
	h.Write([]byte{salt})
	h.Write([]byte(key))
	if h.Sum32()%4 == 0 {
		return StatusFailure
	}
	return StatusSuccess
}

// oracle predicts the statuses of a run from GitHub's documented rules,
// independently of the planner: a job runs when its `if` passes, the
// default `if` is success(), and the status functions look at every job
// before it in the chain of needs, including the jobs of the workflows those
// jobs called and the jobs before the job that called its workflow.
type oracle struct {
	t      *testing.T
	main   []genJob
	called []genJob
	salt   byte
}

// aggregateOf is GitHub's result of a job with several parts: any failure
// fails it, then any cancellation cancels it, and it is skipped only when
// every part is.
func aggregateOf(statuses []Status) Status {
	switch {
	case slices.Contains(statuses, StatusFailure):
		return StatusFailure
	case slices.Contains(statuses, StatusCancelled):
		return StatusCancelled
	case !slices.ContainsFunc(statuses, func(s Status) bool { return s != StatusSkipped }):
		return StatusSkipped
	}
	return StatusSuccess
}

// check checks a graph of jobs defs, whose jobs are preceded in the chain
// by the statuses before, and returns the statuses of all its jobs
// including those of the workflows they called.
func (o *oracle) check(g *Graph, defs []genJob, before []Status) []Status {
	require.Len(o.t, g.Nodes, len(defs))
	results := make([]Status, len(defs))
	// chain[i] are the statuses of every job before job i in the chain of
	// needs, and inner[i] the statuses of job i and the jobs it called.
	chain := make([][]Status, len(defs))
	inner := make([][]Status, len(defs))
	for i, def := range defs {
		n := g.Nodes[i]
		prior := slices.Clone(before)
		seen := map[int]bool{}
		var visit func(k int)
		visit = func(k int) {
			if !seen[k] {
				seen[k] = true
				prior = append(prior, inner[k]...)
				for _, d := range defs[k].needs {
					visit(d)
				}
			}
		}
		for _, k := range def.needs {
			visit(k)
		}
		chain[i] = prior

		success := !slices.ContainsFunc(prior, func(s Status) bool { return s != StatusSuccess })
		failure := slices.Contains(prior, StatusFailure)
		run := success
		switch def.cond {
		case "always()":
			run = true
		case "failure()":
			run = failure
		case "!failure()":
			run = !failure
		case "cancelled()", "false":
			run = false
		case "needs":
			run = results[def.needs[0]] == StatusFailure
		}
		if !run {
			require.Equal(o.t, StatusSkipped, n.Status, "%s should be skipped", n.Key)
			require.Empty(o.t, n.Calls, n.Key)
			results[i], inner[i] = StatusSkipped, []Status{StatusSkipped}
			continue
		}

		if def.call {
			require.Len(o.t, n.Calls, max(def.matrix, 1), n.Key)
			var calls []Status
			for _, c := range n.Calls {
				statuses := o.check(c, o.called, prior)
				inner[i] = append(inner[i], statuses...)
				calls = append(calls, aggregateOf(statuses[len(statuses)-len(o.called):]))
				require.Equal(o.t, calls[len(calls)-1], c.Status, "result of call %s", c.Key)
			}
			results[i] = aggregateOf(calls)
		} else {
			require.Len(o.t, n.Jobs, max(def.matrix, 1), n.Key)
			var legs []Status
			failed := false
			for _, j := range n.Jobs {
				if j.Status == StatusFailure {
					failed = true
				}
			}
			for _, j := range n.Jobs {
				want := outcome(o.salt, j.Key)
				if j.Status == StatusCancelled {
					require.True(o.t, def.failFast && failed && !def.coe, "%s was cancelled without a failed sibling", j.Key)
					want = StatusCancelled
				}
				require.Equal(o.t, want, j.Status, j.Key)
				if want == StatusFailure && def.coe {
					want = StatusSuccess
				}
				legs = append(legs, want)
			}
			results[i] = aggregateOf(legs)
			inner[i] = []Status{results[i]}
		}
		require.Equal(o.t, results[i], n.Status, "result of %s", n.Key)
	}
	var all []Status
	for i := range defs {
		all = append(all, inner[i]...)
	}
	// The last statuses are the results of the jobs of this workflow, which
	// make up its result.
	return append(all, results...)
}

// FuzzPlanOracle generates workflows with needs, status functions, matrices,
// fail-fast, continue-on-error, and matrices of calls with max-parallel, runs
// them with results picked by job key, and compares the statuses with the
// oracle.
func FuzzPlanOracle(f *testing.F) {
	f.Add([]byte{7, 0, 0, 3, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20})
	f.Add([]byte{5, 2, 0, 0, 1, 0, 2, 2, 3, 0, 1, 0, 0, 4, 0, 0, 0, 6, 3, 0, 0, 2, 1})
	github, err := expr.FromJSON(`{"event_name": "push", "ref": "refs/heads/main", "repository": "o/r", "event": {}}`)
	require.NoError(f, err)
	f.Fuzz(func(t *testing.T, data []byte) {
		pos := 0
		next := func() byte {
			if pos >= len(data) {
				return 0
			}
			pos++
			return data[pos-1]
		}
		salt := next()
		called := genWorkflow(next, "k", false)
		main := genWorkflow(next, "j", true)
		files := fuzzFiles{genYAML("push", main), genYAML("workflow_call", called)}
		w, err := model.Load(files, "o/r/.github/workflows/ci.yml@sha", model.DefaultOptions())
		require.NoError(t, err)
		require.Empty(t, w.Errors, files.main)
		r, err := Plan(Options{Workflow: w, GitHub: github.(*expr.Object)})
		require.NoError(t, err)
		runToEnd(t, r, func(j *Job, _ int) Status {
			var running func(g *Graph)
			running = func(g *Graph) {
				for i, n := range g.Nodes {
					queued := 0
					for _, c := range n.Calls {
						if c.Status == StatusQueued {
							queued++
						}
						running(c)
					}
					if defs := main; g.Key == "" && main[i].maxParallel {
						require.LessOrEqual(t, queued, 1, "%s runs calls at once despite max-parallel: 1\n%s", n.Key, genYAML("push", defs))
					}
				}
			}
			running(r.Graph())
			return outcome(salt, j.Key)
		})
		o := &oracle{t: t, main: main, called: called, salt: salt}
		o.check(r.Graph(), main, nil)
	})
}
