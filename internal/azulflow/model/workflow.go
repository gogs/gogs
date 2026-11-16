// Package model converts a workflow file into GitHub's workflow template: the
// jobs, steps, and settings of a workflow, with the values that are only
// known at run time kept as template tokens. It is a port of the converters
// of GitHub's workflow parsers, and produces the same template and errors.
package model

import (
	"slices"

	"gogs.io/gogs/internal/azulflow/template"
)

// Workflow is a workflow template. When Errors is not empty, the workflow is
// not valid and only FileTable is set.
type Workflow struct {
	// Name and RunName are the workflow name and run-name. GitHub's template
	// leaves them out, so JSON does too.
	Name    string
	RunName *template.Token
	// InputTypes are the types of the workflow_dispatch inputs.
	InputTypes  []InputType
	Events      *Events
	Env         *template.Token
	Permissions Permissions
	Defaults    *template.Token
	Concurrency *template.Token
	Jobs        []*Job
	// FileTable are the files read, indexed by token file ID minus one.
	FileTable []string
	Errors    []string
}

// InputType is the type of a workflow_dispatch input.
type InputType struct {
	Name, Type string
}

// JobType is the kind of a job.
type JobType string

const (
	// JobTypeJob is a job that runs steps on a runner.
	JobTypeJob JobType = "job"
	// JobTypeReusable is a job that calls a reusable workflow.
	JobTypeReusable JobType = "reusableWorkflowJob"
)

// Job is a job of a workflow. The fields after Snapshot are only used by
// reusable workflow jobs.
type Job struct {
	Type  JobType
	ID    *template.Token
	Name  *template.Token
	Needs []*template.Token
	// If is the condition, always with a status function.
	If                   *template.Token
	Strategy             *template.Token
	ContinueOnError      *template.Token
	TimeoutMinutes       *template.Token
	CancelTimeoutMinutes *template.Token
	Concurrency          *template.Token
	Permissions          Permissions
	Env                  *template.Token
	Environment          *template.Token
	Defaults             *template.Token
	RunsOn               *template.Token
	Container            *template.Token
	Services             *template.Token
	Outputs              *template.Token
	Steps                []*Step
	Snapshot             *template.Token

	// Ref is the reusable workflow, "{owner}/{repo}/{path}@{ref}" or
	// "./{path}".
	Ref               *template.Token
	InputDefinitions  *template.Token
	InputValues       *template.Token
	SecretDefinitions *template.Token
	SecretValues      *template.Token
	InheritSecrets    bool
	// EmbeddedConcurrency is the concurrency of the reusable workflow.
	EmbeddedConcurrency *template.Token
	// Jobs are the jobs of the reusable workflow, empty when it was not
	// loaded.
	Jobs []*Job
}

// Step is a step of a job. It runs a command (Run), runs an action (Uses), or
// with background steps, controls background steps (Wait, WaitAll, Cancel,
// Parallel).
type Step struct {
	ID               string
	Name             *template.Token
	If               *template.Token
	ContinueOnError  *template.Token
	TimeoutMinutes   *template.Token
	Env              *template.Token
	WorkingDirectory *template.Token
	Shell            *template.Token
	Run              *template.Token
	Uses             *template.Token
	With             *template.Token
	Background       *bool
	Wait             []*template.Token
	WaitAll          *bool
	Cancel           *template.Token
	Parallel         []*Step
}

// Clone returns a deep copy of the workflow. With omitSource, the copy has no
// source positions and no file table.
func (w *Workflow) Clone(omitSource bool) *Workflow {
	c := *w
	c.InputTypes = slices.Clone(w.InputTypes)
	c.Events = w.Events
	c.RunName = cloneToken(w.RunName, omitSource)
	c.Env = cloneToken(w.Env, omitSource)
	c.Permissions = w.Permissions.clone()
	c.Defaults = cloneToken(w.Defaults, omitSource)
	c.Concurrency = cloneToken(w.Concurrency, omitSource)
	c.Jobs = cloneJobs(w.Jobs, omitSource)
	c.FileTable = slices.Clone(w.FileTable)
	if omitSource {
		c.FileTable = nil
	}
	c.Errors = slices.Clone(w.Errors)
	return &c
}

func cloneToken(t *template.Token, omitSource bool) *template.Token {
	if t == nil {
		return nil
	}
	return t.Clone(omitSource)
}

func cloneTokens(ts []*template.Token, omitSource bool) []*template.Token {
	if ts == nil {
		return nil
	}
	out := make([]*template.Token, len(ts))
	for i, t := range ts {
		out[i] = t.Clone(omitSource)
	}
	return out
}

func cloneJobs(jobs []*Job, omitSource bool) []*Job {
	if jobs == nil {
		return nil
	}
	out := make([]*Job, len(jobs))
	for i, j := range jobs {
		c := *j
		for _, f := range []**template.Token{
			&c.ID, &c.Name, &c.If, &c.Strategy, &c.ContinueOnError, &c.TimeoutMinutes,
			&c.CancelTimeoutMinutes, &c.Concurrency, &c.Env, &c.Environment, &c.Defaults,
			&c.RunsOn, &c.Container, &c.Services, &c.Outputs, &c.Snapshot, &c.Ref,
			&c.InputDefinitions, &c.InputValues, &c.SecretDefinitions, &c.SecretValues,
			&c.EmbeddedConcurrency,
		} {
			*f = cloneToken(*f, omitSource)
		}
		c.Needs = cloneTokens(j.Needs, omitSource)
		c.Permissions = j.Permissions.clone()
		c.Steps = cloneSteps(j.Steps, omitSource)
		c.Jobs = cloneJobs(j.Jobs, omitSource)
		if j.Jobs != nil && c.Jobs == nil {
			c.Jobs = []*Job{}
		}
		out[i] = &c
	}
	return out
}

func cloneSteps(steps []*Step, omitSource bool) []*Step {
	if steps == nil {
		return nil
	}
	out := make([]*Step, len(steps))
	for i, s := range steps {
		c := *s
		for _, f := range []**template.Token{
			&c.Name, &c.If, &c.ContinueOnError, &c.TimeoutMinutes, &c.Env,
			&c.WorkingDirectory, &c.Shell, &c.Run, &c.Uses, &c.With, &c.Cancel,
		} {
			*f = cloneToken(*f, omitSource)
		}
		c.Wait = cloneTokens(s.Wait, omitSource)
		c.Parallel = cloneSteps(s.Parallel, omitSource)
		out[i] = &c
	}
	return out
}

// JSON returns the workflow in the JSON form of GitHub's workflow template.
// Without includeSource, the source positions and the file table are left
// out.
func (w *Workflow) JSON(includeSource bool) string {
	if !includeSource {
		w = w.Clone(true)
	}
	o := &jsonObject{}
	if w.Events != nil {
		o.set("events", w.Events.json())
	}
	if len(w.InputTypes) > 0 {
		types := &jsonObject{}
		for _, t := range w.InputTypes {
			types.set(t.Name, t.Type)
		}
		o.set("input-types", types)
	}
	setToken(o, "env", w.Env)
	if w.Permissions != nil {
		o.set("permissions", w.Permissions.json())
	}
	setToken(o, "defaults", w.Defaults)
	setToken(o, "concurrency", w.Concurrency)
	if len(w.Jobs) > 0 {
		o.set("jobs", jobsJSON(w.Jobs))
	}
	if len(w.FileTable) > 0 {
		o.set("file-table", stringsJSON(w.FileTable))
	}
	if len(w.Errors) > 0 {
		errs := make([]jsonValue, len(w.Errors))
		for i, msg := range w.Errors {
			e := &jsonObject{}
			e.set("Message", msg)
			errs[i] = e
		}
		o.set("errors", errs)
	}
	return writeJSON(o)
}

func setToken(o *jsonObject, key string, t *template.Token) {
	if t != nil {
		o.set(key, tokenJSON(t))
	}
}

func tokensJSON(ts []*template.Token) []jsonValue {
	out := make([]jsonValue, len(ts))
	for i, t := range ts {
		out[i] = tokenJSON(t)
	}
	return out
}

func jobsJSON(jobs []*Job) []jsonValue {
	out := make([]jsonValue, len(jobs))
	for i, j := range jobs {
		out[i] = j.json()
	}
	return out
}

func (j *Job) json() jsonValue {
	o := &jsonObject{}
	o.set("type", string(j.Type))
	setToken(o, "id", j.ID)
	setToken(o, "name", j.Name)
	// Unlike other jobs, a reusable workflow job always has needs.
	if len(j.Needs) > 0 || j.Type == JobTypeReusable {
		o.set("needs", tokensJSON(j.Needs))
	}
	setToken(o, "if", j.If)
	if j.Type == JobTypeReusable {
		setToken(o, "ref", j.Ref)
		if j.Permissions != nil {
			o.set("permissions", j.Permissions.json())
		}
		setToken(o, "input-definitions", j.InputDefinitions)
		setToken(o, "input-values", j.InputValues)
		setToken(o, "secret-definitions", j.SecretDefinitions)
		setToken(o, "secret-values", j.SecretValues)
		if j.InheritSecrets {
			o.set("inherit-secrets", true)
		}
		setToken(o, "outputs", j.Outputs)
		setToken(o, "defaults", j.Defaults)
		setToken(o, "env", j.Env)
		setToken(o, "concurrency", j.Concurrency)
		setToken(o, "embedded-concurrency", j.EmbeddedConcurrency)
		setToken(o, "strategy", j.Strategy)
		jobs := j.Jobs
		if jobs == nil {
			jobs = []*Job{}
		}
		o.set("jobs", jobsJSON(jobs))
		return o
	}
	setToken(o, "strategy", j.Strategy)
	setToken(o, "continue-on-error", j.ContinueOnError)
	setToken(o, "timeout-minutes", j.TimeoutMinutes)
	setToken(o, "cancel-timeout-minutes", j.CancelTimeoutMinutes)
	setToken(o, "concurrency", j.Concurrency)
	if j.Permissions != nil {
		o.set("permissions", j.Permissions.json())
	}
	setToken(o, "env", j.Env)
	setToken(o, "environment", j.Environment)
	setToken(o, "defaults", j.Defaults)
	setToken(o, "runs-on", j.RunsOn)
	setToken(o, "container", j.Container)
	setToken(o, "services", j.Services)
	setToken(o, "outputs", j.Outputs)
	if len(j.Steps) > 0 {
		o.set("steps", stepsJSON(j.Steps))
	}
	setToken(o, "snapshot", j.Snapshot)
	return o
}

func stepsJSON(steps []*Step) []jsonValue {
	out := make([]jsonValue, len(steps))
	for i, s := range steps {
		out[i] = s.json()
	}
	return out
}

func (s *Step) json() jsonValue {
	o := &jsonObject{}
	if s.ID != "" {
		o.set("id", s.ID)
	}
	setToken(o, "name", s.Name)
	setToken(o, "if", s.If)
	setToken(o, "continue-on-error", s.ContinueOnError)
	setToken(o, "timeout-minutes", s.TimeoutMinutes)
	setToken(o, "env", s.Env)
	setToken(o, "working-directory", s.WorkingDirectory)
	setToken(o, "shell", s.Shell)
	setToken(o, "run", s.Run)
	setToken(o, "uses", s.Uses)
	setToken(o, "with", s.With)
	if s.Background != nil {
		o.set("background", *s.Background)
	}
	if s.Wait != nil {
		o.set("wait", tokensJSON(s.Wait))
	}
	if s.WaitAll != nil {
		o.set("wait-all", *s.WaitAll)
	}
	setToken(o, "cancel", s.Cancel)
	if s.Parallel != nil {
		o.set("parallel", stepsJSON(s.Parallel))
	}
	return o
}
