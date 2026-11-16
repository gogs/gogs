package template

import (
	_ "embed"
	"slices"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"

	"gogs.io/gogs/internal/azulflow/expr"
)

// workflowSchema is GitHub's workflow schema. It merges the copies in
// actions/runner and actions/languageservices (see NOTICE), so that every
// workflow either copy accepts is accepted.
//
//go:embed workflow-v1.0.json
var workflowSchema []byte

// The schema definitions of a workflow file. GitHub's server validates
// workflows with WorkflowRoot, which accepts any event configuration.
// WorkflowRootStrict also validates the events and their filters, as GitHub's
// editor tooling does.
const (
	WorkflowRoot       = "workflow-root"
	WorkflowRootStrict = "workflow-root-strict"
)

const (
	defNull     = "null"
	defBoolean  = "boolean"
	defNumber   = "number"
	defString   = "string"
	defSequence = "sequence"
	defMapping  = "mapping"
	defAny      = "any"
)

type defKind int

const (
	kindNull defKind = iota
	kindBoolean
	kindNumber
	kindString
	kindSequence
	kindMapping
	kindOneOf
)

type property struct {
	name     string
	typ      string
	required bool
}

// definition is one entry of the schema.
type definition struct {
	name    string
	kind    defKind
	context []string // Contexts and functions expressions may use.

	// String.
	constant        string
	ignoreCase      bool
	requireNonEmpty bool

	// Sequence.
	itemType string

	// Mapping.
	properties     []*property
	looseKeyType   string
	looseValueType string

	// One-of.
	oneOf []string
}

func (d *definition) property(name string) *property {
	for _, p := range d.properties {
		if p.name == name {
			return p
		}
	}
	return nil
}

func (d *definition) isScalar() bool {
	return d.kind == kindNull || d.kind == kindBoolean || d.kind == kindNumber || d.kind == kindString
}

// matches reports whether a literal is valid for a scalar definition.
func (d *definition) matches(t *Token) bool {
	switch d.kind {
	case kindNull:
		return t.Kind == Null
	case kindBoolean:
		return t.Kind == Boolean
	case kindNumber:
		return t.Kind == Number
	case kindString:
		if t.Kind != String {
			return false
		}
		switch {
		case d.constant != "" && d.ignoreCase:
			return strings.EqualFold(d.constant, t.Str)
		case d.constant != "":
			return d.constant == t.Str
		case d.requireNonEmpty:
			return t.Str != ""
		}
		return true
	}
	return false
}

// Schema is a set of definitions that templates are validated against.
type Schema struct {
	defs map[string]*definition
}

func (s *Schema) get(name string) *definition {
	d, ok := s.defs[name]
	if !ok {
		panic("schema definition '" + name + "' not found")
	}
	return d
}

// expand returns the definition, or the definitions a one-of refers to.
func (s *Schema) expand(d *definition) []*definition {
	if d.kind != kindOneOf {
		return []*definition{d}
	}
	out := make([]*definition, 0, len(d.oneOf))
	for _, name := range d.oneOf {
		out = append(out, s.get(name))
	}
	return out
}

// scalars returns the scalar definitions a definition accepts, expanding
// nested one-of definitions.
func (s *Schema) scalars(d *definition) []*definition {
	switch {
	case d.isScalar():
		return []*definition{d}
	case d.kind == kindOneOf:
		var out []*definition
		for _, name := range d.oneOf {
			out = append(out, s.scalars(s.get(name))...)
		}
		return out
	}
	return nil
}

// defInfo is a definition together with the contexts its expressions may
// use, which accumulate from the parent definitions.
type defInfo struct {
	def            *definition
	allowedContext []string
}

func (s *Schema) info(parent *defInfo, name string) *defInfo {
	d := s.get(name)
	info := &defInfo{def: d}
	if parent != nil {
		info.allowedContext = parent.allowedContext
	}
	if len(d.context) > 0 {
		merged := slices.Clone(info.allowedContext)
		for _, c := range d.context {
			if !slices.ContainsFunc(merged, func(m string) bool { return strings.EqualFold(m, c) }) {
				merged = append(merged, c)
			}
		}
		info.allowedContext = merged
	}
	return info
}

var (
	schemaOnce        sync.Once
	defaultSchema     *Schema
	errSchemaNotValid error
)

// WorkflowSchema returns GitHub's workflow schema.
func WorkflowSchema() *Schema {
	schemaOnce.Do(func() {
		defaultSchema, errSchemaNotValid = loadSchema(workflowSchema)
	})
	if errSchemaNotValid != nil {
		panic(errSchemaNotValid)
	}
	return defaultSchema
}

func loadSchema(data []byte) (*Schema, error) {
	v, err := expr.FromJSON(string(data))
	if err != nil {
		return nil, errors.Wrap(err, "decode schema")
	}
	root, _ := v.(*expr.Object)
	rawDefs, _ := root.Get("definitions")
	defs, ok := rawDefs.(*expr.Object)
	if !ok {
		return nil, errors.New("schema has no definitions")
	}

	s := &Schema{defs: map[string]*definition{
		defNull:     {name: defNull, kind: kindNull},
		defBoolean:  {name: defBoolean, kind: kindBoolean},
		defNumber:   {name: defNumber, kind: kindNumber},
		defString:   {name: defString, kind: kindString},
		defSequence: {name: defSequence, kind: kindSequence, itemType: defAny},
		defMapping:  {name: defMapping, kind: kindMapping, looseKeyType: defString, looseValueType: defAny},
		defAny: {name: defAny, kind: kindOneOf, oneOf: []string{
			defNull, defBoolean, defNumber, defString, defSequence, defMapping,
		}},
	}}

	str := func(o *expr.Object, key string) string {
		v, _ := o.Get(key)
		s, _ := v.(string)
		return s
	}
	boolean := func(o *expr.Object, key string) bool {
		v, _ := o.Get(key)
		b, _ := v.(bool)
		return b
	}
	strs := func(o *expr.Object, key string) []string {
		v, _ := o.Get(key)
		items, _ := v.([]any)
		out := make([]string, 0, len(items))
		for _, item := range items {
			s, _ := item.(string)
			out = append(out, s)
		}
		return out
	}

	for _, name := range defs.Keys() {
		raw, _ := defs.Get(name)
		o, ok := raw.(*expr.Object)
		if !ok {
			return nil, errors.Newf("definition %q is not a mapping", name)
		}
		d := &definition{name: name, context: strs(o, "context")}
		switch {
		case has(o, "null"):
			d.kind = kindNull
		case has(o, "boolean"):
			d.kind = kindBoolean
		case has(o, "number"):
			d.kind = kindNumber
		case has(o, "string"):
			d.kind = kindString
			v, _ := o.Get("string")
			props, _ := v.(*expr.Object)
			d.constant = str(props, "constant")
			d.ignoreCase = boolean(props, "ignore-case")
			d.requireNonEmpty = boolean(props, "require-non-empty")
		case has(o, "sequence"):
			d.kind = kindSequence
			v, _ := o.Get("sequence")
			d.itemType = str(v.(*expr.Object), "item-type")
		case has(o, "mapping"):
			d.kind = kindMapping
			v, _ := o.Get("mapping")
			m := v.(*expr.Object)
			d.looseKeyType = str(m, "loose-key-type")
			d.looseValueType = str(m, "loose-value-type")
			rawProps, _ := m.Get("properties")
			props, _ := rawProps.(*expr.Object)
			for _, pname := range props.Keys() {
				pv, _ := props.Get(pname)
				p := &property{name: pname}
				switch pv := pv.(type) {
				case string:
					p.typ = pv
				case *expr.Object:
					p.typ = str(pv, "type")
					p.required = boolean(pv, "required")
				}
				d.properties = append(d.properties, p)
			}
		case has(o, "one-of"):
			d.kind = kindOneOf
			d.oneOf = strs(o, "one-of")
		case has(o, "allowed-values"):
			// Allowed values become a one-of of constant strings.
			d.kind = kindOneOf
			for _, value := range strs(o, "allowed-values") {
				key := name + "-" + value
				s.defs[key] = &definition{name: key, kind: kindString, constant: value}
				d.oneOf = append(d.oneOf, key)
			}
		default:
			return nil, errors.Newf("definition %q has no type", name)
		}
		s.defs[name] = d
	}

	for name, d := range s.defs {
		refs := slices.Clone(d.oneOf)
		if d.itemType != "" {
			refs = append(refs, d.itemType)
		}
		if d.looseKeyType != "" {
			refs = append(refs, d.looseKeyType, d.looseValueType)
		}
		for _, p := range d.properties {
			refs = append(refs, p.typ)
		}
		for _, ref := range refs {
			if _, ok := s.defs[ref]; !ok {
				return nil, errors.Newf("definition %q refers to unknown definition %q", name, ref)
			}
		}
	}
	return s, nil
}

func has(o *expr.Object, key string) bool {
	_, ok := o.Get(key)
	return ok
}
