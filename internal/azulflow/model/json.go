package model

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"gogs.io/gogs/internal/azulflow/template"
)

// The JSON form of a workflow template is the one GitHub's parsers produce,
// so that it can be compared with GitHub's conformance corpus and stored in a
// format GitHub's tools read. Values are written in the order they are added,
// as JavaScript's JSON.stringify does.

type jsonValue interface{}

// jsonObject is an object whose members keep their order.
type jsonObject struct {
	keys   []string
	values []jsonValue
}

func (o *jsonObject) set(key string, value jsonValue) {
	o.keys = append(o.keys, key)
	o.values = append(o.values, value)
}

// jsonNumber is a number written in JavaScript's format.
type jsonNumber float64

type jsonWriter struct {
	b strings.Builder
}

// writeJSON writes v with two spaces of indentation, as
// JSON.stringify(v, null, "  ") does.
func writeJSON(v jsonValue) string {
	var w jsonWriter
	w.value(v, "")
	return w.b.String()
}

func (w *jsonWriter) value(v jsonValue, indent string) {
	switch v := v.(type) {
	case nil:
		w.b.WriteString("null")
	case bool:
		w.b.WriteString(strconv.FormatBool(v))
	case jsonNumber:
		w.b.WriteString(formatJSNumber(float64(v)))
	case string:
		w.str(v)
	case []jsonValue:
		if len(v) == 0 {
			w.b.WriteString("[]")
			return
		}
		inner := indent + "  "
		w.b.WriteString("[\n")
		for i, item := range v {
			if i > 0 {
				w.b.WriteString(",\n")
			}
			w.b.WriteString(inner)
			w.value(item, inner)
		}
		w.b.WriteString("\n" + indent + "]")
	case *jsonObject:
		if len(v.keys) == 0 {
			w.b.WriteString("{}")
			return
		}
		inner := indent + "  "
		w.b.WriteString("{\n")
		for i, key := range v.keys {
			if i > 0 {
				w.b.WriteString(",\n")
			}
			w.b.WriteString(inner)
			w.str(key)
			w.b.WriteString(": ")
			w.value(v.values[i], inner)
		}
		w.b.WriteString("\n" + indent + "}")
	default:
		panic("unexpected JSON value")
	}
}

func (w *jsonWriter) str(s string) {
	w.b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			w.b.WriteString(`\"`)
		case '\\':
			w.b.WriteString(`\\`)
		case '\b':
			w.b.WriteString(`\b`)
		case '\f':
			w.b.WriteString(`\f`)
		case '\n':
			w.b.WriteString(`\n`)
		case '\r':
			w.b.WriteString(`\r`)
		case '\t':
			w.b.WriteString(`\t`)
		case utf8.RuneError:
			w.b.WriteString(`\ufffd`)
		default:
			if r < 0x20 {
				w.b.WriteString(`\u00`)
				w.b.WriteString(strconv.FormatInt(int64(r)>>4, 16))
				w.b.WriteString(strconv.FormatInt(int64(r)&0xf, 16))
				continue
			}
			w.b.WriteRune(r)
		}
	}
	w.b.WriteByte('"')
}

// formatJSNumber formats a finite number as JavaScript's Number.toString does.
func formatJSNumber(f float64) string {
	if f == 0 {
		return "0"
	}
	sign := ""
	if f < 0 {
		sign, f = "-", -f
	}
	// The shortest digits that round trip, and the decimal exponent n such
	// that f = 0.digits × 10^n.
	e := strconv.FormatFloat(f, 'e', -1, 64)
	mantissa, exp, _ := strings.Cut(e, "e")
	digits := strings.Replace(mantissa, ".", "", 1)
	x, _ := strconv.Atoi(exp)
	n, k := x+1, len(digits)
	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits
	}
	out := digits[:1]
	if k > 1 {
		out += "." + digits[1:]
	}
	if n-1 >= 0 {
		return sign + out + "e+" + strconv.Itoa(n-1)
	}
	return sign + out + "e-" + strconv.Itoa(1-n)
}

// numberJSON returns a number as GitHub writes it: infinity and NaN are
// strings, since JSON has no literal for them.
func numberJSON(f float64) jsonValue {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return template.FormatNumber(f)
	}
	return jsonNumber(f)
}

func hasSource(t *template.Token) bool {
	return t.File != 0 || t.Line != 0 || t.Col != 0
}

// tokenJSON returns a token as GitHub serializes it. Literals without a
// source position are bare values.
func tokenJSON(t *template.Token) jsonValue {
	if t == nil {
		return nil
	}
	source := hasSource(t)
	if !source {
		switch t.Kind {
		case template.Null:
			return nil
		case template.Boolean:
			return t.Bool
		case template.Number:
			return numberJSON(t.Num)
		case template.String:
			return t.Str
		}
	}
	o := &jsonObject{}
	o.set("type", jsonNumber(t.Kind))
	if t.File != 0 {
		o.set("file", jsonNumber(t.File))
	}
	if t.Line != 0 {
		o.set("line", jsonNumber(t.Line))
	}
	if t.Col != 0 {
		o.set("col", jsonNumber(t.Col))
	}
	switch t.Kind {
	case template.Boolean:
		o.set("bool", t.Bool)
	case template.Number:
		o.set("num", numberJSON(t.Num))
	case template.String:
		o.set("lit", t.Str)
	case template.BasicExpression:
		if t.Expr != "" {
			o.set("expr", t.Expr)
		}
	case template.InsertExpression:
		o.set("directive", template.InsertDirective)
	case template.Sequence:
		if len(t.Seq) > 0 {
			items := make([]jsonValue, len(t.Seq))
			for i, item := range t.Seq {
				items[i] = tokenJSON(item)
			}
			o.set("seq", items)
		}
	case template.Mapping:
		if len(t.Map) > 0 {
			items := make([]jsonValue, len(t.Map))
			for i, p := range t.Map {
				pair := &jsonObject{}
				pair.set("Key", tokenJSON(p.Key))
				pair.set("Value", tokenJSON(p.Value))
				items[i] = pair
			}
			o.set("map", items)
		}
	}
	return o
}

func stringsJSON(s []string) []jsonValue {
	out := make([]jsonValue, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// utf16Len returns the length of s in UTF-16 code units, which is how GitHub
// measures strings.
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}
