package model

import (
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/template"
)

// loadReusableWorkflows loads the workflows that reusable workflow jobs call,
// and the workflows those call, and validates the calls.
func (c *converter) loadReusableWorkflows(w *Workflow) (err error) {
	if c.opts.SkipReusableWorkflows || c.failed() {
		return nil
	}
	defer c.catch()
	for _, j := range w.Jobs {
		if j.Type != JobTypeReusable {
			continue
		}
		if err := c.loadReusable(j, 1); err != nil {
			return err
		}
		if c.failed() {
			return nil
		}
	}
	return nil
}

func (c *converter) loadReusable(j *Job, depth int) error {
	if depth > c.opts.MaxNestedDepth {
		throw("Nested reusable workflow depth exceeded " + strconv.Itoa(c.opts.MaxNestedDepth) + ".")
	}

	err := func() error {
		defer c.prefixCallerErrors(j)
		ref, err := c.qualifyRef(j.Ref)
		if err != nil {
			return err
		}
		j.Ref = ref
		root := c.loadFile(ref.Str)
		if c.failed() {
			return nil
		}
		c.referencedWorkflow(root, j)
		return nil
	}()
	if err != nil || c.failed() {
		return err
	}

	for _, nested := range j.Jobs {
		if nested.Type != JobTypeReusable {
			continue
		}
		if err := c.loadReusable(nested, depth+1); err != nil {
			return err
		}
		if c.failed() {
			return nil
		}
	}
	return nil
}

// qualifyRef returns the full name of a reusable workflow in the same
// repository, "./{path}" or "$/{path}".
func (c *converter) qualifyRef(ref *template.Token) (*template.Token, error) {
	if !strings.HasPrefix(ref.Str, "./") && !strings.HasPrefix(ref.Str, selfRepositoryPrefix) {
		return ref, nil
	}
	caller := c.ctx.FileName(ref.File)
	full, err := c.files.ResolveRef(caller, ref.Str)
	if err != nil {
		return nil, errors.Wrapf(ErrReferencedWorkflowNotFound, "cannot find the caller workflow '%s' from the referenced workflows: %v", caller, err)
	}
	q := template.NewString(full)
	q.File, q.Line, q.Col = ref.File, ref.Line, ref.Col
	return q, nil
}

// prefixCallerErrors names the call in the errors that do not come from the
// file of the caller.
func (c *converter) prefixCallerErrors(j *Job) {
	caller := c.ctx.FileName(j.Ref.File)
	for i, msg := range c.ctx.Errors() {
		if msg == "" || !strings.HasPrefix(msg, caller) {
			c.ctx.PrefixError(i, "In "+caller+" "+lineCol(j.Ref)+": Error from called workflow")
		}
	}
}

// referencedWorkflow converts a reusable workflow into the job that calls
// it.
func (c *converter) referencedWorkflow(root *template.Token, j *Job) {
	explicitMax := j.Permissions
	mustOK(root.AssertMapping("root"))
	for _, p := range root.Map {
		mustOK(p.Key.AssertString("root key"))
		switch p.Key.Str {
		case "on":
			c.workflowCallTrigger(p.Value, j)
		case "description", "name", "run-name":
		case "defaults":
			j.Defaults = p.Value
		case "env":
			j.Env = p.Value
		case "concurrency":
			c.concurrency(p.Value)
			j.EmbeddedConcurrency = p.Value
		case "jobs":
			j.Jobs = c.jobs(p.Value)
		case "permissions":
			requested := c.permissions(p.Value)
			c.validateEmbeddedPermissions(j, nil, requested, explicitMax, false)
			j.Permissions = requested
		default:
			throwUnexpectedValue("root key", p.Key)
		}
	}
	if j.Jobs == nil {
		j.Jobs = []*Job{}
	}

	for _, embedded := range j.Jobs {
		if embedded.Permissions != nil {
			c.validateEmbeddedPermissions(j, embedded, embedded.Permissions, explicitMax, false)
		} else if j.Permissions != nil {
			embedded.Permissions = j.Permissions
		}
	}
}

func (c *converter) workflowCallTrigger(on *template.Token, j *Job) {
	const call = "workflow_call"
	switch on.Kind {
	case template.String:
		if on.Str == call {
			c.validateCall(j)
			return
		}
	case template.Sequence:
		for _, item := range on.Seq {
			mustOK(item.AssertString("Reference workflow on value " + item.String()))
			if item.Str == call {
				c.validateCall(j)
				return
			}
		}
	case template.Mapping:
		for _, p := range on.Map {
			mustOK(p.Key.AssertString("Reference workflow on value " + p.Key.String()))
			if p.Key.Str != call {
				continue
			}
			if p.Value.Kind != template.Null {
				mustOK(p.Value.AssertMapping("Reference workflow on value " + p.Key.String()))
				for _, d := range p.Value.Map {
					what := "on-workflow_call-" + d.Key.String()
					mustOK(d.Key.AssertString(what))
					switch d.Key.Str {
					case "inputs":
						mustOK(d.Value.AssertMapping(what))
						j.InputDefinitions = d.Value
					case "secrets":
						mustOK(d.Value.AssertMapping(what))
						j.SecretDefinitions = d.Value
					case "outputs":
						mustOK(d.Value.AssertMapping(what))
						j.Outputs = d.Value
					}
				}
			}
			c.validateCall(j)
			return
		}
	}
	c.ctx.Error(on, call+" key is not defined in the referenced workflow.")
}

// lookup returns the value of the first entry whose key equals name,
// ignoring case.
func lookup(m *template.Token, name string) *template.Token {
	if m == nil {
		return nil
	}
	for _, p := range m.Map {
		if strings.EqualFold(p.Key.Str, name) {
			return p.Value
		}
	}
	return nil
}

var inputTypeDefinitions = map[string]string{
	"boolean": "boolean-needs-context",
	"number":  "number-needs-context",
	"string":  "string-needs-context",
}

// validateCall checks the inputs and secrets of a call against the ones the
// reusable workflow declares.
func (c *converter) validateCall(j *Job) {
	if j.InputDefinitions != nil {
		for _, p := range j.InputDefinitions.Map {
			mustOK(p.Key.AssertString("inputs key"))
			name := p.Key.Str
			mustOK(p.Value.AssertMapping("input " + name))
			typ := lookup(p.Value, "type")
			if typ == nil {
				continue
			}
			mustOK(typ.AssertString("inputs " + name + " type"))
			def, ok := inputTypeDefinitions[typ.Str]
			if !ok {
				throw("Unexpected defined type '" + typ.Str + "' when converting input value for '" + name + "'")
			}
			if d := lookup(p.Value, "default"); d != nil {
				c.ctx.Evaluate(def, d)
			}
			if v := lookup(j.InputValues, name); v != nil {
				c.ctx.Evaluate(def, v)
				continue
			}
			if r := lookup(p.Value, "required"); r != nil {
				mustOK(r.AssertBoolean("required"))
				if r.Bool {
					c.ctx.Error(j.Ref, "Input "+name+" is required, but not provided while calling.")
				}
			}
		}
	}
	c.validateUndefined(j.InputDefinitions, j.InputValues, "input")

	if j.InheritSecrets {
		return
	}
	if j.SecretDefinitions != nil {
		for _, p := range j.SecretDefinitions.Map {
			mustOK(p.Key.AssertString("secrets key"))
			if p.Value.Kind == template.Null {
				continue
			}
			name := p.Key.Str
			mustOK(p.Value.AssertMapping("secret " + name))
			if lookup(j.SecretValues, name) != nil {
				continue
			}
			if r := lookup(p.Value, "required"); r != nil {
				mustOK(r.AssertBoolean("required"))
				if r.Bool {
					c.ctx.Error(j.Ref, "Secret "+name+" is required, but not provided while calling.")
				}
			}
		}
	}
	c.validateUndefined(j.SecretDefinitions, j.SecretValues, "secret")
}

func (c *converter) validateUndefined(definitions, values *template.Token, kind string) {
	if values == nil {
		return
	}
	for _, p := range values.Map {
		if lookup(definitions, p.Key.Str) == nil {
			c.ctx.Error(p.Value, "Invalid "+kind+", "+p.Key.Str+" is not defined in the referenced workflow.")
		}
	}
}
