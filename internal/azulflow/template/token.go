// Package template is a port of GitHub's object templating: it reads YAML into
// tokens that keep their source positions, validates them against a schema,
// and evaluates them. Expressions stay unevaluated in the tokens until run
// time.
//
// The behavior follows GitHub's own parser in actions/runner
// (src/Sdk/WorkflowParser/ObjectTemplating), and is checked against GitHub's
// cross-language conformance corpus.
package template

// Kind is the type of a token. The values are those of GitHub's serialized
// tokens.
type Kind int

const (
	String           Kind = 0
	Sequence         Kind = 1
	Mapping          Kind = 2
	BasicExpression  Kind = 3
	InsertExpression Kind = 4
	Boolean          Kind = 5
	Number           Kind = 6
	Null             Kind = 7
)

// Name returns the name GitHub uses for the kind in error messages.
func (k Kind) Name() string {
	switch k {
	case String:
		return "StringToken"
	case Sequence:
		return "SequenceToken"
	case Mapping:
		return "MappingToken"
	case BasicExpression:
		return "BasicExpressionToken"
	case InsertExpression:
		return "InsertExpressionToken"
	case Boolean:
		return "BooleanToken"
	case Number:
		return "NumberToken"
	case Null:
		return "NullToken"
	}
	return "UnknownToken"
}

// InsertDirective is the directive of an insert expression, `${{ insert }}`,
// which merges a mapping into its parent.
const InsertDirective = "insert"

// Token is a node of a template. File, Line, and Col are zero for tokens that
// do not come from a file, such as defaults the converter adds.
type Token struct {
	Kind Kind
	File int
	Line int
	Col  int

	Str  string  // String: the value.
	Num  float64 // Number: the value.
	Bool bool    // Boolean: the value.
	Expr string  // BasicExpression: the expression, without "${{ }}".
	Seq  []*Token
	Map  []*Pair

	// def is the schema definition the reader matched, used to look up the
	// allowed contexts of an expression.
	def *defInfo
}

// Pair is an entry of a mapping. Its key is a string or an expression.
type Pair struct {
	Key   *Token
	Value *Token
}

// NewString returns a string token without a source position.
func NewString(s string) *Token {
	return &Token{Kind: String, Str: s}
}

// NewExpression returns an expression token without a source position.
func NewExpression(expr string) *Token {
	return &Token{Kind: BasicExpression, Expr: expr}
}

// IsLiteral reports whether the token is a string, number, boolean, or null.
func (t *Token) IsLiteral() bool {
	switch t.Kind {
	case String, Number, Boolean, Null:
		return true
	}
	return false
}

// IsExpression reports whether the token is an expression.
func (t *Token) IsExpression() bool {
	return t.Kind == BasicExpression || t.Kind == InsertExpression
}

// IsScalar reports whether the token is a literal or an expression.
func (t *Token) IsScalar() bool {
	return t.IsLiteral() || t.IsExpression()
}

// AllowedContext returns the contexts and functions that expressions in the
// token may use, as the schema declares them, such as "github" and
// "hashFiles(1,255)".
func (t *Token) AllowedContext() []string {
	if t.def == nil {
		return nil
	}
	return t.def.allowedContext
}

// String converts a literal token to a string the way GitHub does, and
// returns the "${{ }}" form of an expression.
func (t *Token) String() string {
	switch t.Kind {
	case String:
		return t.Str
	case Number:
		return FormatNumber(t.Num)
	case Boolean:
		if t.Bool {
			return "true"
		}
		return "false"
	case Null:
		return ""
	case BasicExpression:
		return "${{ " + t.Expr + " }}"
	case InsertExpression:
		return "${{ " + InsertDirective + " }}"
	}
	return ""
}

// Get returns the value of the first entry whose key is the string key.
func (t *Token) Get(key string) *Token {
	for _, p := range t.Map {
		if p.Key.Kind == String && p.Key.Str == key {
			return p.Value
		}
	}
	return nil
}

// Clone returns a deep copy of the token. With omitSource, the copy has no
// source positions.
func (t *Token) Clone(omitSource bool) *Token {
	c := *t
	if omitSource {
		c.File, c.Line, c.Col = 0, 0, 0
	}
	if t.Seq != nil {
		c.Seq = make([]*Token, len(t.Seq))
		for i, item := range t.Seq {
			c.Seq[i] = item.Clone(omitSource)
		}
	}
	if t.Map != nil {
		c.Map = make([]*Pair, len(t.Map))
		for i, p := range t.Map {
			c.Map[i] = &Pair{Key: p.Key.Clone(omitSource), Value: p.Value.Clone(omitSource)}
		}
	}
	return &c
}

// Traverse calls fn for the token and every token under it, depth first,
// including mapping keys. It stops descending when fn returns false.
func (t *Token) Traverse(fn func(*Token) bool) {
	if !fn(t) {
		return
	}
	for _, item := range t.Seq {
		item.Traverse(fn)
	}
	for _, p := range t.Map {
		p.Key.Traverse(fn)
		p.Value.Traverse(fn)
	}
}

// HasExpression reports whether the token or any token under it is an
// expression.
func (t *Token) HasExpression() bool {
	found := false
	t.Traverse(func(x *Token) bool {
		if x.IsExpression() {
			found = true
		}
		return !found
	})
	return found
}

// TypeError is the error GitHub reports when a token has an unexpected kind.
type TypeError struct {
	Token *Token
	Msg   string
}

func (e *TypeError) Error() string {
	return e.Msg
}

func (t *Token) assert(kind Kind, name, what string) error {
	if t.Kind == kind {
		return nil
	}
	return &TypeError{Token: t, Msg: "Unexpected type '" + t.Kind.Name() + "' encountered while reading '" + what + "'. The type '" + name + "' was expected."}
}

// AssertString returns an error unless the token is a string.
func (t *Token) AssertString(what string) error {
	return t.assert(String, String.Name(), what)
}

// AssertMapping returns an error unless the token is a mapping.
func (t *Token) AssertMapping(what string) error {
	return t.assert(Mapping, Mapping.Name(), what)
}

// AssertSequence returns an error unless the token is a sequence.
func (t *Token) AssertSequence(what string) error {
	return t.assert(Sequence, Sequence.Name(), what)
}

// AssertBoolean returns an error unless the token is a boolean.
func (t *Token) AssertBoolean(what string) error {
	return t.assert(Boolean, Boolean.Name(), what)
}

// AssertNumber returns an error unless the token is a number.
func (t *Token) AssertNumber(what string) error {
	return t.assert(Number, Number.Name(), what)
}

// AssertScalar returns an error unless the token is a scalar.
func (t *Token) AssertScalar(what string) error {
	if t.IsScalar() {
		return nil
	}
	return &TypeError{Token: t, Msg: "Unexpected type '" + t.Kind.Name() + "' encountered while reading '" + what + "'. The type 'ScalarToken' was expected."}
}
