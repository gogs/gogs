package model

import (
	"slices"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/expr"
	"gogs.io/gogs/internal/azulflow/template"
)

// Permission policies, which limit the permissions a reusable workflow may
// request when its caller does not declare any.
const (
	PolicyLimitedRead = "LimitedRead"
	PolicyWrite       = "Write"
)

// FileProvider reads the workflow files a workflow refers to.
type FileProvider interface {
	// ReadFile returns the content of a file: the workflow being loaded, or
	// a reusable workflow "{owner}/{repo}/{path}@{ref}".
	ReadFile(name string) (string, error)
	// ResolveRef returns the full name "{owner}/{repo}/{path}@{ref}" of a
	// reusable workflow in the same repository called from the file caller:
	// "./{path}" is at the same commit as the caller, and "$/{path}" is in
	// the repository of the run at the commit of the run.
	ResolveRef(caller, ref string) (string, error)
}

// Options are the limits and settings of loading a workflow.
type Options struct {
	Limits template.Limits
	// AllowAnchors allows YAML anchors and aliases.
	AllowAnchors bool
	// BackgroundSteps enables background steps, which GitHub puts behind a
	// feature flag that it enables for its server.
	BackgroundSteps bool
	// StrictEvents rejects unknown events and event properties, which
	// GitHub's server accepts and its editor tooling reports.
	StrictEvents bool
	// MaxFileSize is the most characters a file may have.
	MaxFileSize int
	// MaxFiles is the most files that may be read, zero for no limit.
	MaxFiles int
	// MaxJobs is the most jobs the workflow and the workflows it calls may
	// have together, zero for no limit.
	MaxJobs int
	// MaxNestedDepth is the most levels of reusable workflows that may be
	// called, where 1 means a workflow may call reusable workflows that call
	// none.
	MaxNestedDepth int
	// SkipReusableWorkflows leaves out the jobs of reusable workflows.
	SkipReusableWorkflows bool
	// PermissionsPolicy is the default permission policy of the repository,
	// PolicyLimitedRead or PolicyWrite.
	PermissionsPolicy string
	// Scopes are the permission scopes that "read-all" and "write-all"
	// grant.
	Scopes []string
}

// DefaultOptions returns the options GitHub uses.
func DefaultOptions() Options {
	return Options{
		Limits:          template.DefaultLimits(),
		AllowAnchors:    true,
		BackgroundSteps: true,
		MaxFileSize:     1024 * 1024,
		MaxFiles:        51,
		// GitHub connects at most ten levels: the caller and nine levels of
		// reusable workflows.
		MaxNestedDepth:    9,
		PermissionsPolicy: PolicyLimitedRead,
		Scopes:            DefaultScopes,
	}
}

// ErrReferencedWorkflowNotFound is returned when ResolveRef fails.
var ErrReferencedWorkflowNotFound = errors.New("referenced workflow not found")

// Load reads and converts the workflow file name and the reusable workflows
// it calls. Problems with the workflow are reported in Workflow.Errors. The
// error is only for a local reusable workflow that cannot be resolved.
func Load(files FileProvider, name string, opts Options) (*Workflow, error) {
	ctx := template.NewContext(template.WorkflowSchema(), opts.Limits)
	ctx.BackgroundSteps = opts.BackgroundSteps
	c := &converter{ctx: ctx, opts: opts, files: files, cache: map[string]string{}, referenced: map[string]bool{}}

	root := c.loadFile(name)
	w := c.workflow(root)
	if err := c.loadReusableWorkflows(w); err != nil {
		return nil, err
	}
	if !c.failed() && opts.MaxJobs > 0 && c.jobCount > opts.MaxJobs {
		ctx.AddError("The workflow may not contain more than " + strconv.Itoa(opts.MaxJobs) + " jobs")
	}
	if errs := ctx.Errors(); len(errs) > 0 {
		return &Workflow{Errors: slices.Clone(errs), FileTable: slices.Clone(ctx.Files())}, nil
	}
	w.FileTable = slices.Clone(ctx.Files())
	return w, nil
}

type converter struct {
	ctx        *template.Context
	opts       Options
	files      FileProvider
	cache      map[string]string
	referenced map[string]bool
	jobCount   int
}

// convertError stops the conversion, as an exception does in GitHub's
// parser. Its message is reported without a position.
type convertError struct {
	msg string
}

func throw(msg string) {
	panic(convertError{msg})
}

func mustOK(err error) {
	if err != nil {
		throw(err.Error())
	}
}

func throwUnexpectedValue(what string, t *template.Token) {
	throw("Error while reading '" + what + "'. Unexpected value '" + t.String() + "'")
}

// catch reports a convertError that stops the function it is deferred in.
func (c *converter) catch() {
	if r := recover(); r != nil {
		e, ok := r.(convertError)
		if !ok {
			panic(r)
		}
		c.ctx.AddError(e.msg)
	}
}

func (c *converter) failed() bool {
	return len(c.ctx.Errors()) > 0
}

func lineCol(t *template.Token) string {
	return "(Line: " + strconv.Itoa(t.Line) + ", Col: " + strconv.Itoa(t.Col) + ")"
}

func (c *converter) loadFile(name string) (result *template.Token) {
	defer c.catch()

	key := strings.ToLower(name)
	c.referenced[key] = true
	if c.opts.MaxFiles > 0 && len(c.referenced) > c.opts.MaxFiles {
		throw("The maximum file count of " + strconv.Itoa(c.opts.MaxFiles) + " has been exceeded")
	}
	id := c.ctx.FileID(name)
	content, ok := c.cache[key]
	if !ok {
		var err error
		content, err = c.files.ReadFile(name)
		if err != nil {
			throw(err.Error())
		}
		if utf16Len(content) > c.opts.MaxFileSize {
			throw(name + ": The maximum file size of " + strconv.Itoa(c.opts.MaxFileSize) + " characters has been exceeded")
		}
		c.cache[key] = content
	}
	root := template.WorkflowRoot
	if c.opts.StrictEvents {
		root = template.WorkflowRootStrict
	}
	return template.Read(c.ctx, root, id, []byte(content), c.opts.AllowAnchors)
}

func (c *converter) workflow(root *template.Token) *Workflow {
	w := &Workflow{}
	if root == nil || c.failed() {
		return w
	}
	defer c.catch()

	mustOK(root.AssertMapping("root"))
	for _, p := range root.Map {
		mustOK(p.Key.AssertString("root key"))
		switch p.Key.Str {
		case "on":
			w.InputTypes = dispatchInputTypes(p.Value)
			w.Events = c.events(p.Value)
		case "description":
		case "name":
			w.Name = p.Value.String()
		case "run-name":
			w.RunName = p.Value
		case "defaults":
			w.Defaults = p.Value
		case "env":
			w.Env = p.Value
		case "concurrency":
			c.concurrency(p.Value)
			w.Concurrency = p.Value
		case "jobs":
			w.Jobs = c.jobs(p.Value)
		case "permissions":
			w.Permissions = c.permissions(p.Value)
		default:
			throwUnexpectedValue("root key", p.Key)
		}
	}
	if w.Permissions != nil {
		for _, j := range w.Jobs {
			if j.Permissions == nil {
				j.Permissions = w.Permissions
			}
		}
	}
	return w
}

func dispatchInputTypes(on *template.Token) []InputType {
	if on.Kind != template.Mapping {
		return nil
	}
	dispatch := on.Get("workflow_dispatch")
	if dispatch == nil || dispatch.Kind == template.Null {
		return nil
	}
	mustOK(dispatch.AssertMapping("workflow on value workflow_dispatch"))
	inputs := dispatch.Get("inputs")
	if inputs == nil || inputs.Kind == template.Null {
		return nil
	}
	mustOK(inputs.AssertMapping("on-workflow_dispatch-inputs"))
	var result []InputType
	for _, p := range inputs.Map {
		mustOK(p.Key.AssertString("inputs key"))
		typ := "string"
		if p.Value.Kind != template.Null {
			mustOK(p.Value.AssertMapping("input " + p.Key.Str))
			for _, q := range p.Value.Map {
				if strings.EqualFold(q.Key.Str, "type") && q.Value.Kind == template.String {
					typ = q.Value.Str
				}
			}
		}
		result = append(result, InputType{Name: p.Key.Str, Type: typ})
	}
	return result
}

func successExpr(t *template.Token) *template.Token {
	e := template.NewExpression("success()")
	if t != nil {
		e.File, e.Line, e.Col = t.File, t.Line, t.Col
	}
	return e
}

func (c *converter) jobs(t *template.Token) []*Job {
	mustOK(t.AssertMapping("jobs"))
	type node struct {
		name  string
		needs []*template.Token
	}
	var result []*Job
	var ready, unsatisfied []*node
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("jobs key"))
		c.jobCount++
		mustOK(p.Value.AssertMapping("jobs value"))
		j := c.job(p.Key, p.Value)
		result = append(result, j)
		n := &node{name: j.ID.Str, needs: slices.Clone(j.Needs)}
		if len(n.needs) == 0 {
			ready = append(ready, n)
		} else {
			unsatisfied = append(unsatisfied, n)
		}
	}
	if c.failed() {
		return result
	}
	if len(ready) == 0 {
		c.ctx.Error(t, "The workflow must contain at least one job with no dependencies.")
		return result
	}

	for len(ready) > 0 {
		current := ready[0]
		ready = ready[1:]
		for i := len(unsatisfied) - 1; i >= 0; i-- {
			n := unsatisfied[i]
			for k := len(n.needs) - 1; k >= 0; k-- {
				if strings.EqualFold(n.needs[k].Str, current.name) {
					n.needs = slices.Delete(n.needs, k, k+1)
					if len(n.needs) == 0 {
						ready = append(ready, n)
						unsatisfied = slices.Delete(unsatisfied, i, i+1)
					}
				}
			}
		}
	}

	for _, n := range unsatisfied {
		for _, need := range n.needs {
			known := slices.ContainsFunc(result, func(j *Job) bool { return strings.EqualFold(j.ID.Str, need.Str) })
			if known {
				c.ctx.Error(need, "Job '"+n.name+"' depends on job '"+need.Str+"' which creates a cycle in the dependency graph.")
			} else {
				c.ctx.Error(need, "Job '"+n.name+"' depends on unknown job '"+need.Str+"'.")
			}
		}
	}
	return result
}

func earlyBoolean(t *template.Token, what string) {
	if !t.IsExpression() {
		mustOK(t.AssertBoolean(what))
	}
}

func earlyNumber(t *template.Token, what string) {
	if !t.IsExpression() {
		mustOK(t.AssertNumber(what))
	}
}

func (c *converter) job(id, def *template.Token) *Job {
	if msg := (&idBuilder{}).addKnownID(id.Str); msg != "" {
		c.ctx.Error(id, msg)
	}

	j := &Job{ID: id, If: successExpr(nil)}
	var ref, with, secrets *template.Token
	inherit := false
	for _, p := range def.Map {
		mustOK(p.Key.AssertString("job property name"))
		v := p.Value
		switch p.Key.Str {
		case "continue-on-error":
			earlyBoolean(v, "job continue-on-error")
			mustOK(v.AssertScalar("job continue-on-error"))
			j.ContinueOnError = v
		case "if":
			j.If = c.ifCondition(v, ifJob)
		case "name":
			mustOK(v.AssertScalar("job name"))
			if !v.IsExpression() {
				mustOK(v.AssertString("job name"))
			}
			j.Name = v
		case "needs":
			if v.Kind == template.String {
				j.Needs = append(j.Needs, v)
			} else {
				mustOK(v.AssertSequence("job needs"))
				for _, item := range v.Seq {
					mustOK(item.AssertString("job needs item"))
					j.Needs = append(j.Needs, item)
				}
			}
		case "runs-on":
			c.runsOn(v)
			j.RunsOn = v
		case "snapshot":
			c.snapshot(v)
			j.Snapshot = v
		case "steps":
			j.Steps = c.steps(v)
		case "strategy":
			c.strategy(v)
			j.Strategy = v
		case "timeout-minutes":
			earlyNumber(v, "job timeout-minutes")
			j.TimeoutMinutes = v
		case "cancel-timeout-minutes":
			earlyNumber(v, "job cancel-timeout-minutes")
			j.CancelTimeoutMinutes = v
		case "container":
			c.container(v, false)
			j.Container = v
		case "services":
			c.services(v)
			j.Services = v
		case "concurrency":
			c.concurrency(v)
			j.Concurrency = v
		case "env":
			j.Env = v
		case "environment":
			c.environment(v)
			j.Environment = v
		case "outputs":
			j.Outputs = v
		case "defaults":
			j.Defaults = v
		case "permissions":
			j.Permissions = c.permissions(v)
		case "uses":
			mustOK(v.AssertString("job uses value"))
			ref = v
		case "with":
			mustOK(v.AssertMapping("uses-with value"))
			with = v
		case "secrets":
			if v.Kind == template.String && v.Str == "inherit" {
				inherit = true
			} else {
				mustOK(v.AssertMapping("uses-secrets value"))
				secrets = v
			}
		default:
			throwUnexpectedValue("job key", p.Key)
		}
	}
	if j.Name == nil || (j.Name.Kind == template.String && j.Name.Str == "") {
		j.Name = j.ID
	}

	if ref == nil {
		j.Type = JobTypeJob
		return j
	}
	return &Job{
		Type:           JobTypeReusable,
		ID:             j.ID,
		Name:           j.Name,
		Needs:          j.Needs,
		If:             j.If,
		Ref:            ref,
		InputValues:    with,
		SecretValues:   secrets,
		InheritSecrets: inherit,
		Permissions:    j.Permissions,
		Concurrency:    j.Concurrency,
		Strategy:       j.Strategy,
	}
}

type ifKind int

const (
	ifJob ifKind = iota
	ifStep
	ifSnapshot
)

var (
	jobIfOptions = expr.ParseOptions{
		Contexts: []string{"github", "vars", "inputs", "needs"},
		Funcs: []expr.FuncSig{
			{Name: "always"}, {Name: "failure", Max: -1}, {Name: "cancelled"}, {Name: "success", Max: -1},
		},
	}
	stepIfOptions = expr.ParseOptions{
		Contexts: []string{"github", "vars", "inputs", "needs", "strategy", "matrix", "steps", "job", "runner", "env"},
		Funcs: []expr.FuncSig{
			{Name: "always"}, {Name: "cancelled"}, {Name: "failure"}, {Name: "success"}, {Name: "hashFiles", Min: 1, Max: 255},
		},
	}
)

var statusFunctions = []string{"always", "cancelled", "failure", "success"}

// ifCondition returns the condition of an if, which defaults to success()
// and is combined with success() when it calls no status function. It
// returns nil when the condition is not valid.
func (c *converter) ifCondition(t *template.Token, kind ifKind) *template.Token {
	var cond string
	switch {
	case t == nil:
	case t.Kind == template.BasicExpression:
		cond = t.Expr
	default:
		what := map[ifKind]string{ifJob: "Job if", ifStep: "Step if", ifSnapshot: "Snapshot if"}[kind]
		mustOK(t.AssertString(what))
		cond = t.Str
	}
	if strings.TrimSpace(cond) == "" {
		return successExpr(t)
	}

	opts := stepIfOptions
	if kind == ifJob {
		opts = jobIfOptions
	}
	e, err := expr.ParseWith(cond, opts)
	if err != nil {
		c.ctx.Error(t, err.Error())
		return nil
	}
	if !slices.ContainsFunc(e.Functions(), func(f string) bool { return slices.Contains(statusFunctions, f) }) {
		cond = "success() && (" + cond + ")"
	}
	result := successExpr(t)
	result.Expr = cond
	return result
}

func (c *converter) concurrency(t *template.Token) {
	if t.IsExpression() {
		return
	}
	group := ""
	if t.Kind == template.String {
		group = t.Str
	} else {
		mustOK(t.AssertMapping("concurrency"))
		for _, p := range t.Map {
			mustOK(p.Key.AssertString("concurrency key"))
			if p.Value.IsExpression() {
				continue
			}
			switch p.Key.Str {
			case "group":
				mustOK(p.Value.AssertString("group key"))
				group = p.Value.Str
			case "cancel-in-progress":
				mustOK(p.Value.AssertBoolean("cancel-in-progress key"))
			case "queue":
				mustOK(p.Value.AssertString("queue key"))
			default:
				throwUnexpectedValue("concurrency key", p.Key)
			}
		}
	}
	if utf16Len(group) > 400 {
		c.ctx.Error(t, "Concurrency group name must be less than 400 characters")
	}
}

func (c *converter) environment(t *template.Token) {
	if t.IsExpression() || t.Kind == template.String {
		return
	}
	mustOK(t.AssertMapping("job environment"))
	url := t.Get("url")
	hasExpression := false
	t.Traverse(func(x *template.Token) bool {
		if x.IsExpression() && x != url {
			hasExpression = true
		}
		return true
	})
	if hasExpression {
		return
	}
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("job environment key"))
		switch p.Key.Str {
		case "name":
			mustOK(p.Value.AssertString("job environment name"))
		case "url":
		case "deployment":
			mustOK(p.Value.AssertBoolean("job environment deployment"))
		default:
			throwUnexpectedValue("job environment key", p.Key)
		}
	}
}

func (c *converter) runsOn(t *template.Token) {
	c.runsOnLabels(t)
	if t.Kind != template.Mapping {
		return
	}
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("job runs-on property name"))
		switch p.Key.Str {
		case "group":
			if p.Value.IsExpression() {
				continue
			}
			mustOK(p.Value.AssertString("job runs-on group name"))
			name := p.Value.Str
			names := strings.Split(name, "/")
			invalidPrefix := "Invalid runs-on group name '" + name + "'. Please use 'organization/' or 'enterprise/' prefix to target a single runner group."
			switch {
			case len(names) == 2 && names[1] == "":
				c.ctx.Error(p.Value, "Invalid runs-on group name '"+name+"'.")
			case len(names) == 2 && !slices.Contains([]string{"org", "organization", "ent", "enterprise"}, names[0]):
				c.ctx.Error(p.Value, invalidPrefix)
			case len(names) > 2:
				c.ctx.Error(p.Value, invalidPrefix)
			}
		case "labels":
			c.runsOnLabels(p.Value)
		}
	}
}

func (c *converter) runsOnLabels(t *template.Token) {
	if t.Kind != template.Sequence {
		return
	}
	for _, item := range t.Seq {
		if !item.IsExpression() {
			mustOK(item.AssertString("job runs-on labels sequence item"))
		}
	}
}

func (c *converter) snapshot(t *template.Token) {
	if t.IsExpression() {
		return
	}
	hasImageName := false
	switch t.Kind {
	case template.String:
		hasImageName = true
	case template.Mapping:
		for _, p := range t.Map {
			mustOK(p.Key.AssertString("job snapshot key"))
			v := p.Value
			switch p.Key.Str {
			case "image-name":
				if v.IsExpression() {
					return
				}
				mustOK(v.AssertString("job snapshot image-name"))
				hasImageName = true
			case "if":
				c.ifCondition(v, ifSnapshot)
			case "version":
				if v.IsExpression() {
					return
				}
				mustOK(v.AssertString("job snapshot version"))
				if !validSnapshotVersion(v.Str) {
					c.ctx.Error(t, "Expected format '{major-version}.*' Actual '"+v.Str+"'")
					return
				}
			default:
				throwUnexpectedValue("job snapshot key", p.Key)
			}
		}
	}
	if !hasImageName {
		c.ctx.Error(t, "job snapshot image-name is required.")
	}
}

func validSnapshotVersion(s string) bool {
	major, rest, ok := strings.Cut(s, ".")
	if !ok || rest != "*" || major == "" {
		return false
	}
	for _, r := range major {
		if r < '0' || r > '9' {
			return false
		}
	}
	_, err := strconv.ParseInt(major, 10, 32)
	return err == nil
}

func (c *converter) strategy(t *template.Token) {
	if t.IsExpression() {
		return
	}
	mustOK(t.AssertMapping("strategy"))
	hasExpressions := false
	var vectors []string
	var include, exclude *template.Token
	for _, p := range t.Map {
		if p.Key.IsExpression() {
			hasExpressions = true
			continue
		}
		mustOK(p.Key.AssertString("strategy key"))
		v := p.Value
		switch p.Key.Str {
		case "fail-fast":
			if v.IsExpression() {
				hasExpressions = true
				continue
			}
			mustOK(v.AssertBoolean("strategy fail-fast"))
		case "max-parallel":
			if v.IsExpression() {
				hasExpressions = true
				continue
			}
			mustOK(v.AssertNumber("strategy max-parallel"))
		case "matrix":
			if v.IsExpression() {
				hasExpressions = true
				continue
			}
			mustOK(v.AssertMapping("matrix"))
			hasExpressions = hasExpressions || v.HasExpression()
			hasVector, hasInclude := false, false
			for _, mp := range v.Map {
				if mp.Key.IsExpression() {
					hasVector = true
					continue
				}
				mustOK(mp.Key.AssertString("matrix key"))
				switch mp.Key.Str {
				case "include":
					if mp.Value.HasExpression() {
						hasInclude = true
						continue
					}
					mustOK(mp.Value.AssertSequence("matrix includes"))
					hasInclude = len(mp.Value.Seq) > 0
					include = mp.Value
				case "exclude":
					if mp.Value.HasExpression() {
						continue
					}
					mustOK(mp.Value.AssertSequence("matrix excludes"))
					exclude = mp.Value
				default:
					hasVector = true
					if mp.Value.HasExpression() {
						continue
					}
					mustOK(mp.Value.AssertSequence("matrix vector value"))
					if len(mp.Value.Seq) == 0 {
						c.ctx.Error(mp.Value, "Matrix vector '"+mp.Key.Str+"' does not contain any values")
					} else {
						vectors = append(vectors, mp.Key.Str)
					}
				}
			}
			if !hasVector && !hasInclude {
				c.ctx.Error(v, "Matrix must define at least one vector")
			}
		default:
			throwUnexpectedValue("strategy key", p.Key)
		}
	}
	if hasExpressions {
		return
	}

	isVector := func(key string) bool {
		return slices.ContainsFunc(vectors, func(v string) bool { return strings.EqualFold(v, key) })
	}
	if include != nil {
		for _, item := range include.Seq {
			mustOK(item.AssertMapping("matrix includes item"))
			for _, p := range item.Map {
				mustOK(p.Key.AssertString("matrix include item key"))
			}
			if len(item.Map) == 0 {
				c.ctx.Error(item, "Matrix include mapping does not contain any values")
			}
		}
	}
	if exclude != nil {
		for _, item := range exclude.Seq {
			mustOK(item.AssertMapping("matrix excludes item"))
			if len(item.Map) == 0 {
				c.ctx.Error(item, "Matrix exclude filter must not be empty")
				continue
			}
			for _, p := range item.Map {
				mustOK(p.Key.AssertString("matrix excludes item key"))
				if !isVector(p.Key.Str) {
					c.ctx.Error(p.Key, "Matrix exclude key '"+p.Key.Str+"' does not match any key within the matrix")
				}
			}
		}
	}
}

func (c *converter) container(t *template.Token, service bool) {
	if t.HasExpression() {
		return
	}
	const dockerPrefix = "docker://"
	image := ""
	if t.Kind == template.String {
		image = t.Str
	} else {
		mustOK(t.AssertMapping("container"))
		for _, p := range t.Map {
			mustOK(p.Key.AssertString("container key"))
			what := "container " + p.Key.Str
			v := p.Value
			switch p.Key.Str {
			case "image":
				mustOK(v.AssertString(what))
				image = v.Str
			case "env":
				mustOK(v.AssertMapping(what))
				for _, e := range v.Map {
					mustOK(e.Value.AssertString(what + " " + e.Key.String()))
				}
			case "options", "entrypoint", "command":
				mustOK(v.AssertString(what))
			case "ports", "volumes":
				mustOK(v.AssertSequence(what))
				for _, item := range v.Seq {
					mustOK(item.AssertString(what + " " + item.String()))
				}
			case "credentials":
				mustOK(v.AssertMapping("credentials"))
				for _, cp := range v.Map {
					mustOK(cp.Key.AssertString("credentials key"))
					switch cp.Key.Str {
					case "username", "password":
						mustOK(cp.Value.AssertString(cp.Key.Str))
					default:
						throwUnexpectedValue("credentials key "+cp.Key.Str, cp.Key)
					}
				}
			default:
				throwUnexpectedValue("container key", p.Key)
			}
		}
	}
	trimmed, hadPrefix := strings.CutPrefix(image, dockerPrefix)
	// An empty shorthand only fails for services, so that "container: ''"
	// means no container, but a mapping must name an image.
	if trimmed == "" && (hadPrefix || service || t.Kind == template.Mapping) {
		c.ctx.Error(t, "Container image cannot be empty")
	}
}

func (c *converter) services(t *template.Token) {
	if t.HasExpression() {
		return
	}
	mustOK(t.AssertMapping("services"))
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("services key"))
		c.container(p.Value, true)
	}
}

func (c *converter) steps(t *template.Token) []*Step {
	mustOK(t.AssertSequence("job steps"))
	ids := &idBuilder{}
	known := map[string]bool{}
	result := make([]*Step, 0, len(t.Seq))
	for _, item := range t.Seq {
		result = append(result, c.step(item, ids, known))
	}

	for _, s := range result {
		if s.ID != "" {
			continue
		}
		id := ""
		switch {
		case s.Uses != nil:
			id = actionStepID(s.Uses.Str)
		case s.Wait != nil:
			id = "wait"
		case s.WaitAll != nil:
			id = "wait-all"
		case s.Cancel != nil:
			id = "cancel"
		case s.Parallel != nil:
			id = "parallel"
		}
		if id == "" {
			id = "run"
		}
		ids.appendSegment("__" + id)
		s.ID = ids.build(true, maxIDLength)
	}
	return result
}

func actionStepID(uses string) string {
	switch {
	case strings.HasPrefix(uses, "docker://"):
		return strings.TrimPrefix(uses, "docker://")
	case strings.HasPrefix(uses, "./"), strings.HasPrefix(uses, ".\\"):
		return "self"
	case strings.HasPrefix(uses, selfRepositoryPrefix):
		return "selfRepository"
	}
	if owner, repo, ok := splitActionRef(uses); ok {
		return owner + "/" + repo
	}
	return ""
}

const selfRepositoryPrefix = "$/"

// splitActionRef splits "{owner}/{repo}[/path]@{ref}", and reports whether
// it has that form.
func splitActionRef(uses string) (owner, repo string, ok bool) {
	parts := strings.Split(uses, "@")
	if len(parts) != 2 || parts[1] == "" {
		return "", "", false
	}
	path := strings.FieldsFunc(parts[0], func(r rune) bool { return r == '/' || r == '\\' })
	if len(path) < 2 {
		return "", "", false
	}
	return path[0], path[1], true
}

func boolPtr(b bool) *bool {
	return &b
}

func (c *converter) step(t *template.Token, ids *idBuilder, known map[string]bool) *Step {
	mustOK(t.AssertMapping("steps item"))
	s := &Step{}
	var ifToken, id *template.Token
	var waitToken, cancelToken, parallel *template.Token
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("steps item key"))
		v := p.Value
		switch p.Key.Str {
		case "continue-on-error":
			earlyBoolean(v, "step continue-on-error")
			mustOK(v.AssertScalar("steps continue-on-error"))
			s.ContinueOnError = v
		case "env", "with":
			what := map[string]string{"env": "environment", "with": "inputs"}[p.Key.Str]
			if !v.IsExpression() {
				mustOK(v.AssertMapping(what))
				for _, e := range v.Map {
					if e.Key.IsExpression() {
						continue
					}
					mustOK(e.Key.AssertString(what + " key"))
					if !e.Value.IsExpression() {
						mustOK(e.Value.AssertString(what + " value"))
					}
				}
			}
			if p.Key.Str == "env" {
				s.Env = v
			} else {
				s.With = v
			}
		case "id":
			mustOK(v.AssertString("steps item id"))
			id = v
			if v.Str != "" {
				if msg := ids.addKnownID(v.Str); msg != "" {
					c.ctx.Error(v, msg)
				} else {
					known[v.Str] = true
				}
			}
		case "if":
			mustOK(v.AssertScalar("steps item if"))
			ifToken = v
		case "name":
			mustOK(v.AssertScalar("steps item name"))
			s.Name = v
		case "run":
			mustOK(v.AssertScalar("steps item run"))
			s.Run = v
		case "shell":
			mustOK(v.AssertScalar("steps item shell"))
			s.Shell = v
		case "working-directory":
			mustOK(v.AssertScalar("steps item working-directory"))
			s.WorkingDirectory = v
		case "timeout-minutes":
			earlyNumber(v, "step timeout-minutes")
			mustOK(v.AssertScalar("steps item timeout-minutes"))
			s.TimeoutMinutes = v
		case "uses":
			mustOK(v.AssertString("steps item uses"))
			s.Uses = v
		case "background":
			mustOK(v.AssertBoolean("steps item background"))
			s.Background = boolPtr(v.Bool)
		case "wait":
			waitToken = v
		case "wait-all":
			if v.Kind != template.Null {
				mustOK(v.AssertBoolean("steps item wait-all"))
				if !v.Bool {
					c.ctx.Error(v, "The value of 'wait-all' must be true or omitted")
				}
			}
			s.WaitAll = boolPtr(true)
		case "cancel":
			mustOK(v.AssertString("steps item cancel"))
			cancelToken = v
		case "parallel":
			parallel = v
		default:
			throwUnexpectedValue("steps item key", p.Key)
		}
	}
	if id != nil {
		s.ID = id.Str
	}

	// Wait and cancel refer to the steps before them, which are known by
	// now, so they are checked after the step's own ID.
	if waitToken != nil {
		if waitToken.Kind == template.String {
			s.Wait = []*template.Token{waitToken}
		} else {
			mustOK(waitToken.AssertSequence("steps item wait"))
			s.Wait = []*template.Token{}
			for _, item := range waitToken.Seq {
				mustOK(item.AssertString("steps item wait item"))
				s.Wait = append(s.Wait, item)
			}
		}
		for _, target := range s.Wait {
			c.validateTargetStep(known, target, id)
		}
	}
	if cancelToken != nil {
		s.Cancel = cancelToken
		c.validateTargetStep(known, cancelToken, id)
	}

	switch {
	case s.Run != nil, s.Uses != nil:
		s.If = c.ifCondition(ifToken, ifStep)
		if s.Run == nil {
			c.validateUses(s.Uses)
		}
	case s.Wait != nil:
		s.Name = syntheticName(s.Name, "Wait")
	case s.WaitAll != nil:
		s.Name = syntheticName(s.Name, "Wait for all")
	case s.Cancel != nil:
		s.Name = syntheticName(s.Name, "Cancel")
	case parallel != nil:
		s.Name = syntheticName(s.Name, "Parallel group")
		s.ContinueOnError = nil
		s.Parallel = c.steps(parallel)
		for _, ps := range s.Parallel {
			switch {
			case ps.Background != nil:
				c.ctx.Error(t, "'background' is not allowed inside a parallel block. Steps in a parallel block are automatically run as background steps.")
			case ps.Wait != nil:
				c.ctx.Error(t, "'wait' is not allowed inside a parallel block.")
			case ps.WaitAll != nil:
				c.ctx.Error(t, "'wait-all' is not allowed inside a parallel block.")
			case ps.Cancel != nil:
				c.ctx.Error(t, "'cancel' is not allowed inside a parallel block.")
			case ps.Parallel != nil:
				c.ctx.Error(t, "Nested 'parallel' blocks are not allowed.")
			}
		}
	default:
		if c.opts.BackgroundSteps {
			c.ctx.Error(t, "Expected one of uses, run, wait, wait-all, cancel, or parallel to be defined")
		} else {
			c.ctx.Error(t, "Expected one of uses or run to be defined")
		}
	}
	return s
}

func syntheticName(name *template.Token, value string) *template.Token {
	if name != nil {
		return name
	}
	return template.NewString(value)
}

func (c *converter) validateTargetStep(known map[string]bool, target, own *template.Token) {
	switch {
	case strings.HasPrefix(target.Str, "__"):
		c.ctx.Error(target, "The identifier '"+target.Str+"' is invalid. IDs starting with '__' are reserved.")
	case own != nil && strings.EqualFold(target.Str, own.Str):
		c.ctx.Error(target, "Step '"+own.Str+"' cannot reference itself")
	case !known[target.Str]:
		c.ctx.Error(target, "Step references unknown step ID '"+target.Str+"'")
	}
}

func (c *converter) validateUses(uses *template.Token) {
	v := uses.Str
	if rest, ok := strings.CutPrefix(v, selfRepositoryPrefix); ok {
		if strings.Contains(rest, "@") {
			c.ctx.Error(uses, "Self-repository references do not support an '@ref' suffix. Actual '"+v+"'")
		}
		return
	}
	if strings.HasPrefix(v, "docker://") || strings.HasPrefix(v, "./") || strings.HasPrefix(v, ".\\") {
		return
	}
	if _, _, ok := splitActionRef(v); !ok {
		c.ctx.Error(uses, "Expected format {org}/{repo}[/path]@ref. Actual '"+v+"'")
	}
}
