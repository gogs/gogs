package template

import (
	"strconv"
	"unicode/utf16"

	"github.com/cockroachdb/errors"
)

// Limits are the limits GitHub applies while reading a workflow.
type Limits struct {
	MaxDepth  int // Nesting of sequences and mappings.
	MaxEvents int // Values read.
	MaxBytes  int // Estimated memory of the result.
}

// DefaultLimits returns the limits GitHub uses for workflow files.
func DefaultLimits() Limits {
	return Limits{MaxDepth: 50, MaxEvents: 1000000, MaxBytes: 10 * 1024 * 1024}
}

const (
	maxErrors        = 10
	maxErrorLength   = 500
	minObjectSize    = 24
	stringOverhead   = 26
	errMaxDepth      = "Maximum object depth exceeded"
	errMaxEvents     = "Maximum events exceeded while evaluating the template. This may indicate an infinite loop or too many nested loops."
	errMaxObjectSize = "Maximum object size exceeded"
)

// Context is the state shared while reading and converting the files of one
// workflow: the file names, the errors, and the memory used.
type Context struct {
	Schema *Schema

	// BackgroundSteps enables the experimental background steps.
	BackgroundSteps bool

	files  []string
	errors []string

	limits Limits
	depth  int
	events int
	bytes  int
}

// NewContext returns a context that validates against schema.
func NewContext(schema *Schema, limits Limits) *Context {
	return &Context{Schema: schema, limits: limits}
}

// FileID returns the ID of a file, adding it on first use. IDs start at 1.
func (c *Context) FileID(name string) int {
	for i, f := range c.files {
		if f == name {
			return i + 1
		}
	}
	c.files = append(c.files, name)
	// The file name counts toward the memory used.
	c.bytes += stringBytes(name)
	return len(c.files)
}

// FileName returns the name of a file ID, or "" when unknown.
func (c *Context) FileName(id int) string {
	if id < 1 || id > len(c.files) {
		return ""
	}
	return c.files[id-1]
}

// Files returns the names of the files read, in the order of their IDs.
func (c *Context) Files() []string {
	return c.files
}

// Errors returns the errors reported, at most 10.
func (c *Context) Errors() []string {
	return c.errors
}

// PrefixError puts prefix and a space before error i.
func (c *Context) PrefixError(i int, prefix string) {
	if c.errors[i] == "" {
		c.errors[i] = prefix
		return
	}
	c.errors[i] = prefix + " " + c.errors[i]
}

// prefix returns the location prefix GitHub puts before an error message.
func (c *Context) prefix(file, line, col int) string {
	name := c.FileName(file)
	pos := ""
	if line > 0 && col > 0 {
		pos = "(Line: " + strconv.Itoa(line) + ", Col: " + strconv.Itoa(col) + ")"
	}
	switch {
	case name != "" && pos != "":
		return name + " " + pos + ":"
	case name != "":
		return name + ":"
	case pos != "":
		return pos + ":"
	}
	return ""
}

// Error reports an error at the position of a token, which may be nil.
func (c *Context) Error(t *Token, msg string) {
	if t == nil {
		c.ErrorAt(0, 0, 0, msg)
		return
	}
	c.ErrorAt(t.File, t.Line, t.Col, msg)
}

// ErrorAt reports an error at a position. Zero values omit the parts of the
// position.
func (c *Context) ErrorAt(file, line, col int, msg string) {
	if p := c.prefix(file, line, col); p != "" {
		msg = p + " " + msg
	}
	c.AddError(msg)
}

// AddError reports an error message as is. As in GitHub, only the first 10
// errors are kept, and each is cut to 500 characters.
func (c *Context) AddError(msg string) {
	if len(c.errors) >= maxErrors {
		return
	}
	if units := utf16.Encode([]rune(msg)); len(units) > maxErrorLength {
		msg = string(utf16.Decode(units[:maxErrorLength])) + "[...]"
	}
	c.errors = append(c.errors, msg)
}

// abort stops reading, as an exception does in GitHub's parser.
type abort struct {
	err error
}

// catch recovers from an abort and reports its error at the start of a file.
func (c *Context) catch(file int) {
	if r := recover(); r != nil {
		a, ok := r.(abort)
		if !ok {
			panic(r)
		}
		c.ErrorAt(file, 0, 0, a.err.Error())
	}
}

func (c *Context) incrementDepth() {
	c.depth++
	if c.depth > c.limits.MaxDepth {
		panic(abort{errors.New(errMaxDepth)})
	}
}

func (c *Context) decrementDepth() {
	c.depth--
}

func (c *Context) incrementEvents() {
	c.events++
	if c.events > c.limits.MaxEvents {
		panic(abort{errors.New(errMaxEvents)})
	}
}

// AddBytes adds to the estimated memory used, and aborts when it exceeds the
// limit.
func (c *Context) AddBytes(n int) {
	c.bytes += n
	if c.bytes > c.limits.MaxBytes {
		panic(abort{errors.New(errMaxObjectSize)})
	}
}

// Bytes returns the estimated memory used.
func (c *Context) Bytes() int {
	return c.bytes
}

// Run calls fn, and reports an abort from it as an error of the file.
func (c *Context) Run(file int, fn func()) {
	defer c.catch(file)
	fn()
}

// stringBytes estimates the memory of a string, as GitHub does.
func stringBytes(s string) int {
	return stringOverhead + 2*len(utf16.Encode([]rune(s)))
}

// TokenBytes estimates the memory of a token, and with traverse, of the
// tokens under it.
func TokenBytes(t *Token, traverse bool) int {
	if !traverse {
		return tokenBytes(t)
	}
	n := 0
	t.Traverse(func(x *Token) bool {
		n += tokenBytes(x)
		return true
	})
	return n
}

func tokenBytes(t *Token) int {
	switch t.Kind {
	case String:
		return minObjectSize + stringBytes(t.Str)
	case BasicExpression:
		return minObjectSize + stringBytes(t.Expr)
	}
	return minObjectSize
}
