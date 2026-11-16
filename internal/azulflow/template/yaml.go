package template

import (
	"bytes"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	yaml "go.yaml.in/yaml/v3"
)

var (
	errExpectedDocumentEnd = errors.New("Expected document end parse event")
	errExpectedMappingEnd  = errors.New("Expected mapping end")
	errExpectedValue       = errors.New("Expected a scalar value, a sequence, or a mapping")
)

// yamlError returns the message of a YAML syntax error without the "yaml: "
// prefix of the decoder.
func yamlError(err error) error {
	return errors.New(strings.TrimPrefix(err.Error(), "yaml: "))
}

type eventKind int

const (
	evLiteral eventKind = iota
	evSequenceStart
	evSequenceEnd
	evMappingStart
	evMappingEnd
	evDocumentEnd
	evError // Reading stops here with err.
)

type event struct {
	kind  eventKind
	token *Token
	err   error
}

// maxYAMLNodes limits the nodes read, which bounds documents that expand
// aliases many times.
const maxYAMLNodes = 50000

// yamlReader turns a YAML document into the stream of events that the
// template reader consumes, with GitHub's rules: plain scalars are typed with
// the YAML 1.2 core schema, explicit tags are checked, and anchors are only
// allowed when enabled. Problems become an error event at the position GitHub
// would detect them, so validation errors before it are still reported.
type yamlReader struct {
	file         int
	allowAnchors bool
	events       []event
	pos          int
	nodes        int
}

func newYAMLReader(file int, content []byte, allowAnchors bool) (*yamlReader, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			// A file without a document has no document start event.
			r := &yamlReader{file: file}
			r.fail(errors.New("Expected document start parse event"))
			return r, nil
		}
		return nil, err
	}
	r := &yamlReader{file: file, allowAnchors: allowAnchors}
	if len(doc.Content) == 0 {
		r.events = append(r.events, event{kind: evLiteral, token: &Token{Kind: Null, File: file, Line: doc.Line, Col: doc.Column}})
	} else if !r.walk(doc.Content[0]) {
		return r, nil
	}
	var next yaml.Node
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		r.events = append(r.events, event{kind: evError, err: errors.New("Expected stream end parse event")})
		return r, nil
	}
	r.events = append(r.events, event{kind: evDocumentEnd})
	return r, nil
}

// walk appends the events of a node, and returns false when it appended an
// error event.
func (r *yamlReader) walk(n *yaml.Node) bool {
	if r.allowAnchors {
		r.nodes++
		if r.nodes > maxYAMLNodes {
			return r.fail(errors.New("Maximum YAML nodes exceeded"))
		}
	}
	if n.Anchor != "" && !r.allowAnchors {
		return r.fail(errors.Newf("Anchors are not currently supported. Remove the anchor '%s'", n.Anchor))
	}
	switch n.Kind {
	case yaml.AliasNode:
		if !r.allowAnchors {
			return r.fail(errors.New("Unexpected parsing event type: AnchorAlias"))
		}
		return r.walk(n.Alias)
	case yaml.ScalarNode:
		t, err := r.literal(n)
		if err != nil {
			return r.fail(err)
		}
		r.events = append(r.events, event{kind: evLiteral, token: t})
		return true
	case yaml.SequenceNode:
		r.events = append(r.events, event{kind: evSequenceStart, token: &Token{Kind: Sequence, File: r.file, Line: n.Line, Col: n.Column}})
		for _, item := range n.Content {
			if !r.walk(item) {
				return false
			}
		}
		r.events = append(r.events, event{kind: evSequenceEnd})
		return true
	case yaml.MappingNode:
		r.events = append(r.events, event{kind: evMappingStart, token: &Token{Kind: Mapping, File: r.file, Line: n.Line, Col: n.Column}})
		for _, item := range n.Content {
			if !r.walk(item) {
				return false
			}
		}
		r.events = append(r.events, event{kind: evMappingEnd})
		return true
	}
	return r.fail(errors.Newf("Unexpected parsing event type: %v", n.Kind))
}

// addToken appends the events of a token, with copies of its tokens.
func (r *yamlReader) addToken(t *Token) {
	c := *t
	c.Seq, c.Map, c.def = nil, nil, nil
	switch t.Kind {
	case Sequence:
		r.events = append(r.events, event{kind: evSequenceStart, token: &c})
		for _, item := range t.Seq {
			r.addToken(item)
		}
		r.events = append(r.events, event{kind: evSequenceEnd})
	case Mapping:
		r.events = append(r.events, event{kind: evMappingStart, token: &c})
		for _, p := range t.Map {
			r.addToken(p.Key)
			r.addToken(p.Value)
		}
		r.events = append(r.events, event{kind: evMappingEnd})
	default:
		r.events = append(r.events, event{kind: evLiteral, token: &c})
	}
}

func (r *yamlReader) fail(err error) bool {
	r.events = append(r.events, event{kind: evError, err: err})
	return false
}

const (
	tagBool  = "tag:yaml.org,2002:bool"
	tagFloat = "tag:yaml.org,2002:float"
	tagInt   = "tag:yaml.org,2002:int"
	tagNull  = "tag:yaml.org,2002:null"
	tagStr   = "tag:yaml.org,2002:str"
)

func longTag(tag string) string {
	if rest, ok := strings.CutPrefix(tag, "!!"); ok {
		return "tag:yaml.org,2002:" + rest
	}
	return tag
}

func styleName(n *yaml.Node) string {
	switch {
	case n.Style&yaml.DoubleQuotedStyle != 0:
		return "DoubleQuoted"
	case n.Style&yaml.SingleQuotedStyle != 0:
		return "SingleQuoted"
	case n.Style&yaml.LiteralStyle != 0:
		return "Literal"
	case n.Style&yaml.FoldedStyle != 0:
		return "Folded"
	}
	return "Plain"
}

func (r *yamlReader) literal(n *yaml.Node) (*Token, error) {
	t := &Token{File: r.file, Line: n.Line, Col: n.Column}
	plain := styleName(n) == "Plain"

	if n.Style&yaml.TaggedStyle != 0 {
		tag := longTag(n.Tag)
		if tag == tagStr {
			t.Kind, t.Str = String, n.Value
			return t, nil
		}
		if !plain {
			return nil, errors.Newf("The scalar style '%s' on line %d and column %d is not valid with the tag '%s'", styleName(n), n.Line, n.Column, tag)
		}
		var ok bool
		var err error
		switch tag {
		case tagBool:
			ok = matchBoolean(n.Value, t)
		case tagFloat:
			ok = matchFloat(n.Value, t)
		case tagInt:
			ok, err = matchInteger(n.Value, t)
		case tagNull:
			ok = matchNull(n.Value, t)
		default:
			return nil, errors.Newf("Unexpected tag '%s'", tag)
		}
		if err == nil && !ok {
			err = errInvalidValue(n.Value, n.Line, n.Column, tag)
		}
		if err != nil {
			return nil, withPos(err, n)
		}
		return t, nil
	}

	if plain {
		if matchNull(n.Value, t) || matchBoolean(n.Value, t) {
			return t, nil
		}
		if ok, err := matchInteger(n.Value, t); ok || err != nil {
			return t, withPos(err, n)
		}
		if matchFloat(n.Value, t) {
			return t, nil
		}
	}
	t.Kind, t.Str = String, n.Value
	return t, nil
}

type invalidValue struct {
	value, tag string
}

func (e *invalidValue) Error() string {
	return "invalid " + e.tag
}

func errInvalidValue(value string, line, col int, tag string) error {
	return errors.Newf("The value '%s' on line %d and column %d is invalid for the type '%s'", value, line, col, tag)
}

func withPos(err error, n *yaml.Node) error {
	var iv *invalidValue
	if errors.As(err, &iv) {
		return errInvalidValue(iv.value, n.Line, n.Column, iv.tag)
	}
	return err
}

func matchNull(s string, t *Token) bool {
	switch s {
	case "", "null", "Null", "NULL", "~":
		t.Kind = Null
		return true
	}
	return false
}

func matchBoolean(s string, t *Token) bool {
	switch s {
	case "true", "True", "TRUE":
		t.Kind, t.Bool = Boolean, true
		return true
	case "false", "False", "FALSE":
		t.Kind, t.Bool = Boolean, false
		return true
	}
	return false
}

func allDigits(s string, valid func(byte) bool) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !valid(s[i]) {
			return false
		}
	}
	return true
}

func isDecimal(c byte) bool { return c >= '0' && c <= '9' }
func isOctal(c byte) bool   { return c >= '0' && c <= '7' }
func isHex(c byte) bool {
	return isDecimal(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// parseDouble parses like .NET's Double.TryParse, which turns values out of
// range into infinity.
func parseDouble(s string) float64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return math.NaN()
	}
	return f
}

func matchInteger(s string, t *Token) (bool, error) {
	if s == "" {
		return false, nil
	}
	switch {
	case allDigits(s, isDecimal),
		(s[0] == '-' || s[0] == '+') && allDigits(s[1:], isDecimal):
		t.Kind, t.Num = Number, parseDouble(s)
		return true, nil
	case len(s) > 2 && s[0] == '0' && s[1] == 'x' && allDigits(s[2:], isHex):
		n, err := strconv.ParseInt(s[2:], 16, 32)
		if err != nil {
			return false, &invalidValue{value: s, tag: tagInt}
		}
		t.Kind, t.Num = Number, float64(n)
		return true, nil
	case len(s) > 2 && s[0] == '0' && s[1] == 'o' && allDigits(s[2:], isOctal):
		n, err := strconv.ParseInt(s[2:], 8, 32)
		if err != nil {
			return false, &invalidValue{value: s, tag: tagInt}
		}
		t.Kind, t.Num = Number, float64(n)
		return true, nil
	}
	return false, nil
}

func matchFloat(s string, t *Token) bool {
	switch s {
	case ".inf", ".Inf", ".INF", "+.inf", "+.Inf", "+.INF":
		t.Kind, t.Num = Number, math.Inf(1)
		return true
	case "-.inf", "-.Inf", "-.INF":
		t.Kind, t.Num = Number, math.Inf(-1)
		return true
	case ".nan", ".NaN", ".NAN":
		t.Kind, t.Num = Number, math.NaN()
		return true
	}

	// [-+]?(\.[0-9]+|[0-9]+(\.[0-9]*)?)([eE][-+]?[0-9]+)?
	i := 0
	if i < len(s) && (s[i] == '-' || s[i] == '+') {
		i++
	}
	start := i
	for i < len(s) && isDecimal(s[i]) {
		i++
	}
	hasInteger := i > start
	hasDot := i < len(s) && s[i] == '.'
	if hasDot {
		i++
	}
	start = i
	for i < len(s) && isDecimal(s[i]) {
		i++
	}
	hasDecimal := i > start
	if !(hasDot && hasDecimal) && !hasInteger {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '-' || s[i] == '+') {
			i++
		}
		start = i
		for i < len(s) && isDecimal(s[i]) {
			i++
		}
		if i == start {
			return false
		}
	}
	if i != len(s) {
		return false
	}
	t.Kind, t.Num = Number, parseDouble(s)
	return true
}

func (r *yamlReader) peek() event {
	return r.events[r.pos]
}

func (r *yamlReader) next() event {
	e := r.events[r.pos]
	if e.kind == evError {
		panic(abort{e.err})
	}
	r.pos++
	return e
}

func (r *yamlReader) allow(kind eventKind) (*Token, bool) {
	e := r.peek()
	if e.kind == evError {
		panic(abort{e.err})
	}
	if e.kind != kind {
		return nil, false
	}
	r.pos++
	return e.token, true
}
