package template

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	"gogs.io/gogs/internal/azulflow/expr"
)

const (
	openExpression  = "${{"
	closeExpression = "}}"

	errUnexpectedSequence = "A sequence was not expected"
	errUnexpectedMapping  = "A mapping was not expected"
	errExpressionNotAllow = "A template expression is not allowed in this context"
	errExpressionNotClose = "The expression is not closed. An unescaped ${{ sequence was found, but the closing }} sequence was not found."
	errExpectedExpression = "An expression was expected"
)

// The properties of background steps, an experimental feature that is
// disabled by default.
var (
	backgroundStepProperties     = map[string]bool{"background": true, "wait": true, "wait-all": true, "cancel": true}
	backgroundStepOnlyProperties = map[string]bool{"wait": true, "wait-all": true, "cancel": true}
)

func errUnexpectedValue(value string) string {
	return "Unexpected value '" + value + "'"
}

// Read reads a YAML file into a token validated against the definition
// named typ, such as WorkflowRoot. Problems are reported to the context, and
// the result is nil when reading stopped early.
func Read(c *Context, typ string, file int, content []byte, allowAnchors bool) *Token {
	var result *Token
	c.Run(file, func() {
		yr, err := newYAMLReader(file, content, allowAnchors)
		if err != nil {
			panic(abort{yamlError(err)})
		}
		r := &reader{c: c, schema: c.Schema, yaml: yr, file: file}
		result = r.readValue(c.Schema.info(nil, typ))
		if e := yr.next(); e.kind != evDocumentEnd {
			panic(abort{errExpectedDocumentEnd})
		}
	})
	return result
}

type reader struct {
	c      *Context
	schema *Schema
	yaml   *yamlReader
	file   int
	// evaluating is set when reading a token instead of a file. Its strings
	// are values, not templates.
	evaluating bool

	// env is set by Expand, which evaluates the expressions whose allowed
	// contexts are all available.
	env       *expr.Env
	parseOpts expr.ParseOptions
	available map[string]bool
}

// Evaluate checks a token against the definition named typ, and returns the
// result, in which numbers, booleans, and nulls that only a string definition
// accepts are strings. It is how GitHub validates a template whose
// expressions cannot be evaluated yet, so expressions are kept.
func (c *Context) Evaluate(typ string, t *Token) *Token {
	var result *Token
	c.bytes -= tokenBytes(t)
	c.Run(t.File, func() {
		yr := &yamlReader{file: t.File}
		yr.addToken(t)
		yr.events = append(yr.events, event{kind: evDocumentEnd})
		r := &reader{c: c, schema: c.Schema, yaml: yr, file: t.File, evaluating: true}
		result = r.readValue(c.Schema.info(nil, typ))
	})
	return result
}

func (r *reader) readValue(def *defInfo) *Token {
	r.c.incrementEvents()
	if r.env != nil {
		r.expandValue(def)
	}

	if lit, ok := r.yaml.allow(evLiteral); ok {
		scalar := r.parseScalar(lit, def)
		scalar = r.validate(scalar, def)
		r.c.AddBytes(tokenBytes(scalar))
		return scalar
	}

	if seq, ok := r.yaml.allow(evSequenceStart); ok {
		r.c.incrementDepth()
		r.c.AddBytes(tokenBytes(seq))
		seq.def = def
		var seqDef *definition
		for _, d := range r.schema.expand(def.def) {
			if d.kind == kindSequence {
				seqDef = d
				break
			}
		}
		if seqDef != nil {
			item := r.schema.info(def, seqDef.itemType)
			for !r.allowEnd(evSequenceEnd) {
				if r.env != nil && r.expandItem(item) {
					continue
				}
				seq.Seq = append(seq.Seq, r.readValue(item))
			}
		} else {
			r.c.Error(seq, errUnexpectedSequence)
			for !r.allowEnd(evSequenceEnd) {
				r.skipValue()
			}
		}
		r.c.decrementDepth()
		return seq
	}

	if m, ok := r.yaml.allow(evMappingStart); ok {
		r.c.incrementDepth()
		r.c.AddBytes(tokenBytes(m))
		m.def = def
		var mapDefs []*definition
		for _, d := range r.schema.expand(def.def) {
			if d.kind == kindMapping {
				mapDefs = append(mapDefs, d)
			}
		}
		switch {
		case len(mapDefs) == 0:
			r.c.Error(m, errUnexpectedMapping)
			for !r.allowEnd(evMappingEnd) {
				r.skipValue()
				r.skipValue()
			}
		case len(mapDefs) > 1 || len(mapDefs[0].properties) > 0 || mapDefs[0].looseKeyType == "":
			r.wellKnownProperties(def, mapDefs, m)
		default:
			r.looseProperties(def, r.schema.info(def, mapDefs[0].looseKeyType), r.schema.info(def, mapDefs[0].looseValueType), m)
		}
		r.c.decrementDepth()
		return m
	}

	panic(abort{errExpectedValue})
}

func (r *reader) allowEnd(kind eventKind) bool {
	_, ok := r.yaml.allow(kind)
	return ok
}

// key parses a mapping key. It returns the key, and whether it is an
// expression.
func (r *reader) key(lit *Token, def *defInfo) *Token {
	key := r.parseScalar(lit, def)
	if key.IsExpression() || key.Kind == String {
		return key
	}
	return &Token{Kind: String, File: key.File, Line: key.Line, Col: key.Col, Str: key.String()}
}

func (r *reader) wellKnownProperties(def *defInfo, mapDefs []*definition, m *Token) {
	var looseKey, looseValue *defInfo
	keys := map[string]bool{}
	hasExpressionKey := false

	for {
		lit, ok := r.yaml.allow(evLiteral)
		if !ok {
			break
		}
		if r.env != nil {
			if lit = r.expandKey(lit, def); lit == nil {
				continue
			}
		}
		key := r.key(lit, def)

		if key.IsExpression() {
			hasExpressionKey = true
			if len(def.allowedContext) > 0 || r.env != nil {
				r.c.AddBytes(tokenBytes(key))
				m.Map = append(m.Map, &Pair{Key: key, Value: r.readValue(r.schema.info(def, defAny))})
			} else {
				r.c.Error(key, errExpressionNotAllow)
				r.skipValue()
			}
			continue
		}

		upper := strings.ToUpper(key.Str)
		if keys[upper] {
			r.c.Error(key, "'"+key.Str+"' is already defined")
			r.skipValue()
			continue
		}
		keys[upper] = true

		if typ, filtered, ok := matchKey(mapDefs, key.Str); ok {
			mapDefs = filtered
			if !r.c.BackgroundSteps && backgroundStepProperties[key.Str] {
				r.c.Error(key, errUnexpectedValue(key.Str))
				r.skipValue()
				continue
			}
			r.c.AddBytes(tokenBytes(key))
			m.Map = append(m.Map, &Pair{Key: key, Value: r.readValue(r.schema.info(def, typ))})
			continue
		}

		if mapDefs[0].looseKeyType != "" {
			if looseKey == nil {
				looseKey = r.schema.info(def, mapDefs[0].looseKeyType)
				looseValue = r.schema.info(def, mapDefs[0].looseValueType)
			}
			r.validate(key, looseKey)
			r.c.AddBytes(tokenBytes(key))
			m.Map = append(m.Map, &Pair{Key: key, Value: r.readValue(looseValue)})
			continue
		}

		r.c.Error(key, errUnexpectedValue(key.Str))
		r.skipValue()
	}

	if len(mapDefs) > 1 {
		hits := map[string]int{}
		var order []string
		for _, d := range mapDefs {
			for _, p := range d.properties {
				if hits[p.name] == 0 {
					order = append(order, p.name)
				}
				hits[p.name]++
			}
		}
		var unique []string
		for _, name := range order {
			if hits[name] == 1 && (r.c.BackgroundSteps || !backgroundStepOnlyProperties[name]) {
				unique = append(unique, name)
			}
		}
		slices.Sort(unique)
		r.c.Error(m, "There's not enough info to determine what you meant. Add one of these properties: "+strings.Join(unique, ", "))
	} else if !hasExpressionKey {
		m.def = &defInfo{def: mapDefs[0], allowedContext: def.allowedContext}
		for _, p := range mapDefs[0].properties {
			if p.required && !keys[strings.ToUpper(p.name)] {
				r.c.Error(m, "Required property is missing: "+p.name)
			}
		}
	}
	if !r.allowEnd(evMappingEnd) {
		panic(abort{errExpectedMappingEnd})
	}
}

// matchKey finds the type of a well-known property, and narrows the mapping
// definitions to those that have it.
func matchKey(defs []*definition, key string) (typ string, filtered []*definition, ok bool) {
	for _, d := range defs {
		if p := d.property(key); p != nil {
			if typ == "" {
				typ = p.typ
			}
			filtered = append(filtered, d)
		}
	}
	if typ == "" {
		return "", defs, false
	}
	return typ, filtered, true
}

func (r *reader) looseProperties(def, keyDef, valueDef *defInfo, m *Token) {
	keys := map[string]bool{}
	for {
		lit, ok := r.yaml.allow(evLiteral)
		if !ok {
			break
		}
		if r.env != nil {
			if lit = r.expandKey(lit, def); lit == nil {
				continue
			}
		}
		key := r.key(lit, def)

		if key.IsExpression() {
			if len(def.allowedContext) > 0 || r.env != nil {
				r.c.AddBytes(tokenBytes(key))
				m.Map = append(m.Map, &Pair{Key: key, Value: r.readValue(valueDef)})
			} else {
				r.c.Error(key, errExpressionNotAllow)
				r.skipValue()
			}
			continue
		}

		upper := strings.ToUpper(key.Str)
		if keys[upper] {
			r.c.Error(key, "'"+key.Str+"' is already defined")
			r.skipValue()
			continue
		}
		keys[upper] = true

		r.validate(key, keyDef)
		r.c.AddBytes(tokenBytes(key))
		m.Map = append(m.Map, &Pair{Key: key, Value: r.readValue(valueDef)})
	}
	if !r.allowEnd(evMappingEnd) {
		panic(abort{errExpectedMappingEnd})
	}
}

func (r *reader) skipValue() {
	r.c.incrementEvents()
	if _, ok := r.yaml.allow(evLiteral); ok {
		return
	}
	if _, ok := r.yaml.allow(evSequenceStart); ok {
		r.c.incrementDepth()
		for !r.allowEnd(evSequenceEnd) {
			r.skipValue()
		}
		r.c.decrementDepth()
		return
	}
	if _, ok := r.yaml.allow(evMappingStart); ok {
		r.c.incrementDepth()
		for !r.allowEnd(evMappingEnd) {
			r.skipValue()
			r.skipValue()
		}
		r.c.decrementDepth()
		return
	}
	panic(abort{errExpectedValue})
}

// validate checks a scalar against a definition. A number, boolean, or null
// that only a string definition accepts becomes a string.
func (r *reader) validate(t *Token, def *defInfo) *Token {
	switch t.Kind {
	case Null, Boolean, Number, String:
		defs := r.schema.scalars(def.def)
		for _, d := range defs {
			if d.matches(t) {
				t.def = def
				return t
			}
		}
		if t.Kind != String {
			s := &Token{Kind: String, File: t.File, Line: t.Line, Col: t.Col, Str: t.String(), def: def}
			for _, d := range defs {
				if d.kind == kindString && d.matches(s) {
					return s
				}
			}
		}
		r.c.Error(t, errUnexpectedValue(t.String()))
	case BasicExpression:
		if len(def.allowedContext) == 0 && r.env == nil {
			r.c.Error(t, errExpressionNotAllow)
		}
	default:
		r.c.Error(t, errUnexpectedValue(t.String()))
	}
	t.def = def
	return t
}

// parseScalar splits a string into its literal parts and expressions. A
// string with one expression becomes that expression, and a string mixing
// text and expressions becomes a format() expression.
func (r *reader) parseScalar(t *Token, def *defInfo) *Token {
	if t.Kind != String || r.evaluating {
		return t
	}
	raw := t.Str
	start := strings.Index(raw, openExpression)
	if raw == "" || start < 0 {
		return t
	}

	var segments []*Token
	addString := func(s string) {
		if n := len(segments); n > 0 && segments[n-1].Kind == String {
			segments[n-1] = &Token{Kind: String, File: r.file, Line: t.Line, Col: t.Col, Str: segments[n-1].Str + s}
			return
		}
		segments = append(segments, &Token{Kind: String, File: r.file, Line: t.Line, Col: t.Col, Str: s})
	}

	i := 0
	for i < len(raw) {
		switch {
		case i == start:
			end := -1
			inString := false
			for i += len(openExpression); i < len(raw); i++ {
				if raw[i] == '\'' {
					inString = !inString
				} else if !inString && raw[i] == '}' && raw[i-1] == '}' {
					end = i
					i++
					break
				}
			}
			if end < start {
				r.c.Error(t, errExpressionNotClose)
				return t
			}
			e, err := r.parseExpression(t, raw[start+len(openExpression):end-1], def.allowedContext)
			if err != "" {
				r.c.Error(t, err)
				return t
			}
			if e.Kind == InsertExpression && (start != 0 || i < len(raw)) {
				r.c.Error(t, "The directive '"+InsertDirective+"' is not allowed in this context. Directives are not supported for expressions that are embedded within a string. Directives are only supported when the entire value is an expression.")
				return t
			}
			segments = append(segments, e)
			if next := strings.Index(raw[i:], openExpression); next >= 0 {
				start = i + next
			} else {
				start = -1
			}
		case start > i:
			addString(raw[i:start])
			i = start
		default:
			addString(raw[i:])
			i = len(raw)
		}
	}

	if len(segments) == 1 && segments[0].Kind == BasicExpression {
		if s, ok := expressionString(segments[0].Expr); ok {
			return &Token{Kind: String, File: r.file, Line: t.Line, Col: t.Col, Str: s}
		}
	}
	if len(segments) == 1 {
		return segments[0]
	}

	var format, args strings.Builder
	n := 0
	for _, seg := range segments {
		if seg.Kind == String {
			text := strings.ReplaceAll(seg.Str, "'", "''")
			text = strings.ReplaceAll(text, "{", "{{")
			text = strings.ReplaceAll(text, "}", "}}")
			format.WriteString(text)
			continue
		}
		format.WriteString("{" + strconv.Itoa(n) + "}")
		n++
		args.WriteString(", ")
		args.WriteString(seg.Expr)
	}
	final := "format('" + format.String() + "'" + args.String() + ")"
	if _, err := expr.ParseWith(final, expr.ParseAllowed(def.allowedContext)); err != nil {
		r.c.Error(t, err.Error())
		return t
	}
	return &Token{Kind: BasicExpression, File: r.file, Line: t.Line, Col: t.Col, Expr: final}
}

// parseExpression parses the text between "${{" and "}}". It returns an
// error message instead of a token when the text is not valid.
func (r *reader) parseExpression(t *Token, value string, allowed []string) (*Token, string) {
	trimmed := strings.TrimFunc(value, unicode.IsSpace)
	if trimmed == "" {
		return nil, errExpectedExpression
	}

	if strings.HasPrefix(trimmed, InsertDirective) &&
		(len(trimmed) == len(InsertDirective) || unicode.IsSpace(rune(trimmed[len(InsertDirective)]))) {
		if n := directiveParameters(trimmed, InsertDirective); n != 0 {
			return nil, "Exactly 0 parameter(s) were expected following the directive '" + InsertDirective + "'. Actual parameter count: " + strconv.Itoa(n)
		}
		return &Token{Kind: InsertExpression, File: r.file, Line: t.Line, Col: t.Col}, ""
	}

	if _, err := expr.ParseWith(trimmed, expr.ParseAllowed(allowed)); err != nil {
		return nil, err.Error()
	}
	return &Token{Kind: BasicExpression, File: r.file, Line: t.Line, Col: t.Col, Expr: trimmed}, ""
}

// directiveParameters counts the parameters after a directive, which are
// separated by whitespace outside of strings and parentheses.
func directiveParameters(trimmed, directive string) int {
	n := 0
	start := len(directive)
	inString := false
	parens := 0
	for i := start; i < len(trimmed); i++ {
		c := trimmed[i]
		switch {
		case unicode.IsSpace(rune(c)) && !inString && parens == 0:
			if start < i {
				n++
			}
			start = i + 1
		case c == '\'':
			inString = !inString
		case c == '(' && !inString:
			parens++
		case c == ')' && !inString:
			parens--
		}
	}
	if start < len(trimmed) {
		n++
	}
	return n
}

// expressionString returns the value of an expression that is only a string
// literal, such as ${{ '{{ not an expression }}' }}.
func expressionString(trimmed string) (string, bool) {
	var b strings.Builder
	inString := false
	for i := 0; i < len(trimmed); i++ {
		c := trimmed[i]
		switch {
		case c == '\'':
			inString = !inString
			if inString && i != 0 {
				b.WriteByte(c)
			}
		case !inString:
			return "", false
		default:
			b.WriteByte(c)
		}
	}
	return b.String(), true
}
