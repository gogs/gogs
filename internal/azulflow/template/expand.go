package template

import (
	"slices"
	"strings"

	"gogs.io/gogs/internal/azulflow/expr"
)

// Expand evaluates the expressions of a token whose contexts env provides,
// and checks the result against the definition named typ, as GitHub's
// template evaluator does at run time. An expression is evaluated only when
// env has every context and function its definition allows, so expressions
// meant for a later stage, such as an environment URL that reads step
// outputs, are kept. Problems are reported to the context.
//
// As in GitHub, a sequence item that evaluates to a sequence is replaced by
// its items, and a `${{ insert }}` key merges the mapping that its value
// evaluates to into the parent mapping.
func (c *Context) Expand(typ string, t *Token, env *expr.Env) *Token {
	opts := expr.ParseOptions{Contexts: make([]string, 0, len(env.Contexts))}
	available := make(map[string]bool, len(env.Contexts)+len(env.Funcs))
	for name := range env.Contexts {
		opts.Contexts = append(opts.Contexts, name)
		available[strings.ToLower(name)] = true
	}
	for name := range env.Funcs {
		opts.Funcs = append(opts.Funcs, expr.FuncSig{Name: name, Min: 0, Max: -1})
		available[strings.ToLower(name)+"()"] = true
	}

	var result *Token
	c.Run(t.File, func() {
		yr := &yamlReader{file: t.File}
		yr.addToken(t)
		yr.events = append(yr.events, event{kind: evDocumentEnd})
		r := &reader{
			c: c, schema: c.Schema, yaml: yr, file: t.File, evaluating: true,
			env: env, parseOpts: opts, available: available,
		}
		result = r.readValue(c.Schema.info(nil, typ))
	})
	return result
}

// expands reports whether expressions read with the definition are
// evaluated: the evaluation provides every context the definition allows.
func (r *reader) expands(def *defInfo) bool {
	if r.env == nil || len(def.allowedContext) == 0 {
		return false
	}
	for _, name := range def.allowedContext {
		name = strings.ToLower(name)
		if i := strings.IndexByte(name, '('); i > 0 {
			name = name[:i+1] + ")"
		}
		if !r.available[name] {
			return false
		}
	}
	return true
}

func (r *reader) eval(t *Token) (any, error) {
	e, err := expr.ParseWith(t.Expr, r.parseOpts)
	if err != nil {
		return nil, err
	}
	return e.Eval(r.env)
}

// nextExpression consumes the next event when it is an expression that the
// definition evaluates, and returns its token.
func (r *reader) nextExpression(def *defInfo) *Token {
	e := r.yaml.peek()
	if e.kind != evLiteral || e.token.Kind != BasicExpression || !r.expands(def) {
		return nil
	}
	r.yaml.pos++
	return e.token
}

// expandValue replaces an expression value with its result. A value that
// cannot be evaluated becomes an empty string.
func (r *reader) expandValue(def *defInfo) {
	t := r.nextExpression(def)
	if t == nil {
		return
	}
	v, err := r.eval(t)
	if err != nil {
		r.c.Error(t, err.Error())
		r.yaml.insert(&Token{Kind: String, File: t.File, Line: t.Line, Col: t.Col}, false)
		return
	}
	r.yaml.insert(FromValue(v, t), false)
}

// expandItem replaces an expression sequence item with its result, or with
// the items of a sequence result. An item that cannot be evaluated is left
// out. It reports whether it consumed an expression.
func (r *reader) expandItem(def *defInfo) bool {
	t := r.nextExpression(def)
	if t == nil {
		return false
	}
	v, err := r.eval(t)
	if err != nil {
		r.c.Error(t, err.Error())
		return true
	}
	value := FromValue(v, t)
	r.yaml.insert(value, value.Kind == Sequence)
	return true
}

// expandKey evaluates an expression mapping key, or merges the mapping of an
// insert directive. It returns the key to read, or nil when the entry was
// consumed.
func (r *reader) expandKey(key *Token, def *defInfo) *Token {
	if !r.expands(def) {
		return key
	}
	switch key.Kind {
	case BasicExpression:
		v, err := r.eval(key)
		if err != nil {
			r.c.Error(key, err.Error())
			r.skipValue()
			return nil
		}
		s := &Token{Kind: String, File: key.File, Line: key.Line, Col: key.Col}
		switch v.(type) {
		case []any, *expr.Object:
			r.c.Error(key, "Expected a string")
			s.Str = key.Expr
		default:
			s.Str = expr.ToString(v)
		}
		return s
	case InsertExpression:
		value := r.yaml.take()
		if value.Kind == BasicExpression {
			v, err := r.eval(value)
			if err != nil {
				r.c.Error(value, err.Error())
				return nil
			}
			if m := FromValue(v, value); m.Kind == Mapping {
				value = m
			} else {
				r.c.Error(value, "Expected a mapping")
				return nil
			}
		}
		if value.Kind != Mapping {
			r.c.Error(value, "Expected a mapping")
			return nil
		}
		r.yaml.insert(value, true)
		return nil
	}
	return key
}

// FromValue converts the result of an expression into a token at the
// position of the expression.
func FromValue(v any, at *Token) *Token {
	t := &Token{File: at.File, Line: at.Line, Col: at.Col}
	switch v := v.(type) {
	case nil:
		t.Kind = Null
	case bool:
		t.Kind, t.Bool = Boolean, v
	case float64:
		t.Kind, t.Num = Number, v
	case string:
		t.Kind, t.Str = String, v
	case []any:
		t.Kind = Sequence
		t.Seq = make([]*Token, len(v))
		for i, item := range v {
			t.Seq[i] = FromValue(item, at)
		}
	case *expr.Object:
		t.Kind = Mapping
		for _, k := range v.Keys() {
			item, _ := v.Get(k)
			t.Map = append(t.Map, &Pair{Key: &Token{Kind: String, File: at.File, Line: at.Line, Col: at.Col, Str: k}, Value: FromValue(item, at)})
		}
	default:
		t.Kind, t.Str = String, expr.ToString(v)
	}
	return t
}

// Value converts a token into an expression value. Expressions become their
// "${{ }}" form.
func (t *Token) Value() any {
	switch t.Kind {
	case Null:
		return nil
	case Boolean:
		return t.Bool
	case Number:
		return t.Num
	case Sequence:
		items := make([]any, len(t.Seq))
		for i, item := range t.Seq {
			items[i] = item.Value()
		}
		return items
	case Mapping:
		o := expr.NewObject()
		for _, p := range t.Map {
			o.Set(p.Key.String(), p.Value.Value())
		}
		return o
	}
	return t.String()
}

// insert puts the events of a token at the current position, or with
// contentsOnly, the events of the items or entries of a sequence or mapping.
func (y *yamlReader) insert(t *Token, contentsOnly bool) {
	tmp := &yamlReader{file: y.file}
	tmp.addToken(t)
	events := tmp.events
	if contentsOnly {
		events = events[1 : len(events)-1]
	}
	y.events = slices.Insert(y.events, y.pos, events...)
}

// take consumes the events of the next value and returns it as a token.
func (y *yamlReader) take() *Token {
	e := y.next()
	switch e.kind {
	case evLiteral:
		return e.token
	case evSequenceStart:
		t := *e.token
		for y.peek().kind != evSequenceEnd {
			t.Seq = append(t.Seq, y.take())
		}
		y.next()
		return &t
	case evMappingStart:
		t := *e.token
		for y.peek().kind != evMappingEnd {
			k := y.take()
			t.Map = append(t.Map, &Pair{Key: k, Value: y.take()})
		}
		y.next()
		return &t
	}
	panic(abort{errExpectedValue})
}
