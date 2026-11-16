package expr

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/cockroachdb/errors"
)

// Values in expressions are nil, bool, float64, string, []any, and *Object.
// Normalize converts other Go values into these types.

// Object is an ordered map with case-insensitive keys, as objects are in
// GitHub expressions. Key order is kept because it decides the order of matrix
// combinations and the output of toJSON.
type Object struct {
	keys   []string
	values []any
	index  map[string]int // Lowercased key to position.
}

// NewObject returns an empty object.
func NewObject() *Object {
	return &Object{index: map[string]int{}}
}

// Set sets the value of key, keeping the position and spelling of an existing
// key that differs only in case.
func (o *Object) Set(key string, value any) {
	lower := strings.ToLower(key)
	if i, ok := o.index[lower]; ok {
		o.values[i] = value
		return
	}
	o.index[lower] = len(o.keys)
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

// Get returns the value of key, ignoring case.
func (o *Object) Get(key string) (any, bool) {
	if o == nil {
		return nil, false
	}
	i, ok := o.index[strings.ToLower(key)]
	if !ok {
		return nil, false
	}
	return o.values[i], true
}

// Keys returns the keys in insertion order.
func (o *Object) Keys() []string {
	if o == nil {
		return nil
	}
	return append([]string(nil), o.keys...)
}

// Len returns the number of keys.
func (o *Object) Len() int {
	if o == nil {
		return 0
	}
	return len(o.keys)
}

// Clone returns a shallow copy of the object.
func (o *Object) Clone() *Object {
	c := NewObject()
	for i, k := range o.Keys() {
		c.Set(k, o.values[i])
	}
	return c
}

// MarshalJSON encodes the object with its keys in insertion order.
func (o *Object) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range o.Keys() {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		value, err := json.Marshal(o.values[i])
		if err != nil {
			return nil, err
		}
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON decodes a JSON object, keeping its key order.
func (o *Object) UnmarshalJSON(data []byte) error {
	v, err := decodeJSON(data)
	if err != nil {
		return err
	}
	obj, ok := v.(*Object)
	if !ok {
		return errorf("expected a JSON object")
	}
	*o = *obj
	return nil
}

// Normalize converts a Go value into an expression value: maps become objects
// with sorted keys, slices become []any, and numbers become float64. Values of
// unsupported types become nil.
func Normalize(v any) any {
	switch v := v.(type) {
	case nil, bool, float64, string, *Object:
		return v
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = Normalize(v[i])
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		obj := NewObject()
		for _, k := range keys {
			obj.Set(k, Normalize(v[k]))
		}
		return obj
	case json.Number:
		f, _ := v.Float64()
		return f
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	case reflect.String:
		return rv.String()
	case reflect.Bool:
		return rv.Bool()
	case reflect.Slice, reflect.Array:
		out := make([]any, rv.Len())
		for i := range out {
			out[i] = Normalize(rv.Index(i).Interface())
		}
		return out
	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			return nil
		}
		m := make(map[string]any, rv.Len())
		iter := rv.MapRange()
		for iter.Next() {
			m[iter.Key().String()] = iter.Value().Interface()
		}
		return Normalize(m)
	case reflect.Pointer:
		if rv.IsNil() {
			return nil
		}
		return Normalize(rv.Elem().Interface())
	}
	return nil
}

// Truthy reports whether a value is truthy: false, null, 0, NaN, and the empty
// string are falsy, everything else is truthy.
func Truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0 && !math.IsNaN(v)
	case string:
		return v != ""
	}
	return true
}

// ToString converts a value to a string the way GitHub does when it
// interpolates a value into a string.
func ToString(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return formatNumber(v)
	case string:
		return v
	}
	return kindName(v)
}

// formatNumber formats a number as .NET's "G15" format does, which is what
// GitHub uses: at most 15 significant digits, in scientific notation when the
// exponent is below -4 or above 14.
func formatNumber(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case f == 0:
		return "0"
	}
	mantissa, exponent, _ := strings.Cut(strconv.FormatFloat(f, 'e', 14, 64), "e")
	exp, _ := strconv.Atoi(exponent)
	sign := ""
	if mantissa[0] == '-' {
		sign, mantissa = "-", mantissa[1:]
	}
	digits := strings.TrimRight(strings.Replace(mantissa, ".", "", 1), "0")
	switch {
	case exp < -4 || exp >= 15:
		out := digits[:1]
		if len(digits) > 1 {
			out += "." + digits[1:]
		}
		expSign := "+"
		if exp < 0 {
			expSign, exp = "-", -exp
		}
		return fmt.Sprintf("%s%sE%s%02d", sign, out, expSign, exp)
	case exp < 0:
		return sign + "0." + strings.Repeat("0", -exp-1) + digits
	case len(digits) <= exp+1:
		return sign + digits + strings.Repeat("0", exp+1-len(digits))
	}
	return sign + digits[:exp+1] + "." + digits[exp+1:]
}

// ToNumber converts a value to a number with GitHub's coercion rules: null is
// 0, booleans are 1 or 0, strings are parsed (empty is 0, invalid is NaN), and
// arrays and objects are NaN.
func ToNumber(v any) float64 {
	switch v := v.(type) {
	case nil:
		return 0
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return v
	case string:
		return parseNumber(v)
	}
	return math.NaN()
}

var decimalPattern = regexp.MustCompile(`^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$`)

// parseNumber parses a number the way GitHub does: decimal numbers with an
// optional sign and exponent, 32-bit "0x" hexadecimal and "0o" octal
// integers, Infinity, and NaN.
func parseNumber(s string) float64 {
	s = strings.TrimFunc(s, unicode.IsSpace)
	switch {
	case s == "":
		return 0
	case decimalPattern.MatchString(s):
		// Out of range values are infinite or zero, as in .NET.
		f, _ := strconv.ParseFloat(s, 64)
		return f
	case strings.EqualFold(s, "Infinity"), strings.EqualFold(s, "+Infinity"):
		return math.Inf(1)
	case strings.EqualFold(s, "-Infinity"):
		return math.Inf(-1)
	case strings.HasPrefix(s, "0x") && len(s) > 2:
		return parseInt32(s[2:], 16)
	case strings.HasPrefix(s, "0o") && len(s) > 2:
		return parseInt32(s[2:], 8)
	}
	return math.NaN()
}

// parseInt32 parses an unsigned integer of at most 32 bits, which .NET reads
// as a two's complement signed integer.
func parseInt32(s string, base int) float64 {
	n, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return math.NaN()
	}
	return float64(int32(uint32(n)))
}

type kind int

const (
	kindNull kind = iota
	kindBoolean
	kindNumber
	kindString
	kindArray
	kindObject
)

func kindOf(v any) kind {
	switch v.(type) {
	case nil:
		return kindNull
	case bool:
		return kindBoolean
	case float64:
		return kindNumber
	case string:
		return kindString
	case []any, *filtered:
		return kindArray
	}
	return kindObject
}

// kindName returns the name GitHub gives to the kind of a value.
func kindName(v any) string {
	return [...]string{"Null", "Boolean", "Number", "String", "Array", "Object"}[kindOf(v)]
}

func isPrimitive(v any) bool {
	return kindOf(v) < kindArray
}

func asArray(v any) ([]any, bool) {
	switch v := v.(type) {
	case []any:
		return v, true
	case *filtered:
		return v.items, true
	}
	return nil, false
}

// coerce converts two values of different kinds for a comparison: a number
// and a string compare as numbers, and booleans and null convert to numbers.
func coerce(a, b any) (any, any) {
	ka, kb := kindOf(a), kindOf(b)
	switch {
	case ka == kb:
	case ka == kindNumber && kb == kindString:
		return a, ToNumber(b)
	case ka == kindString && kb == kindNumber:
		return ToNumber(a), b
	case ka == kindBoolean || ka == kindNull:
		return coerce(ToNumber(a), b)
	case kb == kindBoolean || kb == kindNull:
		return coerce(a, ToNumber(b))
	}
	return a, b
}

// Equal compares two values with GitHub's loose equality: values of
// different kinds are coerced, strings are compared ignoring case, NaN equals
// nothing, and arrays and objects are only equal to themselves.
func Equal(a, b any) bool {
	a, b = coerce(a, b)
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && upper(x) == upper(y)
	case []any:
		// An array's identity is its backing array. Empty arrays without
		// capacity share one, so they are all equal.
		y, ok := b.([]any)
		return ok && len(x) == len(y) && cap(x) == cap(y) && reflect.ValueOf(x).Pointer() == reflect.ValueOf(y).Pointer()
	case *filtered:
		y, ok := b.(*filtered)
		return ok && x == y
	case *Object:
		y, ok := b.(*Object)
		return ok && x == y
	}
	return false
}

// greaterThan reports whether a > b with GitHub's rules. Values that cannot
// be ordered (NaN, arrays, objects, and values of different kinds after
// coercion) are neither greater nor less than each other.
func greaterThan(a, b any) bool {
	a, b = coerce(a, b)
	switch x := a.(type) {
	case bool:
		y, ok := b.(bool)
		return ok && x && !y
	case float64:
		y, ok := b.(float64)
		return ok && x > y
	case string:
		y, ok := b.(string)
		return ok && compareUTF16(upper(x), upper(y)) > 0
	}
	return false
}

// upper converts a string to upper case for comparisons that ignore case,
// rune by rune, as .NET's ordinal comparison does. The dotless i is kept, as
// GitHub does.
func upper(s string) string {
	return strings.Map(func(r rune) rune {
		if r == 'ı' {
			return r
		}
		return unicode.ToUpper(r)
	}, s)
}

// compareUTF16 compares two strings by their UTF-16 code units, as .NET does.
func compareUTF16(a, b string) int {
	for a != "" && b != "" {
		ra, sa := utf8.DecodeRuneInString(a)
		rb, sb := utf8.DecodeRuneInString(b)
		if ra != rb {
			// Runes above U+FFFF sort by their high surrogate, after U+D7FF
			// but before U+E000.
			ua, ub := utf16Unit(ra), utf16Unit(rb)
			if ua == ub {
				return cmp.Compare(ra, rb)
			}
			return cmp.Compare(ua, ub)
		}
		a, b = a[sa:], b[sb:]
	}
	return cmp.Compare(len(a), len(b))
}

func utf16Unit(r rune) rune {
	if r >= 0x10000 {
		r1, _ := utf16.EncodeRune(r)
		return r1
	}
	return r
}

// filtered is the result of an object filter (`.*`). Property access on it
// applies to every element.
type filtered struct {
	items []any
}

func unfilter(v any) any {
	if f, ok := v.(*filtered); ok {
		return f.items
	}
	return v
}

// ToJSON encodes a value as indented JSON, as the toJSON function does.
func ToJSON(v any) (string, error) {
	return toJSON(v, nil)
}

// toJSON encodes a value the way GitHub's toJSON does, which differs from
// encoding/json: numbers are formatted as in strings, and only control
// characters, quotes, backslashes, U+0085, U+2028, and U+2029 are escaped. It
// counts every piece it writes against the counter, if any.
func toJSON(v any, counter *memoryCounter) (string, error) {
	w := &jsonWriter{counter: counter}
	if err := w.value(v, "", 0); err != nil {
		return "", err
	}
	return w.b.String(), nil
}

type jsonWriter struct {
	b       strings.Builder
	counter *memoryCounter
}

func (w *jsonWriter) write(s string) error {
	if w.counter != nil {
		if err := w.counter.add(s); err != nil {
			return err
		}
	}
	w.b.WriteString(s)
	return nil
}

// value writes a value at the given depth, preceded by prefix, which
// separates it from what comes before.
func (w *jsonWriter) value(v any, prefix string, depth int) error {
	indent := func(depth int) string {
		return "\n" + strings.Repeat("  ", depth)
	}
	separator := func(i int) string {
		if i == 0 {
			return indent(depth + 1)
		}
		return "," + indent(depth+1)
	}
	if arr, ok := asArray(v); ok {
		if len(arr) == 0 {
			return w.write(prefix + "[]")
		}
		if err := w.write(prefix + "["); err != nil {
			return err
		}
		for i, item := range arr {
			if err := w.value(item, separator(i), depth+1); err != nil {
				return err
			}
		}
		return w.write(indent(depth) + "]")
	}
	switch v := v.(type) {
	case *Object:
		if v.Len() == 0 {
			return w.write(prefix + "{}")
		}
		if err := w.write(prefix + "{"); err != nil {
			return err
		}
		for i, k := range v.keys {
			if err := w.write(separator(i) + quoteJSON(k)); err != nil {
				return err
			}
			if err := w.value(v.values[i], ": ", depth+1); err != nil {
				return err
			}
		}
		return w.write(indent(depth) + "}")
	case nil:
		return w.write(prefix + "null")
	case string:
		return w.write(prefix + quoteJSON(v))
	case bool, float64:
		return w.write(prefix + ToString(v))
	}
	return w.write(prefix + "{}")
}

func quoteJSON(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\u0085', '\u2028', '\u2029':
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			if r < 0x20 {
				fmt.Fprintf(&b, `\u%04x`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// FromJSON decodes JSON into an expression value, keeping object key order.
// When an object has a key more than once, ignoring case, the last value wins
// at the position of the first key.
func FromJSON(s string) (any, error) {
	return decodeJSON([]byte(s))
}

func decodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeJSONValue(dec)
	if err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errorf("unexpected data after JSON value")
	}
	return v, nil
}

func decodeJSONValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch tok := tok.(type) {
	case json.Delim:
		switch tok {
		case '{':
			obj := NewObject()
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return nil, err
				}
				value, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				obj.Set(keyTok.(string), value)
			}
			_, err := dec.Token()
			return obj, err
		case '[':
			// Arrays have capacity so that each has its own identity.
			arr := make([]any, 0, 1)
			for dec.More() {
				value, err := decodeJSONValue(dec)
				if err != nil {
					return nil, err
				}
				arr = append(arr, value)
			}
			_, err := dec.Token()
			return arr, err
		}
		return nil, errorf("unexpected %v in JSON", tok)
	case json.Number:
		// Out of range numbers are infinite, as in .NET.
		f, err := strconv.ParseFloat(string(tok), 64)
		if err != nil && !errors.Is(err, strconv.ErrRange) {
			return nil, err
		}
		return f, nil
	}
	return tok, nil
}
