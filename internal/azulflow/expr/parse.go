package expr

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
)

func errorf(format string, args ...any) error {
	return errors.Newf(format, args...)
}

type node interface{}

type (
	literalNode struct{ value any }
	contextNode struct {
		name   string
		offset int
	}
	// groupNode is a parenthesized expression. Unlike literals, it can be
	// indexed.
	groupNode struct{ x node }
	indexNode struct {
		x     node
		index node // Nil for a wildcard, as in "x.*" and "x[*]".
	}
	callNode struct {
		name   string // Lowercased.
		raw    string
		offset int
		src    string // The source of the whole call, for error messages.
		args   []node
	}
	notNode    struct{ x node }
	binaryNode struct {
		op   tokenType
		x, y node
	}
	logicalNode struct {
		op   tokenType // tokenAnd or tokenOr.
		args []node
	}
)

// Expr is a parsed expression.
type Expr struct {
	src  string
	root node // Nil for an empty expression, which evaluates to null.
}

// String returns the source of the expression.
func (e *Expr) String() string {
	return e.src
}

// ParseOptions limits what an expression may use. Nil Contexts allows any
// named-value.
type ParseOptions struct {
	// Contexts are the allowed named-values, e.g. "github" and "inputs".
	Contexts []string
	// Funcs are the functions allowed in addition to the built-ins.
	Funcs []FuncSig
}

// FuncSig is the name and number of parameters of a function. Max -1 means
// unlimited.
type FuncSig struct {
	Name     string
	Min, Max int
}

var allowedFuncPattern = regexp.MustCompile(`^([a-zA-Z0-9_]+)\(([0-9]+),([0-9]+|MAX)\)$`)

// ParseAllowed converts a list of allowed contexts from GitHub's workflow
// schema, such as "github", "hashFiles(1,255)" and "success(0,MAX)", into
// ParseOptions. As in GitHub's parser, entries that do not have the form
// "name(min,max)" are named-values.
func ParseAllowed(allowed []string) ParseOptions {
	opts := ParseOptions{Contexts: []string{}}
	for _, item := range allowed {
		m := allowedFuncPattern.FindStringSubmatch(item)
		if m == nil {
			opts.Contexts = append(opts.Contexts, item)
			continue
		}
		minArgs, errMin := strconv.Atoi(m[2])
		maxArgs, errMax := strconv.Atoi(m[3])
		if m[3] == "MAX" {
			maxArgs, errMax = -1, nil
		}
		if errMin != nil || errMax != nil {
			opts.Contexts = append(opts.Contexts, item)
			continue
		}
		opts.Funcs = append(opts.Funcs, FuncSig{Name: m[1], Min: minArgs, Max: maxArgs})
	}
	return opts
}

// Parse parses an expression, without the surrounding "${{ }}". It allows any
// named-value and any function, and checks the number of parameters of
// built-in functions only. Errors wrap an *Error.
func Parse(src string) (*Expr, error) {
	e, err := parse(src, nil, nil, true)
	if err != nil {
		return nil, errors.Wrapf(err, "parse expression %q", src)
	}
	return e, nil
}

// ParseWith parses an expression, without the surrounding "${{ }}", allowing
// only the given named-values and functions. Errors are of type *Error.
func ParseWith(src string, opts ParseOptions) (*Expr, error) {
	funcs := make(map[string]FuncSig, len(opts.Funcs))
	for _, f := range opts.Funcs {
		funcs[strings.ToLower(f.Name)] = f
	}
	var contexts map[string]bool
	if opts.Contexts != nil {
		contexts = make(map[string]bool, len(opts.Contexts))
		for _, c := range opts.Contexts {
			contexts[strings.ToLower(c)] = true
		}
	}
	return parse(src, contexts, funcs, false)
}

func parse(src string, contexts map[string]bool, funcs map[string]FuncSig, allowUnknownFuncs bool) (*Expr, error) {
	tokens, err := lex(src)
	if err != nil {
		return nil, err
	}
	p := &parser{src: src, tokens: tokens, contexts: contexts, funcs: funcs, allowUnknownFuncs: allowUnknownFuncs}
	root, err := p.parse()
	if err != nil {
		return nil, err
	}
	return &Expr{src: src, root: root}, nil
}

// parser is a port of the recursive descent parser of GitHub's TypeScript
// implementation, so that errors name the same tokens.
type parser struct {
	src               string
	tokens            []token
	pos               int
	depth             int
	contexts          map[string]bool // Nil allows any named-value.
	funcs             map[string]FuncSig
	allowUnknownFuncs bool
}

func (p *parser) parse() (node, error) {
	if p.atEnd() {
		return nil, nil
	}
	root, err := p.expression()
	if err != nil {
		return nil, err
	}
	if !p.atEnd() {
		return nil, p.error(msgUnexpectedSymbol, p.peek())
	}
	return root, nil
}

func (p *parser) expression() (node, error) {
	if err := p.incrDepth(); err != nil {
		return nil, err
	}
	defer p.decrDepth()
	return p.logical(tokenOr, func() (node, error) {
		return p.logical(tokenAnd, p.equality)
	})
}

func (p *parser) logical(op tokenType, operand func() (node, error)) (node, error) {
	x, err := operand()
	if err != nil || !p.check(op) {
		return x, err
	}
	if err := p.incrDepth(); err != nil {
		return nil, err
	}
	defer p.decrDepth()
	n := &logicalNode{op: op, args: []node{x}}
	for p.match(op) {
		y, err := operand()
		if err != nil {
			return nil, err
		}
		n.args = append(n.args, y)
	}
	return n, nil
}

func (p *parser) equality() (node, error) {
	return p.binary(p.comparison, tokenBangEqual, tokenEqualEqual)
}

func (p *parser) comparison() (node, error) {
	return p.binary(p.unary, tokenGreater, tokenGreaterEqual, tokenLess, tokenLessEqual)
}

func (p *parser) binary(operand func() (node, error), ops ...tokenType) (node, error) {
	x, err := operand()
	if err != nil {
		return nil, err
	}
	for p.match(ops...) {
		op := p.previous().typ
		y, err := operand()
		if err != nil {
			return nil, err
		}
		x = &binaryNode{op: op, x: x, y: y}
	}
	return x, nil
}

func (p *parser) unary() (node, error) {
	if !p.match(tokenBang) {
		return p.index()
	}
	if err := p.incrDepth(); err != nil {
		return nil, err
	}
	defer p.decrDepth()
	x, err := p.unary()
	if err != nil {
		return nil, err
	}
	return &notNode{x: x}, nil
}

func (p *parser) index() (node, error) {
	x, err := p.call()
	if err != nil {
		return nil, err
	}
	switch x.(type) {
	case *groupNode, *callNode, *contextNode:
	default:
		return x, nil
	}
	depth := p.depth
	defer func() { p.depth = depth }()
	for {
		switch {
		case p.match(tokenLeftBracket):
			var index node
			if !p.match(tokenStar) {
				if index, err = p.expression(); err != nil {
					return nil, err
				}
			}
			if !p.match(tokenRightBracket) {
				return nil, p.error(msgUnexpectedSymbol, p.peek())
			}
			if err := p.incrDepth(); err != nil {
				return nil, err
			}
			x = &indexNode{x: x, index: index}

		case p.match(tokenDot):
			if err := p.incrDepth(); err != nil {
				return nil, err
			}
			switch {
			case p.match(tokenIdentifier):
				x = &indexNode{x: x, index: &literalNode{value: p.previous().lexeme}}
			case p.match(tokenStar):
				x = &indexNode{x: x}
			default:
				return nil, p.error(msgUnexpectedSymbol, p.peek())
			}

		default:
			return x, nil
		}
	}
}

func (p *parser) call() (node, error) {
	if !p.check(tokenIdentifier) {
		return p.primary()
	}
	ident := p.next()
	if !p.match(tokenLeftParen) {
		if p.contexts != nil && !p.contexts[strings.ToLower(ident.lexeme)] {
			return nil, p.error(msgUnrecognizedNamed, ident)
		}
		return &contextNode{name: ident.lexeme, offset: ident.offset}, nil
	}
	n := &callNode{name: strings.ToLower(ident.lexeme), raw: ident.lexeme, offset: ident.offset}
	for !p.match(tokenRightParen) {
		arg, err := p.expression()
		if err != nil {
			return nil, err
		}
		n.args = append(n.args, arg)
		if p.atEnd() {
			return nil, p.error(msgUnexpectedEnd, p.previous())
		}
		if !p.check(tokenRightParen) && !p.match(tokenComma) {
			return nil, p.error(msgUnexpectedSymbol, p.peek())
		}
	}
	end := p.previous()
	n.src = p.src[ident.offset : end.offset+len(end.lexeme)]
	return n, p.validateCall(n, ident)
}

func (p *parser) validateCall(n *callNode, ident token) error {
	minArgs, maxArgs := 0, 0
	if b, ok := builtins[n.name]; ok {
		minArgs, maxArgs = b.minArgs, b.maxArgs
	} else if f, ok := p.funcs[n.name]; ok {
		minArgs, maxArgs = f.Min, f.Max
	} else if p.allowUnknownFuncs {
		return nil
	} else {
		return p.error(msgUnrecognizedFunction, ident)
	}
	switch {
	case len(n.args) < minArgs:
		return p.error(msgTooFewParameters, ident)
	case maxArgs >= 0 && len(n.args) > maxArgs:
		return p.error(msgTooManyParameters, ident)
	case n.name == "case" && len(n.args)%2 == 0:
		return p.error(msgEvenParameters, ident)
	}
	return nil
}

func (p *parser) primary() (node, error) {
	switch {
	case p.match(tokenFalse):
		return &literalNode{value: false}, nil
	case p.match(tokenTrue):
		return &literalNode{value: true}, nil
	case p.match(tokenNull):
		return &literalNode{value: nil}, nil
	case p.match(tokenNumber):
		return &literalNode{value: p.previous().num}, nil
	case p.match(tokenString):
		return &literalNode{value: p.previous().str}, nil
	case p.match(tokenLeftParen):
		x, err := p.expression()
		if err != nil {
			return nil, err
		}
		if p.atEnd() {
			return nil, p.error(msgUnexpectedEnd, p.previous())
		}
		if !p.match(tokenRightParen) {
			return nil, p.error(msgUnexpectedSymbol, p.peek())
		}
		return &groupNode{x: x}, nil
	case p.atEnd():
		return nil, p.error(msgUnexpectedEnd, p.previous())
	}
	return nil, p.error(msgUnexpectedSymbol, p.peek())
}

func (p *parser) match(types ...tokenType) bool {
	for _, typ := range types {
		if p.check(typ) {
			p.next()
			return true
		}
	}
	return false
}

func (p *parser) check(typ tokenType) bool {
	return !p.atEnd() && p.peek().typ == typ
}

func (p *parser) atEnd() bool {
	return p.peek().typ == tokenEOF
}

func (p *parser) next() token {
	if !p.atEnd() {
		p.pos++
	}
	return p.previous()
}

func (p *parser) peek() token {
	return p.tokens[p.pos]
}

func (p *parser) previous() token {
	return p.tokens[p.pos-1]
}

func (p *parser) incrDepth() error {
	p.depth++
	if p.depth > maxDepth {
		return &Error{Msg: msgExceededMaxDepth, Pos: utf16Len(p.src[:p.peek().offset]) + 1, Expr: p.src}
	}
	return nil
}

func (p *parser) decrDepth() {
	p.depth--
}

func (p *parser) error(msg string, t token) *Error {
	return tokenError(p.src, msg, t.String(), t.offset)
}

// NamedValues returns the lowercased names of the contexts the expression
// reads, such as "github" and "needs".
func (e *Expr) NamedValues() []string {
	var names []string
	seen := map[string]bool{}
	walk(e.root, func(n node) {
		if n, ok := n.(*contextNode); ok {
			name := strings.ToLower(n.name)
			if !seen[name] {
				seen[name] = true
				names = append(names, name)
			}
		}
	})
	return names
}

// Functions returns the lowercased names of the functions the expression calls.
func (e *Expr) Functions() []string {
	var names []string
	seen := map[string]bool{}
	walk(e.root, func(n node) {
		if n, ok := n.(*callNode); ok && !seen[n.name] {
			seen[n.name] = true
			names = append(names, n.name)
		}
	})
	return names
}

func walk(n node, fn func(node)) {
	if n == nil {
		return
	}
	fn(n)
	switch n := n.(type) {
	case *groupNode:
		walk(n.x, fn)
	case *indexNode:
		walk(n.x, fn)
		walk(n.index, fn)
	case *callNode:
		for _, arg := range n.args {
			walk(arg, fn)
		}
	case *notNode:
		walk(n.x, fn)
	case *binaryNode:
		walk(n.x, fn)
		walk(n.y, fn)
	case *logicalNode:
		for _, arg := range n.args {
			walk(arg, fn)
		}
	}
}
