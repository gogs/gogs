package model

import (
	"math"
	"strconv"
	"strings"

	"gogs.io/gogs/internal/azulflow/template"
)

// Events are the events that trigger a workflow, in the order of the file.
type Events struct {
	Items []*Event
}

// Get returns the event with the name, or nil.
func (e *Events) Get(name string) *Event {
	if e == nil {
		return nil
	}
	for _, ev := range e.Items {
		if ev.Name == name {
			return ev
		}
	}
	return nil
}

// Event is an event that triggers a workflow, with its filters.
type Event struct {
	Name string
	// Filters are the filters of the event, such as "branches" and
	// "paths-ignore", in the order GitHub writes them.
	Filters          []Filter
	Schedule         []Schedule
	WorkflowDispatch *WorkflowDispatch
	WorkflowCall     *WorkflowCall
}

// Filter returns the values of a filter, and whether it is set.
func (e *Event) Filter(name string) ([]string, bool) {
	for _, f := range e.Filters {
		if f.Name == name {
			return f.Values, true
		}
	}
	return nil, false
}

// Filter is a filter of an event.
type Filter struct {
	Name   string
	Values []string
}

// Schedule is a time a workflow runs.
type Schedule struct {
	Cron     string
	Timezone string
}

// WorkflowDispatch is the configuration of the workflow_dispatch event.
type WorkflowDispatch struct {
	Inputs []*Input // Nil when not declared.
}

// WorkflowCall is the configuration of the workflow_call event.
type WorkflowCall struct {
	Inputs  []*Input  // Nil when not declared.
	Secrets []*Secret // Nil when not declared.
}

// Input is an input of a manually triggered or called workflow.
type Input struct {
	Name        string
	Type        string // Empty for a type GitHub does not convert, "number".
	Description string
	Required    bool
	// Default is a string or a boolean, or nil.
	Default any
	// DefaultValue is the default as written, of any type, which GitHub's
	// template leaves out for the types it does not convert.
	DefaultValue *template.Token
	Options      []string
	// keys are the properties set, in the order GitHub writes them.
	keys []string
}

// Secret is a secret of a called workflow.
type Secret struct {
	Name        string
	Description string
	Required    bool
	keys        []string
}

func (e *Events) json() jsonValue {
	o := &jsonObject{}
	for _, ev := range e.Items {
		o.set(ev.Name, ev.json())
	}
	return o
}

func (e *Event) json() jsonValue {
	switch {
	case e.Schedule != nil:
		items := make([]jsonValue, len(e.Schedule))
		for i, s := range e.Schedule {
			o := &jsonObject{}
			o.set("cron", s.Cron)
			if s.Timezone != "" {
				o.set("timezone", s.Timezone)
			}
			items[i] = o
		}
		return items
	case e.WorkflowDispatch != nil:
		o := &jsonObject{}
		if e.WorkflowDispatch.Inputs != nil {
			o.set("inputs", inputsJSON(e.WorkflowDispatch.Inputs))
		}
		return o
	case e.WorkflowCall != nil:
		o := &jsonObject{}
		if e.WorkflowCall.Inputs != nil {
			o.set("inputs", inputsJSON(e.WorkflowCall.Inputs))
		}
		if e.WorkflowCall.Secrets != nil {
			secrets := &jsonObject{}
			for _, s := range e.WorkflowCall.Secrets {
				so := &jsonObject{}
				for _, k := range s.keys {
					switch k {
					case "description":
						so.set(k, s.Description)
					case "required":
						so.set(k, s.Required)
					}
				}
				secrets.set(s.Name, so)
			}
			o.set("secrets", secrets)
		}
		return o
	}
	o := &jsonObject{}
	for _, f := range e.Filters {
		o.set(f.Name, stringsJSON(f.Values))
	}
	return o
}

func inputsJSON(inputs []*Input) jsonValue {
	o := &jsonObject{}
	for _, in := range inputs {
		io := &jsonObject{}
		for _, k := range in.keys {
			switch k {
			case "type":
				io.set(k, in.Type)
			case "description":
				io.set(k, in.Description)
			case "required":
				io.set(k, in.Required)
			case "default":
				io.set(k, in.Default)
			case "options":
				io.set(k, stringsJSON(in.Options))
			}
		}
		o.set(in.Name, io)
	}
	return o
}

// events converts the "on" value of a workflow.
func (c *converter) events(t *template.Token) *Events {
	result := &Events{}
	add := func(ev *Event) {
		for i, existing := range result.Items {
			if existing.Name == ev.Name {
				result.Items[i] = ev
				return
			}
		}
		result.Items = append(result.Items, ev)
	}
	switch {
	case t.IsLiteral():
		mustOK(t.AssertString("on"))
		add(&Event{Name: t.Str})
	case t.Kind == template.Sequence:
		for _, item := range t.Seq {
			mustOK(item.AssertString("on"))
			add(&Event{Name: item.Str})
		}
	case t.Kind == template.Mapping:
		for _, p := range t.Map {
			mustOK(p.Key.AssertString("event name"))
			name := p.Key.Str
			ev := &Event{Name: name}
			switch {
			case p.Value.Kind == template.Null:
			case name == "schedule":
				mustOK(p.Value.AssertSequence("event " + name))
				ev.Schedule = c.schedule(p.Value)
			default:
				mustOK(p.Value.AssertMapping("event " + name))
				switch name {
				case "workflow_call":
					ev.WorkflowCall = c.workflowCall(p.Value)
				case "workflow_dispatch":
					ev.WorkflowDispatch = &WorkflowDispatch{}
					if inputs := p.Value.Get("inputs"); inputs != nil && inputs.Kind != template.Null {
						mustOK(inputs.AssertMapping("workflow dispatch inputs"))
						ev.WorkflowDispatch.Inputs = c.inputs(inputs, true)
					}
				default:
					ev.Filters = eventFilters(p.Value)
				}
			}
			add(ev)
		}
	default:
		c.ctx.Error(t, "Invalid format for 'on'")
	}
	return result
}

func eventFilters(t *template.Token) []Filter {
	var filters []Filter
	list := func(name string, value *template.Token) {
		var values []string
		if value.Kind == template.String {
			values = []string{value.Str}
		} else {
			mustOK(value.AssertSequence(name + " list"))
			values = stringList(name, value)
		}
		filters = append(filters, Filter{Name: name, Values: values})
	}
	for _, name := range []string{"branches", "tags", "paths"} {
		for _, p := range t.Map {
			if p.Key.Str == name || p.Key.Str == name+"-ignore" {
				list(p.Key.Str, p.Value)
			}
		}
	}
	for _, name := range []string{"types", "versions", "names", "workflows"} {
		for _, p := range t.Map {
			if p.Key.Str == name {
				list(name, p.Value)
			}
		}
	}
	return filters
}

func stringList(name string, t *template.Token) []string {
	out := []string{}
	for _, item := range t.Seq {
		mustOK(item.AssertString(name + " item"))
		out = append(out, item.Str)
	}
	return out
}

func (c *converter) schedule(t *template.Token) []Schedule {
	result := []Schedule{}
	for _, item := range t.Seq {
		mustOK(item.AssertMapping("event schedule"))
		s := Schedule{}
		valid := true
		for _, p := range item.Map {
			mustOK(p.Key.AssertString("schedule key"))
			switch p.Key.Str {
			case "cron":
				mustOK(p.Value.AssertString("schedule cron"))
				if !ValidCron(p.Value.Str) {
					c.ctx.Error(p.Value, "Invalid cron expression. Expected format: '* * * * *' (minute hour day month weekday)")
				}
				s.Cron = p.Value.Str
			case "timezone":
				mustOK(p.Value.AssertString("schedule timezone"))
				s.Timezone = p.Value.Str
			default:
				c.ctx.Error(p.Key, "Invalid schedule key")
				valid = false
			}
		}
		switch {
		case valid && s.Cron != "":
			result = append(result, s)
		case valid:
			c.ctx.Error(item, "Missing required key 'cron' in schedule entry")
		}
	}
	return result
}

func (c *converter) workflowCall(t *template.Token) *WorkflowCall {
	result := &WorkflowCall{}
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("workflow dispatch input key"))
		switch p.Key.Str {
		case "inputs":
			mustOK(p.Value.AssertMapping("workflow dispatch inputs"))
			result.Inputs = c.inputs(p.Value, false)
		case "secrets":
			mustOK(p.Value.AssertMapping("workflow dispatch inputs"))
			result.Secrets = []*Secret{}
			for _, sp := range p.Value.Map {
				mustOK(sp.Key.AssertString("secret name"))
				s := &Secret{Name: sp.Key.Str}
				for _, item := range sp.Value.Map {
					mustOK(item.Key.AssertString("workflow call secret key"))
					switch item.Key.Str {
					case "description":
						mustOK(item.Value.AssertString("secret description"))
						s.Description = item.Value.Str
						s.keys = appendKey(s.keys, "description")
					case "required":
						mustOK(item.Value.AssertBoolean("secret required"))
						s.Required = item.Value.Bool
						s.keys = appendKey(s.keys, "required")
					}
				}
				result.Secrets = append(result.Secrets, s)
			}
		}
	}
	return result
}

var inputTypes = map[string]bool{"string": true, "choice": true, "boolean": true, "environment": true}

// inputs converts the inputs of workflow_dispatch, or with dispatch false,
// of workflow_call. A default that is an expression is only allowed for
// workflow_call.
func (c *converter) inputs(t *template.Token, dispatch bool) []*Input {
	result := []*Input{}
	for _, p := range t.Map {
		mustOK(p.Key.AssertString("input name"))
		in := &Input{Name: p.Key.Str, Type: "string", keys: []string{"type"}}
		result = append(result, in)
		if p.Value.Kind == template.Null {
			continue
		}
		mustOK(p.Value.AssertMapping("input configuration"))
		var def *template.Token
		for _, item := range p.Value.Map {
			mustOK(item.Key.AssertString("workflow dispatch input key"))
			switch item.Key.Str {
			case "description":
				mustOK(item.Value.AssertString("input description"))
				in.Description = item.Value.Str
				in.keys = appendKey(in.keys, "description")
			case "required":
				mustOK(item.Value.AssertBoolean("input required"))
				in.Required = item.Value.Bool
				in.keys = appendKey(in.keys, "required")
			case "default":
				mustOK(item.Value.AssertScalar("input default"))
				def = item.Value
				in.DefaultValue = item.Value
			case "type":
				mustOK(item.Value.AssertString("input type"))
				in.Type = item.Value.Str
				if !inputTypes[in.Type] {
					in.Type = ""
					in.keys = removeKey(in.keys, "type")
				}
			case "options":
				mustOK(item.Value.AssertSequence("input options"))
				in.Options = stringList("input options", item.Value)
				in.keys = appendKey(in.keys, "options")
			default:
				c.ctx.Error(item.Key, "Invalid key '"+item.Key.Str+"'")
			}
		}

		if def != nil && (dispatch || !def.IsExpression()) {
			var err error
			switch in.Type {
			case "boolean":
				if err = def.AssertBoolean("input default"); err == nil {
					in.Default = def.Bool
				}
			case "string", "choice", "environment":
				if err = def.AssertString("input default"); err == nil {
					in.Default = def.Str
				}
			}
			if err != nil {
				c.ctx.Error(def, err.Error())
			} else if in.Default != nil {
				in.keys = appendKey(in.keys, "default")
			}
		}

		if in.Type == "choice" {
			if len(in.Options) == 0 {
				c.ctx.Error(p.Value, "Missing 'options' for choice input")
			}
		} else if in.Options != nil {
			c.ctx.Error(p.Value, "Input type is not 'choice', but 'options' is defined")
		}
	}
	return result
}

func appendKey(keys []string, key string) []string {
	for _, k := range keys {
		if k == key {
			return keys
		}
	}
	return append(keys, key)
}

func removeKey(keys []string, key string) []string {
	out := keys[:0]
	for _, k := range keys {
		if k != key {
			out = append(out, k)
		}
	}
	return out
}

type cronRange struct {
	min, max int
	names    map[string]int
}

var (
	minuteRange = cronRange{min: 0, max: 59}
	hourRange   = cronRange{min: 0, max: 23}
	domRange    = cronRange{min: 1, max: 31}
	monthRange  = cronRange{min: 1, max: 12, names: map[string]int{
		"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "oct": 10, "nov": 11, "dec": 12,
	}}
	dowRange = cronRange{min: 0, max: 6, names: map[string]int{
		"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6,
	}}
)

// ValidCron reports whether a schedule is a valid POSIX cron expression with
// five fields, as GitHub accepts it.
func ValidCron(cron string) bool {
	parts := strings.Split(cron, " ")
	fields := parts[:0]
	for _, p := range parts {
		if p != "" {
			fields = append(fields, p)
		}
	}
	// JavaScript's split(/ +/) keeps empty fields at the ends.
	if strings.HasPrefix(cron, " ") || strings.HasSuffix(cron, " ") || len(fields) != 5 {
		return false
	}
	return cronPart(fields[0], minuteRange, true) && cronPart(fields[1], hourRange, true) &&
		cronPart(fields[2], domRange, true) && cronPart(fields[3], monthRange, true) &&
		cronPart(fields[4], dowRange, true)
}

func cronPart(value string, r cronRange, separators bool) bool {
	if _, ok := r.names[strings.ToLower(value)]; ok {
		return true
	}
	if value == "*" {
		return true
	}
	if strings.Contains(value, ",") {
		if !separators {
			return false
		}
		for _, v := range strings.Split(value, ",") {
			if v == "" || !cronPart(v, r, true) {
				return false
			}
		}
		return true
	}
	if strings.Contains(value, "/") {
		parts := strings.Split(value, "/")
		if !separators || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return false
		}
		step := jsNumber(parts[1])
		if math.IsNaN(step) || step <= 0 {
			return false
		}
		return cronPart(parts[0], r, true) && cronPart(parts[1], r, false)
	}
	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if !separators || len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return false
		}
		return cronPart(parts[0], r, false) && cronPart(parts[1], r, false) &&
			cronNumber(parts[1], r) >= cronNumber(parts[0], r)
	}
	n := jsNumber(value)
	return !math.IsNaN(n) && n >= float64(r.min) && n <= float64(r.max)
}

func cronNumber(value string, r cronRange) float64 {
	if n, ok := r.names[strings.ToLower(value)]; ok {
		return float64(n)
	}
	return jsNumber(value)
}

// jsNumber converts a string to a number as JavaScript's unary plus does, for
// the decimal forms a cron field can have.
func jsNumber(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || strings.ContainsAny(s, "xXpP_") || strings.EqualFold(strings.TrimLeft(s, "+-"), "inf") ||
		strings.EqualFold(strings.TrimLeft(s, "+-"), "infinity") || strings.EqualFold(s, "nan") {
		return math.NaN()
	}
	return f
}
