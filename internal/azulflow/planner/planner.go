// Package planner is the Azulflow planner. It turns a workflow and an event
// into jobs, and decides which jobs can run as earlier jobs finish.
//
// The workflow comes from model.Load, which has already read the reusable
// workflows it calls and checked the calls. The planner evaluates the parts of
// the workflow that GitHub evaluates on the server, with GitHub's schema
// definitions and error messages: run-name, concurrency, and for each job its
// if, strategy, name, runs-on, environment, timeouts, continue-on-error, and
// the inputs, outputs, and secrets of reusable workflows.
//
// A Run is a deterministic state machine without I/O. The scheduler persists
// the jobs it reports and restores a Run by calling Plan with the same Options
// and replaying its Complete and Cancel calls in their original order.
package planner

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/model"
	"gogs.io/gogs/internal/azulflow/template"
)

// Status is the state of a job.
type Status string

const (
	// StatusWaiting is a job whose needs have not finished.
	StatusWaiting Status = "waiting"
	// StatusQueued is a job that the scheduler can run.
	StatusQueued    Status = "queued"
	StatusSuccess   Status = "success"
	StatusFailure   Status = "failure"
	StatusCancelled Status = "cancelled"
	StatusSkipped   Status = "skipped"
)

// Done reports whether the status is final.
func (s Status) Done() bool {
	switch s {
	case StatusSuccess, StatusFailure, StatusCancelled, StatusSkipped:
		return true
	}
	return false
}

const (
	// DefaultTimeoutMinutes is the timeout of a job that sets none.
	DefaultTimeoutMinutes = 360
	// DefaultCancelTimeoutMinutes is how long a cancelled job may take to
	// stop when it sets no cancel-timeout-minutes.
	DefaultCancelTimeoutMinutes = 5
	// maxMemory is the memory GitHub's workflow evaluator allows an
	// expression.
	maxMemory = 10 << 20
)

// Completion is the result of a job in an earlier attempt.
type Completion struct {
	Status  Status
	Outputs map[string]string
}

// Options are the inputs to Plan.
type Options struct {
	// Workflow is the workflow loaded by model.Load.
	Workflow *model.Workflow
	// GitHub is the github context, built by the trigger from the event.
	GitHub *expr.Object
	// Vars is the vars context.
	Vars *expr.Object
	// Inputs are the workflow_dispatch inputs, as the user gave them.
	Inputs map[string]any
	// DefaultPermissions apply to jobs when neither the job, the workflow,
	// nor a calling job sets permissions. Nil means the restricted default:
	// read access to contents and packages.
	DefaultPermissions model.Permissions
	// Attempt is the run attempt, starting at 1.
	Attempt int
	// Reuse maps job keys to their results in the previous attempt. When
	// re-running failed jobs, it holds the jobs that succeeded, which keep
	// their results and outputs instead of running again.
	Reuse map[string]Completion
	// CancelSupersededPullRequestRuns gives pull request runs of a workflow
	// without its own concurrency a group per pull request that cancels runs in
	// progress, so that a new push cancels the older runs.
	CancelSupersededPullRequestRuns bool
}

// Concurrency is an evaluated concurrency setting.
type Concurrency struct {
	Group            string
	CancelInProgress bool
	// Queue is how many runs may wait for the group: "single" (the default
	// when empty) keeps only the latest, and "max" keeps them all.
	Queue string
}

// Environment is the evaluated environment of a job.
type Environment struct {
	Name string
	// URL is evaluated by the agent, since it may read step outputs. It is
	// nil when not set.
	URL *template.Token
	// Deployment is whether the job creates a deployment.
	Deployment bool
}

// Job is a job the scheduler can run, or a placeholder that shows why a job
// was skipped or failed before it could run.
type Job struct {
	// Key identifies the job in the run, and stays the same across attempts.
	// It is the job ID, with the matrix index for matrix jobs and the caller
	// prefix for jobs of reusable workflows, such as "deploy/test[1]".
	Key string
	// JobID is the ID of the job in its workflow file.
	JobID string
	// Group is the key shared by all matrix jobs of one job definition.
	Group string
	Name  string
	// Workflow is the file that defines the job, as named in the file table
	// of the workflow.
	Workflow string
	// Needs lists the IDs of the jobs this job needs in its workflow file.
	Needs []string
	// Definition is the job as loaded. It is nil for the placeholder of a
	// called workflow's outputs.
	Definition *model.Job

	// The fields below are set when the job is queued.

	// Matrix is the matrix combination, or nil for a job without a matrix.
	Matrix               *expr.Object
	FailFast             bool
	MaxParallel          int
	RunsOn               []string
	RunnerGroup          string
	Environment          *Environment
	TimeoutMinutes       int
	CancelTimeoutMinutes int
	ContinueOnError      bool
	Concurrency          *Concurrency
	// CallConcurrency are the concurrency settings of the calls of reusable
	// workflows that contain this job, outermost first: those of the calling
	// jobs and of the called workflows. Each call holds its groups for all of
	// its jobs, so the job runs only once its calls hold them.
	CallConcurrency []CallConcurrency
	// Permissions are the effective permissions of the job token.
	Permissions model.Permissions
	// Contexts holds the contexts the agent needs from the server: github,
	// inputs, vars, needs, strategy, and matrix.
	Contexts *expr.Object

	Status Status
	// Outputs are the outputs the agent reported.
	Outputs map[string]string
	// Error explains a job that failed before it ran, such as a matrix that
	// could not be evaluated.
	Error string
	// Reused is true for a job that kept its result from an earlier attempt.
	Reused bool
	// CancelRequested is true for a queued job the scheduler must stop.
	CancelRequested bool

	node   *node
	seq    int // Order of completion.
	passes []secretsPass
}

// CallConcurrency is a concurrency setting that a call of a reusable
// workflow holds.
type CallConcurrency struct {
	// Call is the key prefix of the jobs of the call, such as "deploy[0]/".
	Call        string
	Concurrency *Concurrency
}

// Changes are what a call changed in the run.
type Changes struct {
	// Jobs were created or changed status. The scheduler persists them and
	// runs the queued ones.
	Jobs []*Job
	// Cancel lists queued jobs the scheduler must stop, then complete as
	// cancelled.
	Cancel []*Job
	// Calls are the key prefixes of the calls of reusable workflows that
	// finished, which release their concurrency groups.
	Calls []string
}

// Run is the planned state of one workflow run attempt.
type Run struct {
	// Name is the evaluated run-name, or empty when the workflow has none.
	Name        string
	Concurrency *Concurrency
	// Jobs are all jobs, in the order they were created.
	Jobs []*Job

	opts      *Options
	files     []string
	root      *scope
	cancelled bool
	seq       int
	byKey     map[string]*Job
	changes   Changes
}

// scope is one instance of a workflow: the top-level workflow, or a reusable
// workflow called by one matrix job of a calling job.
type scope struct {
	key         string // Prefix of the keys of its jobs.
	name        string // Name prefix of its jobs.
	inputs      *expr.Object
	passes      []secretsPass
	concurrency []CallConcurrency
	caller      *node
	nodes       []*node
	cancelled   bool // Cancelled by fail-fast or by concurrency.
	// discarded is true when the scope was cancelled before it started, so
	// none of its jobs runs.
	discarded bool
	// held is true for a called workflow that waits for an earlier one to
	// finish, because the calling job has a max-parallel.
	held bool

	done    bool
	result  Status
	outputs *expr.Object
	seq     int
}

// node is a job definition in a scope.
type node struct {
	scope    *scope
	def      *model.Job
	id       string
	key      string
	needs    []*node
	failFast bool

	released bool
	// rerun is true when a job of the node, or of a workflow it called, runs
	// in this attempt instead of keeping its earlier result.
	rerun    bool
	legs     []*Job   // Jobs of a regular job.
	children []*scope // Called workflows of a calling job.

	done    bool
	result  Status
	outputs *expr.Object
}

// secretsPass is how a calling job passes secrets to the workflow it calls.
type secretsPass struct {
	inherit bool
	values  *template.Token
	env     *expr.Env
	files   []string
}

// Plan plans a run: it evaluates the run settings and queues the jobs that
// need nothing.
func Plan(opts Options) (*Run, error) {
	w := opts.Workflow
	if len(w.Errors) > 0 {
		return nil, errors.New(strings.Join(w.Errors, "\n"))
	}
	if opts.Attempt == 0 {
		opts.Attempt = 1
	}
	if opts.GitHub == nil {
		opts.GitHub = expr.NewObject()
	}
	if opts.Vars == nil {
		opts.Vars = expr.NewObject()
	}
	if opts.DefaultPermissions == nil {
		opts.DefaultPermissions = model.Permissions{"contents": model.LevelRead, "packages": model.LevelRead}
	}
	r := &Run{opts: &opts, files: w.FileTable, byKey: map[string]*Job{}}

	inputs := expr.NewObject()
	if event, _ := opts.GitHub.Get("event_name"); opts.Inputs != nil || event == "workflow_dispatch" {
		var err error
		inputs, err = dispatchInputs(w, opts.Inputs)
		if err != nil {
			return nil, errors.Wrap(err, "workflow_dispatch inputs")
		}
	}

	if w.RunName != nil {
		err := r.evaluate("Error when evaluating 'run-name'.", "run-name", w.RunName, r.env(inputs, nil),
			func(_ *template.Context, t *template.Token) error {
				if err := t.AssertString("run-name"); err != nil {
					return err
				}
				r.Name = t.Str
				return nil
			})
		if err != nil {
			return nil, err
		}
	}
	if w.Concurrency != nil {
		c, err := r.concurrency("Error when evaluating 'concurrency'.", "workflow-concurrency", w.Concurrency, r.env(inputs, nil))
		if err != nil {
			return nil, err
		}
		r.Concurrency = c
	} else if opts.CancelSupersededPullRequestRuns {
		r.Concurrency = pullRequestConcurrency(w, opts.GitHub)
	}

	r.root = r.newScope("", "", w.Jobs, inputs, nil, nil, nil)
	r.advance()
	r.changes = Changes{}
	return r, nil
}

// evaluate expands the expressions of t whose contexts env provides, checks
// the result against the schema definition typ, and passes it to convert.
// Like GitHub's errors, the error starts with prefix and lists every problem.
func evaluate(files []string, prefix, typ string, t *template.Token, env *expr.Env, convert func(*template.Context, *template.Token) error) error {
	ctx := template.NewContext(template.WorkflowSchema(), template.DefaultLimits())
	for _, f := range files {
		ctx.FileID(f)
	}
	env.MaxMemory = maxMemory
	result := ctx.Expand(typ, t, env)
	if len(ctx.Errors()) == 0 && result != nil {
		if err := convert(ctx, result); err != nil {
			ctx.AddError(err.Error())
		}
	}
	errs := ctx.Errors()
	switch {
	case len(errs) == 0:
		return nil
	case prefix == "":
		return errors.New("The template is not valid. " + strings.Join(errs, ","))
	}
	return errors.New(prefix + " " + strings.Join(errs, ","))
}

func (r *Run) evaluate(prefix, typ string, t *template.Token, env *expr.Env, convert func(*template.Context, *template.Token) error) error {
	return evaluate(r.files, prefix, typ, t, env, convert)
}

func jobPrefix(what, jobID string) string {
	return "Error when evaluating '" + what + "' for job '" + jobID + "'."
}

func (r *Run) env(inputs *expr.Object, extra map[string]any) *expr.Env {
	ctxs := map[string]any{"github": r.opts.GitHub, "vars": r.opts.Vars, "inputs": inputs}
	maps.Copy(ctxs, extra)
	return &expr.Env{Contexts: ctxs}
}

func pullRequestConcurrency(w *model.Workflow, github *expr.Object) *Concurrency {
	event, _ := github.Get("event_name")
	switch event {
	case "pull_request", "pull_request_target":
	default:
		return nil
	}
	payload, _ := github.Get("event")
	number := index(payload, "number")
	if number == nil {
		number = index(index(payload, "pull_request"), "number")
	}
	if number == nil || len(w.FileTable) == 0 {
		return nil
	}
	return &Concurrency{
		Group:            fmt.Sprintf("%s/pull/%s", w.FileTable[0], expr.ToString(number)),
		CancelInProgress: true,
	}
}

func index(v any, key string) any {
	obj, _ := v.(*expr.Object)
	value, _ := obj.Get(key)
	return value
}

// concurrency evaluates a concurrency setting, as GitHub's
// ConvertToConcurrency does.
func (r *Run) concurrency(prefix, typ string, t *template.Token, env *expr.Env) (*Concurrency, error) {
	c := &Concurrency{}
	err := r.evaluate(prefix, typ, t, env, func(ctx *template.Context, t *template.Token) error {
		if t.Kind == template.String {
			c.Group = t.Str
		} else {
			if err := t.AssertMapping("concurrency"); err != nil {
				return err
			}
			for _, p := range t.Map {
				if err := p.Key.AssertString("concurrency key"); err != nil {
					return err
				}
				switch p.Key.Str {
				case "group":
					if err := p.Value.AssertString("group key"); err != nil {
						return err
					}
					c.Group = p.Value.Str
				case "cancel-in-progress":
					if err := p.Value.AssertBoolean("cancel-in-progress key"); err != nil {
						return err
					}
					c.CancelInProgress = p.Value.Bool
				case "queue":
					if err := p.Value.AssertString("queue key"); err != nil {
						return err
					}
					c.Queue = p.Value.Str
				default:
					return errors.New("Error while reading 'concurrency key'. Unexpected value '" + p.Key.Str + "'")
				}
			}
		}
		if c.Queue == "max" && c.CancelInProgress {
			ctx.Error(t, "Concurrency queue 'max' cannot be combined with 'cancel-in-progress: true'")
		}
		if c.Group == "" {
			ctx.Error(t, "Concurrency group name cannot be empty")
		} else if len([]rune(c.Group)) > 400 {
			ctx.Error(t, "Concurrency group name must be less than 400 characters")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *Run) newScope(key, name string, jobs []*model.Job, inputs *expr.Object, passes []secretsPass, concurrency []CallConcurrency, caller *node) *scope {
	s := &scope{
		key: key, name: name, inputs: inputs, passes: passes, concurrency: concurrency, caller: caller,
	}
	for _, def := range jobs {
		s.nodes = append(s.nodes, &node{scope: s, def: def, id: def.ID.Str, key: key + def.ID.Str})
	}
	for _, n := range s.nodes {
		for _, need := range n.def.Needs {
			for _, dep := range s.nodes {
				if strings.EqualFold(dep.id, need.Str) {
					n.needs = append(n.needs, dep)
				}
			}
		}
	}
	// Expand matrices that do not depend on needs right away, so that their
	// jobs show while they wait.
	for _, n := range s.nodes {
		if n.def.Type != model.JobTypeJob || (n.def.Strategy != nil && readsContext(n.def.Strategy, "needs")) {
			continue
		}
		st, err := r.strategy(n, expr.NewObject())
		if err != nil {
			continue // Reported when the job is released.
		}
		for i, c := range st.configs {
			r.addJob(&Job{
				Key:        legKey(n, st, i),
				Name:       legName(n, c.name),
				Status:     StatusWaiting,
				Matrix:     c.matrix,
				Definition: n.def,
			}, n)
		}
	}
	return s
}

func legKey(n *node, st *strategy, i int) string {
	if !st.matrix {
		return n.key
	}
	return fmt.Sprintf("%s[%d]", n.key, i)
}

// legName is the display name of a job under the name of the job that called
// its workflow.
func legName(n *node, name string) string {
	if n.scope.name != "" {
		return n.scope.name + " / " + name
	}
	return name
}

// addJob adds a job as one of the matrix jobs of n.
func (r *Run) addJob(j *Job, n *node) {
	n.legs = append(n.legs, j)
	r.register(j, n)
}

// register adds a job to the run, as defined by n.
func (r *Run) register(j *Job, n *node) {
	j.node = n
	j.JobID = n.id
	j.Group = n.key
	j.Workflow = fileName(r.files, n.def.ID.File)
	j.Needs = make([]string, len(n.def.Needs))
	for i, need := range n.def.Needs {
		j.Needs[i] = need.Str
	}
	j.passes = n.scope.passes
	j.CallConcurrency = n.scope.concurrency
	r.Jobs = append(r.Jobs, j)
	r.byKey[j.Key] = j
	r.touch(j)
}

func fileName(files []string, id int) string {
	if id < 1 || id > len(files) {
		return ""
	}
	return files[id-1]
}

func (r *Run) touch(j *Job) {
	if !slices.Contains(r.changes.Jobs, j) {
		r.changes.Jobs = append(r.changes.Jobs, j)
	}
}

func (r *Run) flush() Changes {
	c := r.changes
	r.changes = Changes{}
	return c
}

// Job returns the job with the given key, or nil.
func (r *Run) Job(key string) *Job {
	return r.byKey[key]
}

// Done reports whether every job has finished.
func (r *Run) Done() bool {
	return r.root.done
}

// Result returns the result of the run once it is done.
func (r *Run) Result() Status {
	return r.root.result
}

// Complete records the result of a queued job and returns what changed.
// Status must be success, failure, or cancelled. A failed job with
// continue-on-error counts as a success for the jobs that need it and does not
// cancel the other jobs of its matrix.
func (r *Run) Complete(key string, status Status, outputs map[string]string) (Changes, error) {
	j := r.byKey[key]
	switch {
	case j == nil:
		return Changes{}, errors.Newf("no job %q", key)
	case j.Status != StatusQueued:
		return Changes{}, errors.Newf("job %q is %s, not queued", key, j.Status)
	case status != StatusSuccess && status != StatusFailure && status != StatusCancelled:
		return Changes{}, errors.Newf("job %q cannot complete as %s", key, status)
	}
	r.finish(j, status)
	j.Outputs = outputs
	if status == StatusFailure && !j.ContinueOnError && j.node.failFast {
		for _, leg := range j.node.legs {
			r.cancelJob(leg)
		}
	}
	r.advance()
	return r.flush(), nil
}

// Cancel cancels the run. Queued jobs are stopped, and jobs that have not
// started run only when their `if` still passes, such as with always().
func (r *Run) Cancel() Changes {
	if !r.cancelled {
		r.cancelled = true
		for _, j := range r.Jobs {
			r.cancelJob(j)
		}
		r.advance()
	}
	return r.flush()
}

// CancelClaim cancels what holds a concurrency group that a newer run or job
// claimed with cancel-in-progress: the run for an empty key, a call of a
// reusable workflow for its key prefix, such as "deploy[0]/", or a job. As
// with Cancel, queued jobs are stopped, and jobs that have not started run
// only when their `if` still passes.
func (r *Run) CancelClaim(key string) Changes {
	switch {
	case key == "":
		return r.Cancel()
	case strings.HasSuffix(key, "/"):
		if s := r.callScope(r.root, key); s != nil && !s.done {
			r.cancelScope(s)
		}
	default:
		if j := r.byKey[key]; j != nil {
			r.cancelJob(j)
		}
	}
	r.advance()
	return r.flush()
}

// Discard cancels a run, a call, or a job, keyed as for CancelClaim, that
// never started because a newer claim replaced it while it waited for a
// concurrency group. None of its jobs runs, whatever their `if`.
func (r *Run) Discard(key string) Changes {
	switch {
	case key == "":
		r.cancelled = true
		r.discardScope(r.root)
	case strings.HasSuffix(key, "/"):
		if s := r.callScope(r.root, key); s != nil && !s.done {
			r.discardScope(s)
		}
	default:
		if j := r.byKey[key]; j != nil && !j.Status.Done() {
			r.finish(j, StatusCancelled)
		}
	}
	r.advance()
	return r.flush()
}

func (r *Run) discardScope(s *scope) {
	s.cancelled, s.discarded, s.held = true, true, false
	for _, n := range s.nodes {
		for _, j := range n.legs {
			if j.Status == StatusQueued {
				r.finish(j, StatusCancelled)
			}
		}
		for _, c := range n.children {
			r.discardScope(c)
		}
	}
}

// callScope returns the called workflow with the given key prefix.
func (r *Run) callScope(s *scope, key string) *scope {
	for _, n := range s.nodes {
		for _, c := range n.children {
			if c.key == key {
				return c
			}
			if strings.HasPrefix(key, c.key) {
				return r.callScope(c, key)
			}
		}
	}
	return nil
}

func (r *Run) finish(j *Job, status Status) {
	j.Status = status
	j.CancelRequested = false
	r.seq++
	j.seq = r.seq
	r.touch(j)
}

// cancelJob stops a queued job. A waiting job is left to its `if`, which
// decides whether it runs, as always() does, or is cancelled.
func (r *Run) cancelJob(j *Job) {
	if j.Status == StatusQueued && !j.CancelRequested {
		j.CancelRequested = true
		r.touch(j)
		r.changes.Cancel = append(r.changes.Cancel, j)
	}
}

// cancelScope cancels a called workflow whose sibling failed with fail-fast.
func (r *Run) cancelScope(s *scope) {
	s.cancelled, s.held = true, false
	for _, n := range s.nodes {
		for _, j := range n.legs {
			r.cancelJob(j)
		}
		for _, c := range n.children {
			r.cancelScope(c)
		}
	}
}

// advance releases jobs whose needs are done and concludes finished jobs,
// until nothing changes.
func (r *Run) advance() {
	for r.release(r.root) || r.settle(r.root) {
	}
}

func (r *Run) release(s *scope) bool {
	if s.held {
		return false
	}
	progress := false
	for _, n := range s.nodes {
		if !n.released && !slices.ContainsFunc(n.needs, func(dep *node) bool { return !dep.done }) {
			n.released = true
			r.releaseNode(n)
			progress = true
		}
		for _, c := range n.children {
			if r.release(c) {
				progress = true
			}
		}
	}
	return progress
}

func (r *Run) settle(s *scope) bool {
	progress := false
	for _, n := range s.nodes {
		if !n.released || n.done {
			continue
		}
		for _, c := range n.children {
			if r.settle(c) {
				progress = true
			}
		}
		reusable := n.def.Type == model.JobTypeReusable
		if !reusable && !slices.ContainsFunc(n.legs, func(j *Job) bool { return !j.Status.Done() }) {
			r.concludeLegs(n)
			progress = true
		} else if reusable && len(n.children) > 0 && !slices.ContainsFunc(n.children, func(c *scope) bool { return !c.done }) {
			r.concludeCalls(n)
			progress = true
		}
	}
	if !s.done && !slices.ContainsFunc(s.nodes, func(n *node) bool { return !n.done }) {
		r.finishScope(s)
		progress = true
	}
	return progress
}

func (r *Run) releaseNode(n *node) {
	needs := needsContext(n)
	run, err := r.condition(n, needs)
	if err != nil {
		r.failNode(n, err)
		return
	}
	run = run && !r.isDiscarded(n.scope)
	if !run {
		status := StatusSkipped
		if r.isCancelled(n.scope) {
			status = StatusCancelled
		}
		if len(n.legs) == 0 {
			r.addJob(&Job{Key: n.key, Name: legName(n, defaultName(n.def)), Status: status, Definition: n.def}, n)
		}
		for _, j := range n.legs {
			if !j.Status.Done() {
				r.finish(j, status)
			}
		}
		n.done, n.result = true, status
		return
	}
	if n.def.Type == model.JobTypeReusable {
		r.expandCall(n, needs)
	} else {
		r.expandJobs(n, needs)
	}
}

// defaultName is the name of a job before its matrix and name expressions
// are evaluated: its literal name, or its ID.
func defaultName(def *model.Job) string {
	if def.Name != nil && def.Name.Kind == template.String && def.Name.Str != "" {
		return def.Name.Str
	}
	return def.ID.Str
}

func (r *Run) isDiscarded(s *scope) bool {
	for ; s != nil; s = callerScope(s) {
		if s.discarded {
			return true
		}
	}
	return false
}

func (r *Run) isCancelled(s *scope) bool {
	for ; s != nil; s = callerScope(s) {
		if s.cancelled {
			return true
		}
	}
	return r.cancelled
}

func callerScope(s *scope) *scope {
	if s.caller == nil {
		return nil
	}
	return s.caller.scope
}

// failNode fails a job that could not be planned, keeping the error on the
// job or on a placeholder.
func (r *Run) failNode(n *node, err error) {
	if len(n.legs) == 0 {
		r.addJob(&Job{Key: n.key, Name: legName(n, defaultName(n.def)), Status: StatusQueued, Definition: n.def}, n)
	}
	for _, j := range n.legs {
		if j.Status.Done() {
			continue
		}
		j.Error = err.Error()
		r.finish(j, StatusFailure)
	}
	n.done, n.result = true, StatusFailure
	markRerun(n)
}

// condition evaluates a job-level `if`, which the loader has already combined
// with success() when it calls no status function.
func (r *Run) condition(n *node, needs *expr.Object) (bool, error) {
	ancestors := ancestors(n)
	cancelled := r.isCancelled(n.scope)
	success := !cancelled && !slices.ContainsFunc(ancestors, func(a *node) bool { return a.result != StatusSuccess })
	failure := slices.ContainsFunc(ancestors, func(a *node) bool { return a.result == StatusFailure })
	if n.def.If == nil {
		return success, nil
	}
	env := r.env(n.scope.inputs, map[string]any{"needs": needs})
	env.Funcs = map[string]expr.Func{
		"success":   func([]any) (any, error) { return success, nil },
		"failure":   func([]any) (any, error) { return failure, nil },
		"cancelled": func([]any) (any, error) { return cancelled, nil },
		"always":    func([]any) (any, error) { return true, nil },
	}
	var run bool
	err := r.evaluate(jobPrefix("if", n.id), "job-if-result", n.def.If, env, func(_ *template.Context, t *template.Token) error {
		var unexpected *template.Token
		t.Traverse(func(x *template.Token) bool {
			if x.IsExpression() && unexpected == nil {
				unexpected = x
			}
			return unexpected == nil
		})
		if unexpected != nil {
			return errors.New("Unexpected type '" + unexpected.Kind.Name() + "' encountered while reading 'if'.")
		}
		run = expr.Truthy(t.Value())
		return nil
	})
	return run, err
}

// ancestors returns the jobs before a job in its chain of needs: the jobs it
// needs, directly or not, the jobs of the workflows those jobs called, and
// the same for the jobs that called its own workflow. Like GitHub, the status
// functions see a job skipped inside a called workflow.
func ancestors(n *node) []*node {
	var out []*node
	var add func(m *node)
	add = func(m *node) {
		if slices.Contains(out, m) {
			return
		}
		out = append(out, m)
		for _, dep := range m.needs {
			add(dep)
		}
		for _, c := range m.children {
			for _, x := range c.nodes {
				add(x)
			}
		}
	}
	for ; n != nil; n = n.scope.caller {
		for _, dep := range n.needs {
			add(dep)
		}
	}
	return out
}

func needsContext(n *node) *expr.Object {
	ctx := expr.NewObject()
	for _, dep := range n.needs {
		v := expr.NewObject()
		v.Set("result", string(dep.result))
		v.Set("outputs", orEmpty(dep.outputs))
		ctx.Set(dep.id, v)
	}
	return ctx
}

func orEmpty(o *expr.Object) *expr.Object {
	if o == nil {
		return expr.NewObject()
	}
	return o
}

// matrixValue returns the matrix context, which is null for a job without a
// matrix.
func matrixValue(m *expr.Object) any {
	if m == nil {
		return nil
	}
	return m
}

// strategy evaluates the strategy of a job. A job without a matrix has one
// configuration.
func (r *Run) strategy(n *node, needs *expr.Object) (*strategy, error) {
	base := defaultName(n.def)
	if n.def.Name != nil && n.def.Name.IsExpression() {
		base = n.def.ID.Str
	}
	st := &strategy{failFast: true}
	if n.def.Strategy != nil {
		env := r.env(n.scope.inputs, map[string]any{"needs": needs})
		err := r.evaluate(jobPrefix("strategy", n.id), "strategy", n.def.Strategy, env, func(ctx *template.Context, t *template.Token) error {
			var err error
			st, err = convertStrategy(ctx, t, base)
			return err
		})
		if err != nil {
			return nil, err
		}
	}
	if len(st.configs) == 0 {
		st.matrix = false
		st.configs = []configuration{{name: jobName(base, nil)}}
	}
	return st, nil
}

func (r *Run) expandJobs(n *node, needs *expr.Object) {
	st, err := r.strategy(n, needs)
	if err != nil {
		r.failNode(n, err)
		return
	}
	n.failFast = st.failFast
	if len(n.legs) == 0 {
		for i, c := range st.configs {
			r.addJob(&Job{Key: legKey(n, st, i), Name: legName(n, c.name), Matrix: c.matrix, Definition: n.def, Status: StatusWaiting}, n)
		}
	}
	perms := n.def.Permissions
	if perms == nil {
		perms = r.opts.DefaultPermissions
	}
	for i, j := range n.legs {
		env := r.env(n.scope.inputs, map[string]any{
			"needs":    needs,
			"matrix":   matrixValue(j.Matrix),
			"strategy": st.context(i),
		})
		if err := r.prepare(j, st.configs[i].name, env); err != nil {
			r.failNode(n, err)
			return
		}
		j.FailFast, j.MaxParallel, j.Permissions = st.failFast, st.maxParallel, perms
		j.Contexts = contexts(env)
	}
	// Like GitHub, a job runs again when a job it needs runs again.
	reuse := !slices.ContainsFunc(ancestors(n), func(a *node) bool { return a.rerun })
	for _, j := range n.legs {
		if c, ok := r.opts.Reuse[j.Key]; ok && c.Status == StatusSuccess && reuse {
			j.Reused, j.Outputs = true, c.Outputs
			r.finish(j, StatusSuccess)
			continue
		}
		j.Status = StatusQueued
		r.touch(j)
		markRerun(n)
	}
}

// markRerun marks a node, and the jobs that called its workflow, as running
// again in this attempt.
func markRerun(n *node) {
	for ; n != nil; n = n.scope.caller {
		n.rerun = true
	}
}

func contexts(env *expr.Env) *expr.Object {
	keys := slices.Sorted(maps.Keys(env.Contexts))
	ctx := expr.NewObject()
	for _, k := range keys {
		ctx.Set(k, env.Contexts[k])
	}
	return ctx
}

// name evaluates the name of a job when it is an expression. An empty result
// falls back to the default name.
func (r *Run) name(n *node, defaultName string, env *expr.Env) (string, error) {
	if n.def.Name == nil || !n.def.Name.IsExpression() {
		return defaultName, nil
	}
	name := defaultName
	err := r.evaluate(jobPrefix("name", n.id), "string-strategy-context", n.def.Name, env, func(_ *template.Context, t *template.Token) error {
		if err := t.AssertString("job name"); err != nil {
			return err
		}
		if t.Str != "" {
			name = t.Str
		}
		return nil
	})
	return name, err
}

// prepare evaluates the fields of a job that the server needs.
func (r *Run) prepare(j *Job, defaultName string, env *expr.Env) error {
	n := j.node
	def := n.def
	name, err := r.name(n, defaultName, env)
	if err != nil {
		return err
	}
	j.Name = legName(n, name)

	j.RunsOn, j.RunnerGroup = nil, ""
	if def.RunsOn != nil {
		err := r.evaluate(jobPrefix("runs-on", n.id), "runs-on", def.RunsOn, env, func(ctx *template.Context, t *template.Token) error {
			return convertRunsOn(ctx, t, j)
		})
		if err != nil {
			return err
		}
	}

	j.Environment = nil
	if def.Environment != nil {
		err := r.evaluate(jobPrefix("environment", n.id), "job-environment", def.Environment, env, func(_ *template.Context, t *template.Token) error {
			var err error
			j.Environment, err = convertEnvironment(t)
			return err
		})
		if err != nil {
			return err
		}
	}

	j.TimeoutMinutes = DefaultTimeoutMinutes
	if def.TimeoutMinutes != nil {
		err := r.evaluate(jobPrefix("timeout-minutes", n.id), "number-strategy-context", def.TimeoutMinutes, env, func(_ *template.Context, t *template.Token) error {
			if err := t.AssertNumber("job timeout-minutes"); err != nil {
				return err
			}
			j.TimeoutMinutes = int(t.Num)
			return nil
		})
		if err != nil {
			return err
		}
	}

	j.CancelTimeoutMinutes = DefaultCancelTimeoutMinutes
	if def.CancelTimeoutMinutes != nil {
		err := r.evaluate(jobPrefix("cancel-timeout-minutes", n.id), "number-strategy-context", def.CancelTimeoutMinutes, env, func(_ *template.Context, t *template.Token) error {
			if err := t.AssertNumber("job cancel-timeout-minutes"); err != nil {
				return err
			}
			j.CancelTimeoutMinutes = int(t.Num)
			return nil
		})
		if err != nil {
			return err
		}
	}

	j.ContinueOnError = false
	if def.ContinueOnError != nil {
		err := r.evaluate(jobPrefix("continue-on-error", n.id), "boolean-strategy-context", def.ContinueOnError, env, func(_ *template.Context, t *template.Token) error {
			if err := t.AssertBoolean("job continue-on-error"); err != nil {
				return err
			}
			j.ContinueOnError = t.Bool
			return nil
		})
		if err != nil {
			return err
		}
	}

	j.Concurrency = nil
	if def.Concurrency != nil {
		c, err := r.concurrency(jobPrefix("concurrency", n.id), "job-concurrency", def.Concurrency, env)
		if err != nil {
			return err
		}
		j.Concurrency = c
	}
	return nil
}

// convertRunsOn converts an evaluated runs-on, as GitHub's ConvertToRunsOn
// does.
func convertRunsOn(ctx *template.Context, t *template.Token, j *Job) error {
	labels := func(t *template.Token) error {
		switch t.Kind {
		case template.String:
			j.RunsOn = append(j.RunsOn, t.Str)
		case template.Sequence:
			for _, item := range t.Seq {
				if err := item.AssertString("job runs-on labels sequence item"); err != nil {
					return err
				}
				j.RunsOn = append(j.RunsOn, item.Str)
			}
		}
		return nil
	}
	if err := labels(t); err != nil {
		return err
	}
	if t.Kind != template.Mapping {
		return nil
	}
	for _, p := range t.Map {
		if err := p.Key.AssertString("job runs-on property name"); err != nil {
			return err
		}
		switch p.Key.Str {
		case "group":
			if err := p.Value.AssertString("job runs-on group name"); err != nil {
				return err
			}
			group := p.Value.Str
			names := strings.Split(group, "/")
			invalidPrefix := "Invalid runs-on group name '" + group + "'. Please use 'organization/' or 'enterprise/' prefix to target a single runner group."
			switch {
			case len(names) == 2 && names[1] == "":
				ctx.Error(p.Value, "Invalid runs-on group name '"+group+"'.")
			case len(names) == 2 && !slices.Contains([]string{"org", "organization", "ent", "enterprise"}, names[0]):
				ctx.Error(p.Value, invalidPrefix)
			case len(names) > 2:
				ctx.Error(p.Value, invalidPrefix)
			default:
				j.RunnerGroup = group
			}
		case "labels":
			if err := labels(p.Value); err != nil {
				return err
			}
		}
	}
	return nil
}

// convertEnvironment converts an evaluated environment, as GitHub's
// ConvertToActionEnvironmentReference does. An empty name means no
// environment.
func convertEnvironment(t *template.Token) (*Environment, error) {
	if t.Kind == template.String {
		if t.Str == "" {
			return nil, nil
		}
		return &Environment{Name: t.Str, Deployment: true}, nil
	}
	if err := t.AssertMapping("job environment"); err != nil {
		return nil, err
	}
	env := &Environment{Deployment: true}
	for _, p := range t.Map {
		if err := p.Key.AssertString("job environment key"); err != nil {
			return nil, err
		}
		switch p.Key.Str {
		case "name":
			if err := p.Value.AssertString("job environment name"); err != nil {
				return nil, err
			}
			env.Name = p.Value.Str
		case "url":
			env.URL = p.Value
		case "deployment":
			if err := p.Value.AssertBoolean("job environment deployment"); err != nil {
				return nil, err
			}
			env.Deployment = p.Value.Bool
		default:
			return nil, errors.New("Error while reading 'job environment key'. Unexpected value '" + p.Key.Str + "'")
		}
	}
	if env.Name == "" {
		return nil, nil
	}
	return env, nil
}

// expandCall instantiates the called workflow once for every matrix
// combination of the calling job.
func (r *Run) expandCall(n *node, needs *expr.Object) {
	st, err := r.strategy(n, needs)
	if err != nil {
		r.failNode(n, err)
		return
	}
	n.failFast = st.failFast

	var children []*scope
	for i, c := range st.configs {
		env := r.env(n.scope.inputs, map[string]any{
			"needs":    needs,
			"matrix":   matrixValue(c.matrix),
			"strategy": st.context(i),
		})
		name, err := r.name(n, c.name, env)
		if err != nil {
			r.failNode(n, err)
			return
		}
		inputs, err := r.callInputs(n, env)
		if err != nil {
			r.failNode(n, err)
			return
		}
		key := legKey(n, st, i) + "/"
		concurrency := slices.Clip(n.scope.concurrency)
		if n.def.Concurrency != nil {
			c, err := r.concurrency(jobPrefix("concurrency", n.id), "job-concurrency", n.def.Concurrency, env)
			if err != nil {
				r.failNode(n, err)
				return
			}
			concurrency = append(concurrency, CallConcurrency{key, c})
		}
		if n.def.EmbeddedConcurrency != nil {
			c, err := r.concurrency("Error when evaluating 'concurrency'.", "workflow-concurrency", n.def.EmbeddedConcurrency, r.env(inputs, nil))
			if err != nil {
				r.failNode(n, err)
				return
			}
			concurrency = append(concurrency, CallConcurrency{key, c})
		}
		pass := secretsPass{inherit: n.def.InheritSecrets, values: n.def.SecretValues, env: env, files: r.files}
		children = append(children, &scope{
			key:         key,
			name:        legName(n, name),
			inputs:      inputs,
			passes:      append(slices.Clip(n.scope.passes), pass),
			concurrency: concurrency,
		})
	}
	for i, c := range children {
		child := r.newScope(c.key, c.name, n.def.Jobs, c.inputs, c.passes, c.concurrency, n)
		child.held = st.maxParallel > 0 && i >= st.maxParallel
		n.children = append(n.children, child)
	}
}

// inputTypeDefinitions are the schema definitions that check the input values
// of a reusable workflow by their declared type.
var inputTypeDefinitions = map[string]string{
	"boolean": "boolean-needs-context",
	"number":  "number-needs-context",
	"string":  "string-needs-context",
}

// callInputs evaluates the inputs a calling job passes, with the defaults of
// the inputs it does not pass, as GitHub's ConvertToWorkflowJobInputs does.
func (r *Run) callInputs(n *node, env *expr.Env) (*expr.Object, error) {
	inputs := expr.NewObject()
	if n.def.InputDefinitions == nil {
		return inputs, nil
	}
	err := r.evaluate("", "workflow-call-inputs", n.def.InputDefinitions, env, func(ctx *template.Context, defs *template.Token) error {
		var values *template.Token
		if n.def.InputValues != nil {
			if values = ctx.Expand("workflow-job-with", n.def.InputValues, env); values == nil || len(ctx.Errors()) > 0 {
				return nil
			}
		}
		for _, p := range defs.Map {
			name := p.Key.Str
			typ := get(p.Value, "type")
			if typ == nil {
				continue
			}
			def, ok := inputTypeDefinitions[typ.Str]
			if !ok {
				return errors.New("Unexpected defined type '" + typ.Str + "' when converting input value for '" + name + "'")
			}
			value := get(p.Value, "default")
			if given := get(values, name); given != nil {
				value = given
			}
			if value == nil {
				inputs.Set(name, zeroInput(typ.Str))
				continue
			}
			if v := ctx.Evaluate(def, value); v != nil {
				inputs.Set(name, v.Value())
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inputs, nil
}

func zeroInput(typ string) any {
	switch typ {
	case "boolean":
		return false
	case "number":
		return 0.0
	}
	return ""
}

// get returns the value of the first entry of a mapping whose key equals
// name, ignoring case.
func get(m *template.Token, name string) *template.Token {
	if m == nil {
		return nil
	}
	for _, p := range m.Map {
		if p.Key.Kind == template.String && strings.EqualFold(p.Key.Str, name) {
			return p.Value
		}
	}
	return nil
}

// concludeLegs concludes a regular job once all its matrix jobs finished.
func (r *Run) concludeLegs(n *node) {
	statuses := make([]Status, len(n.legs))
	for i, j := range n.legs {
		statuses[i] = j.Status
		if j.Status == StatusFailure && j.ContinueOnError {
			statuses[i] = StatusSuccess
		}
	}
	legs := slices.Clone(n.legs)
	sort.SliceStable(legs, func(a, b int) bool { return legs[a].seq < legs[b].seq })
	n.outputs = expr.NewObject()
	for _, j := range legs {
		for _, k := range slices.Sorted(maps.Keys(j.Outputs)) {
			n.outputs.Set(k, j.Outputs[k])
		}
	}
	n.done, n.result = true, aggregate(statuses)
}

// concludeCalls concludes a calling job once all its called workflows
// finished. As GitHub documents, each output is the value set by the last
// called workflow to succeed that set it to a non-empty value.
func (r *Run) concludeCalls(n *node) {
	statuses := make([]Status, len(n.children))
	for i, c := range n.children {
		statuses[i] = c.result
	}
	children := slices.Clone(n.children)
	sort.SliceStable(children, func(a, b int) bool { return children[a].seq < children[b].seq })
	n.outputs = expr.NewObject()
	for _, c := range children {
		if c.result != StatusSuccess {
			continue
		}
		for _, k := range c.outputs.Keys() {
			if v, _ := c.outputs.Get(k); v != "" {
				n.outputs.Set(k, v)
			}
		}
	}
	n.done, n.result = true, aggregate(statuses)
}

// finishScope concludes a workflow instance once all its jobs finished, and
// evaluates the outputs of a called workflow.
func (r *Run) finishScope(s *scope) {
	statuses := make([]Status, len(s.nodes))
	jobs := expr.NewObject()
	for i, n := range s.nodes {
		statuses[i] = n.result
		v := expr.NewObject()
		v.Set("result", string(n.result))
		v.Set("outputs", orEmpty(n.outputs))
		jobs.Set(n.id, v)
	}
	s.done, s.result = true, aggregate(statuses)
	r.seq++
	s.seq = r.seq
	if s.caller != nil {
		r.changes.Calls = append(r.changes.Calls, s.key)
	}
	s.outputs = expr.NewObject()
	if s.caller == nil {
		return
	}

	if outputs := s.caller.def.Outputs; outputs != nil {
		env := r.env(s.inputs, map[string]any{"jobs": jobs})
		err := r.evaluate("", "workflow-call-outputs", outputs, env, func(_ *template.Context, t *template.Token) error {
			for _, p := range t.Map {
				v := get(p.Value, "value")
				if v == nil {
					continue
				}
				if err := v.AssertString("workfow job output value"); err != nil {
					return err
				}
				s.outputs.Set(p.Key.Str, v.Str)
			}
			return nil
		})
		if err != nil {
			j := &Job{Key: s.key + "(outputs)", Name: s.name + " / outputs", Status: StatusQueued}
			r.register(j, s.caller)
			j.Error = err.Error()
			r.finish(j, StatusFailure)
			s.result = StatusFailure
		}
	}
	if s.result == StatusFailure && s.caller.failFast {
		for _, sibling := range s.caller.children {
			if sibling != s && !sibling.done {
				r.cancelScope(sibling)
			}
		}
	}
	// Like GitHub, start the next called workflow in matrix order once one
	// finishes.
	for _, sibling := range s.caller.children {
		if sibling.held {
			sibling.held = false
			break
		}
	}
}

// aggregate combines the results of several jobs: any failure fails, then
// any cancellation cancels, and only all skipped jobs skip.
func aggregate(statuses []Status) Status {
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

// readsContext reports whether any expression in a token reads a context.
func readsContext(t *template.Token, name string) bool {
	found := false
	t.Traverse(func(x *template.Token) bool {
		if x.Kind == template.BasicExpression {
			if e, err := expr.Parse(x.Expr); err == nil && slices.Contains(e.NamedValues(), name) {
				found = true
			}
		}
		return !found
	})
	return found
}

// Secrets returns the secrets of a job, given the secrets of the repository
// that the run belongs to. Jobs of called workflows receive only the secrets
// their callers pass, with `secrets: inherit` passing all of them. Like
// GitHub, a secret whose value is empty is not passed.
func (j *Job) Secrets(repo map[string]string) (map[string]string, error) {
	current := repo
	for _, pass := range j.passes {
		if pass.inherit {
			continue
		}
		next := map[string]string{}
		if pass.values != nil {
			secrets := expr.NewObject()
			for _, k := range slices.Sorted(maps.Keys(current)) {
				secrets.Set(k, current[k])
			}
			env := &expr.Env{Contexts: maps.Clone(pass.env.Contexts)}
			env.Contexts["secrets"] = secrets
			err := evaluate(pass.files, "", "workflow-job-secrets", pass.values, env, func(_ *template.Context, t *template.Token) error {
				if err := t.AssertMapping("workflow job secrets"); err != nil {
					return err
				}
				for _, p := range t.Map {
					if err := p.Key.AssertString("workflow job secret key"); err != nil {
						return err
					}
					if err := p.Value.AssertString("workflow job secret value"); err != nil {
						return err
					}
					if p.Value.Str != "" {
						next[p.Key.Str] = p.Value.Str
					}
				}
				return nil
			})
			if err != nil {
				return nil, err
			}
		}
		current = next
	}
	return current, nil
}
