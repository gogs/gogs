package gebra

import (
	"strings"
	"unicode"
)

// ParseDSL parses a model written in OpenFGA's DSL. Gebra extends the DSL with
// an "attributes" section per type, which declares the attributes that list
// queries sort by:
//
//	type repo
//	  attributes
//	    name: string
//	    updated_at: time
//	  relations
//	    define reader: [user, team#member] or writer
func ParseDSL(src string) (*AuthorizationModel, error) {
	p := &dslParser{model: &AuthorizationModel{Conditions: map[string]*Condition{}}}
	lines := strings.Split(src, "\n")
	for p.line = 0; p.line < len(lines); p.line++ {
		raw := lines[p.line]
		text := strings.TrimSpace(raw)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}
		if i := strings.Index(text, " #"); i >= 0 {
			text = strings.TrimSpace(text[:i])
		}
		fields := strings.Fields(text)
		var err error
		switch {
		case text == "model":
			p.section = ""
		case fields[0] == "schema":
			if len(fields) != 2 {
				return nil, p.errorf("want \"schema 1.1\"")
			}
			p.model.SchemaVersion = fields[1]
		case fields[0] == "type":
			if len(fields) != 2 {
				return nil, p.errorf("want \"type <name>\"")
			}
			p.typ = &TypeDefinition{Type: fields[1]}
			p.model.TypeDefinitions = append(p.model.TypeDefinitions, p.typ)
			p.section = ""
		case text == "relations" || text == "attributes":
			if p.typ == nil {
				return nil, p.errorf("%q outside of a type", text)
			}
			p.section = text
		case fields[0] == "define":
			err = p.parseDefine(strings.TrimSpace(strings.TrimPrefix(text, "define")))
		case fields[0] == "condition":
			var consumed int
			consumed, err = p.parseCondition(lines[p.line:])
			p.line += consumed
		case p.section == "attributes":
			err = p.parseAttribute(text)
		default:
			err = p.errorf("unexpected %q", text)
		}
		if err != nil {
			return nil, err
		}
	}
	if p.model.SchemaVersion == "" {
		return nil, invalidf("model is missing \"schema 1.1\"")
	}
	if len(p.model.Conditions) == 0 {
		p.model.Conditions = nil
	}
	return p.model, nil
}

type dslParser struct {
	model   *AuthorizationModel
	typ     *TypeDefinition
	section string
	line    int

	tokens []string
	pos    int
	direct []*RelationReference
}

func (p *dslParser) errorf(format string, args ...any) error {
	return invalidf("line %d: "+format, append([]any{p.line + 1}, args...)...)
}

func (p *dslParser) metadata() *Metadata {
	if p.typ.Metadata == nil {
		p.typ.Metadata = &Metadata{}
	}
	return p.typ.Metadata
}

func (p *dslParser) parseAttribute(text string) error {
	name, typ, ok := strings.Cut(text, ":")
	if !ok {
		return p.errorf("want \"<name>: <type>\" for an attribute")
	}
	md := p.metadata()
	if md.Attributes == nil {
		md.Attributes = map[string]AttributeType{}
	}
	md.Attributes[strings.TrimSpace(name)] = AttributeType(strings.TrimSpace(typ))
	return nil
}

func (p *dslParser) parseDefine(text string) error {
	if p.section != "relations" {
		return p.errorf("\"define\" outside of a relations section")
	}
	name, expr, ok := strings.Cut(text, ":")
	if !ok {
		return p.errorf("want \"define <relation>: <expression>\"")
	}
	name = strings.TrimSpace(name)
	p.tokens = tokenize(expr)
	p.pos = 0
	p.direct = nil
	u, err := p.parseExpr()
	if err != nil {
		return err
	}
	if p.pos < len(p.tokens) {
		return p.errorf("unexpected %q, use parentheses to combine different operators", p.tokens[p.pos])
	}
	if p.typ.Relations == nil {
		p.typ.Relations = map[string]*Userset{}
	}
	if _, ok := p.typ.Relations[name]; ok {
		return p.errorf("relation %q is defined twice", name)
	}
	p.typ.Relations[name] = u
	if p.direct != nil {
		md := p.metadata()
		if md.Relations == nil {
			md.Relations = map[string]*RelationMetadata{}
		}
		md.Relations[name] = &RelationMetadata{DirectlyRelatedUserTypes: p.direct}
	}
	return nil
}

func tokenize(s string) []string {
	var out []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			flush()
		case strings.ContainsRune("[](),", r):
			flush()
			out = append(out, string(r))
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func (p *dslParser) peek() string {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return ""
}

func (p *dslParser) next() string {
	t := p.peek()
	p.pos++
	return t
}

func (p *dslParser) expect(tok string) error {
	if got := p.next(); got != tok {
		return p.errorf("want %q, got %q", tok, got)
	}
	return nil
}

func (p *dslParser) parseExpr() (*Userset, error) {
	first, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	children := []*Userset{first}
	op := ""
	for tok := p.peek(); tok == "or" || tok == "and"; tok = p.peek() {
		if op != "" && op != tok {
			return nil, p.errorf("mixing %q and %q needs parentheses", op, tok)
		}
		op = tok
		p.next()
		c, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		children = append(children, c)
	}
	u := first
	switch op {
	case "or":
		u = &Userset{Union: &Usersets{Child: children}}
	case "and":
		u = &Userset{Intersection: &Usersets{Child: children}}
	}
	if p.peek() == "but" {
		p.next()
		if err := p.expect("not"); err != nil {
			return nil, err
		}
		subtract, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		u = &Userset{Difference: &Difference{Base: u, Subtract: subtract}}
	}
	return u, nil
}

func (p *dslParser) parseOperand() (*Userset, error) {
	switch tok := p.next(); tok {
	case "(":
		u, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		return u, p.expect(")")
	case "[":
		if p.direct != nil {
			return nil, p.errorf("a relation can only have one list of type restrictions")
		}
		p.direct = []*RelationReference{}
		for {
			ref, err := p.parseTypeRestriction()
			if err != nil {
				return nil, err
			}
			p.direct = append(p.direct, ref)
			if p.peek() == "]" {
				p.next()
				return &Userset{This: &struct{}{}}, nil
			}
			if err := p.expect(","); err != nil {
				return nil, err
			}
		}
	case "", ")", "]", ",", "or", "and", "but", "not", "from", "with":
		return nil, p.errorf("unexpected %q", tok)
	default:
		if p.peek() == "from" {
			p.next()
			tupleset := p.next()
			if !validName(tupleset) {
				return nil, p.errorf("invalid tupleset relation %q", tupleset)
			}
			return &Userset{TupleToUserset: &TupleToUserset{
				Tupleset:        ObjectRelation{Relation: tupleset},
				ComputedUserset: ObjectRelation{Relation: tok},
			}}, nil
		}
		return &Userset{ComputedUserset: &ObjectRelation{Relation: tok}}, nil
	}
}

func (p *dslParser) parseTypeRestriction() (*RelationReference, error) {
	tok := p.next()
	ref := &RelationReference{}
	switch {
	case strings.HasSuffix(tok, ":*"):
		ref.Type = strings.TrimSuffix(tok, ":*")
		ref.Wildcard = &struct{}{}
	case strings.Contains(tok, "#"):
		ref.Type, ref.Relation, _ = strings.Cut(tok, "#")
	default:
		ref.Type = tok
	}
	if !validName(ref.Type) {
		return nil, p.errorf("invalid type restriction %q", tok)
	}
	if p.peek() == "with" {
		p.next()
		ref.Condition = p.next()
	}
	return ref, nil
}

// parseCondition parses a condition block that starts on the first line and
// returns how many lines after the first it consumed.
func (p *dslParser) parseCondition(lines []string) (int, error) {
	var block strings.Builder
	consumed := 0
	for i, l := range lines {
		block.WriteString(l)
		block.WriteString("\n")
		consumed = i
		if strings.Contains(l, "}") {
			break
		}
	}
	text := strings.TrimSpace(block.String())
	head, rest, ok := strings.Cut(strings.TrimPrefix(text, "condition"), "{")
	if !ok || !strings.HasSuffix(rest, "}") {
		return 0, p.errorf("want \"condition <name>(<parameters>) { <expression> }\"")
	}
	expr := strings.TrimSpace(strings.TrimSuffix(rest, "}"))
	name, params, ok := strings.Cut(strings.TrimSpace(head), "(")
	if !ok || !strings.HasSuffix(strings.TrimSpace(params), ")") {
		return 0, p.errorf("condition parameters must be in parentheses")
	}
	name = strings.TrimSpace(name)
	params = strings.TrimSuffix(strings.TrimSpace(params), ")")
	c := &Condition{Name: name, Expression: expr, Parameters: map[string]*ConditionParamTypeRef{}}
	for _, param := range splitParams(params) {
		pname, ptype, ok := strings.Cut(param, ":")
		if !ok {
			return 0, p.errorf("want \"<name>: <type>\" for condition parameter %q", param)
		}
		ref, err := parseParamType(strings.TrimSpace(ptype))
		if err != nil {
			return 0, p.errorf("%v", err)
		}
		c.Parameters[strings.TrimSpace(pname)] = ref
	}
	p.model.Conditions[name] = c
	return consumed, nil
}

// splitParams splits on commas that are not inside angle brackets.
func splitParams(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if last := strings.TrimSpace(s[start:]); last != "" {
		out = append(out, last)
	}
	return out
}

func parseParamType(s string) (*ConditionParamTypeRef, error) {
	base, generic, hasGeneric := strings.Cut(s, "<")
	ref := &ConditionParamTypeRef{TypeName: "TYPE_NAME_" + strings.ToUpper(base)}
	if hasGeneric {
		inner, err := parseParamType(strings.TrimSuffix(generic, ">"))
		if err != nil {
			return nil, err
		}
		ref.GenericTypes = []*ConditionParamTypeRef{inner}
	}
	if _, err := celType(s, ref); err != nil {
		return nil, err
	}
	return ref, nil
}
