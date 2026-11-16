package expr

import (
	"math"
	"strings"

	"github.com/cockroachdb/errors"
)

// Func is a function that an Env adds to the built-in functions, such as the
// status functions on the server or hashFiles in the agent.
type Func func(args []any) (any, error)

// Env is what an expression can read. The server and the agent load different
// contexts and functions into it.
type Env struct {
	// Contexts maps lowercased context names, such as "github", to values.
	Contexts map[string]any
	// Funcs maps lowercased function names to functions. They take precedence
	// over built-in functions of the same name.
	Funcs map[string]Func
	// MaxMemory is how many bytes, as GitHub estimates them, an evaluation may
	// hold at once. Zero means 1 MiB, the default of GitHub's expression
	// library. GitHub's workflow evaluator uses 10 MiB.
	MaxMemory int
}

const defaultMaxMemory = 1 << 20

// Eval evaluates the expression. Errors are of type *Error.
func (e *Expr) Eval(env *Env) (any, error) {
	if e.root == nil {
		return nil, nil
	}
	ev := &evaluator{src: e.src, env: env, mem: memory{max: env.MaxMemory}}
	if ev.mem.max <= 0 {
		ev.mem.max = defaultMaxMemory
	}
	v, err := ev.eval(e.root, 0)
	if err != nil {
		if exprErr, ok := err.(*Error); ok {
			return nil, exprErr
		}
		return nil, &Error{Msg: err.Error(), Expr: e.src, cause: err}
	}
	return unfilter(v), nil
}

type evaluator struct {
	src string
	env *Env
	mem memory
}

// eval evaluates a node at the given depth of the tree, which GitHub uses to
// release the memory of intermediate results.
func (ev *evaluator) eval(n node, level int) (any, error) {
	if g, ok := n.(*groupNode); ok {
		return ev.eval(g.x, level)
	}
	v, err := ev.evalCore(n, level)
	if err != nil {
		return nil, err
	}
	if !ev.mem.add(level, v) {
		return nil, errorf("%s%s", msgExceededAllowedMemory, ev.src)
	}
	return v, nil
}

func (ev *evaluator) evalCore(n node, level int) (any, error) {
	switch n := n.(type) {
	case *literalNode:
		return n.value, nil

	case *contextNode:
		v, ok := ev.env.Contexts[strings.ToLower(n.name)]
		if !ok {
			return nil, tokenError(ev.src, msgUnrecognizedNamed, n.name, n.offset)
		}
		return v, nil

	case *indexNode:
		x, err := ev.eval(n.x, level+1)
		if err != nil {
			return nil, err
		}
		if n.index == nil {
			return star(x), nil
		}
		key, err := ev.eval(n.index, level+1)
		if err != nil {
			return nil, err
		}
		return index(x, key), nil

	case *callNode:
		return ev.call(n, level)

	case *notNode:
		x, err := ev.eval(n.x, level+1)
		if err != nil {
			return nil, err
		}
		return !Truthy(x), nil

	case *logicalNode:
		var v any
		for _, arg := range n.args {
			var err error
			if v, err = ev.eval(arg, level+1); err != nil {
				return nil, err
			}
			if Truthy(v) == (n.op == tokenOr) {
				break
			}
		}
		return v, nil

	case *binaryNode:
		x, err := ev.eval(n.x, level+1)
		if err != nil {
			return nil, err
		}
		y, err := ev.eval(n.y, level+1)
		if err != nil {
			return nil, err
		}
		switch n.op {
		case tokenEqualEqual:
			return Equal(x, y), nil
		case tokenBangEqual:
			return !Equal(x, y), nil
		case tokenGreater:
			return greaterThan(x, y), nil
		case tokenGreaterEqual:
			return Equal(x, y) || greaterThan(x, y), nil
		case tokenLess:
			return greaterThan(y, x), nil
		case tokenLessEqual:
			return Equal(x, y) || greaterThan(y, x), nil
		}
	}
	return nil, errorf("unexpected expression node %T", n)
}

// memory estimates the memory an evaluation holds the way GitHub does, so
// that expressions GitHub rejects as too large are rejected here too.
type memory struct {
	max       int
	total     int
	depths    []int // Bytes held by results at each depth of the tree.
	maxActive int
}

// add records the result of a node at the given depth and reports whether
// the total stays within the limit. A primitive result releases the results
// of the nodes below it, which it no longer needs.
func (m *memory) add(level int, v any) bool {
	bytes, primitive := sizeOf(v)
	if primitive {
		for ; m.maxActive > level; m.maxActive-- {
			m.total -= m.depths[m.maxActive]
			m.depths[m.maxActive] = 0
		}
	}
	for len(m.depths) <= level {
		m.depths = append(m.depths, 0)
	}
	m.maxActive = max(m.maxActive, level)
	m.depths[level] += bytes
	m.total += bytes
	return m.total <= m.max
}

const (
	minObjectSize      = 24
	stringBaseOverhead = 26
	pointerSize        = 8
)

func sizeOf(v any) (bytes int, primitive bool) {
	switch v := v.(type) {
	case string:
		return stringSize(v), true
	case nil, bool, float64:
		return minObjectSize, true
	case *filtered:
		return minObjectSize + pointerSize*len(v.items), false
	}
	return minObjectSize, false
}

func stringSize(s string) int {
	return stringBaseOverhead + 2*utf16Len(s)
}

// memoryCounter limits the size of a string that a function builds.
type memoryCounter struct {
	max, bytes int
	src        string // The source of the function call.
}

func (c *memoryCounter) add(s string) error {
	c.bytes += stringSize(s)
	if c.max > 0 && c.bytes > c.max {
		return errorf("%s%s", msgExceededAllowedMemory, c.src)
	}
	return nil
}

func index(x, key any) any {
	switch x := x.(type) {
	case *filtered:
		out := &filtered{}
		for _, item := range x.items {
			switch item := item.(type) {
			case *Object:
				if s, ok := stringIndex(key); ok {
					if v, ok := item.Get(s); ok {
						out.items = append(out.items, v)
					}
				}
			case []any, *filtered:
				arr, _ := asArray(item)
				if i, ok := intIndex(key); ok && i < len(arr) {
					out.items = append(out.items, arr[i])
				}
			}
		}
		return out
	case *Object:
		if s, ok := stringIndex(key); ok {
			v, _ := x.Get(s)
			return v
		}
	case []any:
		if i, ok := intIndex(key); ok && i < len(x) {
			return x[i]
		}
	}
	return nil
}

func stringIndex(key any) (string, bool) {
	if !isPrimitive(key) {
		return "", false
	}
	return ToString(key), true
}

func intIndex(key any) (int, bool) {
	f := ToNumber(key)
	if math.IsNaN(f) || f < 0 {
		return 0, false
	}
	f = math.Floor(f)
	if f > math.MaxInt32 {
		return 0, false
	}
	return int(f), true
}

func star(x any) *filtered {
	out := &filtered{}
	switch x := x.(type) {
	case []any:
		out.items = append(out.items, x...)
	case *Object:
		out.items = append(out.items, x.values...)
	case *filtered:
		for _, item := range x.items {
			switch item := item.(type) {
			case []any:
				out.items = append(out.items, item...)
			case *filtered:
				out.items = append(out.items, item.items...)
			case *Object:
				out.items = append(out.items, item.values...)
			}
		}
	}
	return out
}

func (ev *evaluator) call(n *callNode, level int) (any, error) {
	arg := func(i int) (any, error) {
		return ev.eval(n.args[i], level+1)
	}
	if fn, ok := ev.env.Funcs[n.name]; ok {
		args := make([]any, len(n.args))
		for i := range n.args {
			v, err := arg(i)
			if err != nil {
				return nil, err
			}
			args[i] = unfilter(v)
		}
		return fn(args)
	}
	b, ok := builtins[n.name]
	if !ok {
		return nil, tokenError(ev.src, msgUnrecognizedFunction, n.raw, n.offset)
	}
	return b.fn(&funcCall{n: n, arg: arg, counter: memoryCounter{max: ev.mem.max, src: n.src}})
}

// funcCall is a call of a built-in function. Built-in functions evaluate their
// arguments only when they need them, as GitHub does.
type funcCall struct {
	n       *callNode
	arg     func(i int) (any, error)
	counter memoryCounter
}

func (c *funcCall) nargs() int {
	return len(c.n.args)
}

type builtin struct {
	minArgs, maxArgs int
	fn               func(c *funcCall) (any, error)
}

var builtins = map[string]builtin{
	"case":       {3, 255, fnCase},
	"contains":   {2, 2, fnContains},
	"endswith":   {2, 2, fnEndsWith},
	"format":     {1, 255, fnFormat},
	"fromjson":   {1, 1, fnFromJSON},
	"join":       {1, 2, fnJoin},
	"startswith": {2, 2, fnStartsWith},
	"tojson":     {1, 1, fnToJSON},
}

func fnCase(c *funcCall) (any, error) {
	last := c.nargs() - 1
	for i := 0; i < last; i += 2 {
		pred, err := c.arg(i)
		if err != nil {
			return nil, err
		}
		b, ok := pred.(bool)
		if !ok {
			return nil, errorf("case predicate must evaluate to a boolean value")
		}
		if b {
			return c.arg(i + 1)
		}
	}
	return c.arg(last)
}

func fnContains(c *funcCall) (any, error) {
	left, err := c.arg(0)
	if err != nil {
		return nil, err
	}
	if isPrimitive(left) {
		right, err := c.arg(1)
		if err != nil || !isPrimitive(right) {
			return false, err
		}
		return strings.Contains(upper(ToString(left)), upper(ToString(right))), nil
	}
	arr, ok := asArray(left)
	if !ok || len(arr) == 0 {
		return false, nil
	}
	right, err := c.arg(1)
	if err != nil {
		return nil, err
	}
	for _, item := range arr {
		if Equal(right, item) {
			return true, nil
		}
	}
	return false, nil
}

func fnStartsWith(c *funcCall) (any, error) {
	return compareAffix(c, strings.HasPrefix)
}

func fnEndsWith(c *funcCall) (any, error) {
	return compareAffix(c, strings.HasSuffix)
}

func compareAffix(c *funcCall, has func(s, affix string) bool) (any, error) {
	left, err := c.arg(0)
	if err != nil || !isPrimitive(left) {
		return false, err
	}
	right, err := c.arg(1)
	if err != nil || !isPrimitive(right) {
		return false, err
	}
	return has(upper(ToString(left)), upper(ToString(right))), nil
}

func fnFormat(c *funcCall) (any, error) {
	f, err := c.arg(0)
	if err != nil {
		return nil, err
	}
	format := ToString(f)
	invalid := func() error {
		return errorf("The following format string is invalid: %s", format)
	}

	// The format string is checked in full before any argument is evaluated.
	type segment struct {
		literal    string
		arg        int // Index of the argument, or -1 for a literal.
		specifiers string
	}
	var segments []segment
	for i := 0; i < len(format); {
		lbrace := indexFrom(format, '{', i)
		rbrace := indexFrom(format, '}', i)
		switch {
		case lbrace >= 0 && (rbrace < 0 || rbrace > lbrace):
			if charAt(format, lbrace+1) == '{' {
				segments = append(segments, segment{literal: format[i : lbrace+1], arg: -1})
				i = lbrace + 2
				continue
			}
			if rbrace <= lbrace+1 {
				return nil, invalid()
			}
			argIndex, end, ok := readArgIndex(format, lbrace+1)
			if !ok {
				return nil, invalid()
			}
			specifiers, rbrace, ok := readFormatSpecifiers(format, end)
			if !ok {
				return nil, invalid()
			}
			if argIndex > c.nargs()-2 {
				return nil, errorf("The following format string references more arguments than were supplied: %s", format)
			}
			if lbrace > i {
				segments = append(segments, segment{literal: format[i:lbrace], arg: -1})
			}
			segments = append(segments, segment{arg: argIndex, specifiers: specifiers})
			i = rbrace + 1
		case rbrace >= 0:
			if charAt(format, rbrace+1) != '}' {
				return nil, invalid()
			}
			segments = append(segments, segment{literal: format[i : rbrace+1], arg: -1})
			i = rbrace + 2
		default:
			segments = append(segments, segment{literal: format[i:], arg: -1})
			i = len(format)
		}
	}

	args := make(map[int]any)
	var b strings.Builder
	for _, s := range segments {
		str := s.literal
		if s.arg >= 0 {
			v, ok := args[s.arg]
			if !ok {
				if v, err = c.arg(s.arg + 1); err != nil {
					return nil, err
				}
				args[s.arg] = v
			}
			if s.specifiers != "" {
				return nil, errorf("The format specifiers '%s' are not valid for objects of type '%s'", s.specifiers, kindName(v))
			}
			str = ToString(v)
		}
		if str == "" {
			continue
		}
		if err := c.counter.add(str); err != nil {
			return nil, err
		}
		b.WriteString(str)
	}
	return b.String(), nil
}

func indexFrom(s string, c byte, from int) int {
	i := strings.IndexByte(s[from:], c)
	if i < 0 {
		return -1
	}
	return from + i
}

func charAt(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

// readArgIndex reads the argument index of a placeholder, which GitHub
// limits to 255, and returns the position after it.
func readArgIndex(s string, start int) (index, end int, ok bool) {
	for end = start; end < len(s) && isDigit(s[end]); end++ {
		index = min(index*10+int(s[end]-'0'), 256)
	}
	return index, end, end > start && index <= 255
}

// readFormatSpecifiers reads the optional ":specifiers" of a placeholder and
// returns the position of its closing brace.
func readFormatSpecifiers(s string, start int) (specifiers string, rbrace int, ok bool) {
	switch charAt(s, start) {
	case '}':
		return "", start, true
	case ':':
	default:
		return "", 0, false
	}
	var b strings.Builder
	for i := start + 1; i < len(s); {
		switch {
		case s[i] != '}':
			b.WriteByte(s[i])
			i++
		case charAt(s, i+1) == '}':
			b.WriteByte('}')
			i += 2
		default:
			return b.String(), i, true
		}
	}
	return "", 0, false
}

func fnJoin(c *funcCall) (any, error) {
	items, err := c.arg(0)
	if err != nil {
		return nil, err
	}
	arr, ok := asArray(items)
	if !ok || len(arr) == 0 {
		if isPrimitive(items) {
			return ToString(items), nil
		}
		return "", nil
	}
	sep := ","
	if len(arr) > 1 && c.nargs() > 1 {
		s, err := c.arg(1)
		if err != nil {
			return nil, err
		}
		if isPrimitive(s) {
			sep = ToString(s)
		}
	}
	var b strings.Builder
	for i, item := range arr {
		if i > 0 {
			if err := c.counter.add(sep); err != nil {
				return nil, err
			}
			b.WriteString(sep)
		}
		str := ToString(item)
		if err := c.counter.add(str); err != nil {
			return nil, err
		}
		b.WriteString(str)
	}
	return b.String(), nil
}

func fnToJSON(c *funcCall) (any, error) {
	v, err := c.arg(0)
	if err != nil {
		return nil, err
	}
	return toJSON(v, &c.counter)
}

func fnFromJSON(c *funcCall) (any, error) {
	v, err := c.arg(0)
	if err != nil {
		return nil, err
	}
	out, err := FromJSON(ToString(v))
	if err != nil {
		return nil, errors.Wrap(err, "Error parsing fromJson")
	}
	return out, nil
}
