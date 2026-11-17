package dariomark

import (
	"bytes"
	"strings"

	"gopkg.in/yaml.v3"
)

// frontMatterKind is the kind of a frontMatterValue.
type frontMatterKind uint8

const (
	// frontMatterNone is YAML that is not front matter, i.e., it is empty or
	// is not a mapping. The document renders as if it had none.
	frontMatterNone frontMatterKind = iota
	// frontMatterInvalid is YAML that is not valid, has aliases, keys other
	// than scalars or nests too deeply. The front matter renders as a YAML
	// code block.
	frontMatterInvalid
	frontMatterScalar
	frontMatterMapping
	frontMatterSequence
)

// frontMatterValue is a value of YAML front matter. The front matter itself is
// a mapping, frontMatterNone or frontMatterInvalid.
type frontMatterValue struct {
	kind frontMatterKind
	// text is the text of a scalar, which is empty for null.
	text string
	// keys are the keys of a mapping.
	keys []string
	// values are the values of a mapping, in the order of keys, or the items
	// of a sequence.
	values []frontMatterValue
}

// maxFrontMatterDepth is how deeply collections of front matter may nest.
const maxFrontMatterDepth = 16

// parseFrontMatter returns the value of the YAML of front matter. YAML that is
// empty or is not a mapping is not front matter, and YAML with aliases is
// invalid, which rules out aliases that multiply nodes, e.g., "billion
// laughs", or contain themselves.
func parseFrontMatter(src []byte) frontMatterValue {
	var doc yaml.Node
	if err := yaml.Unmarshal(src, &doc); err != nil {
		return frontMatterValue{kind: frontMatterInvalid}
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode || len(doc.Content[0].Content) == 0 {
		return frontMatterValue{}
	}
	v, ok := convertFrontMatter(doc.Content[0], 0)
	if !ok {
		return frontMatterValue{kind: frontMatterInvalid}
	}
	return v
}

// convertFrontMatter converts the YAML node n at depth, and reports false when
// n has aliases, keys other than scalars, or nests too deeply.
func convertFrontMatter(n *yaml.Node, depth int) (frontMatterValue, bool) {
	if depth > maxFrontMatterDepth {
		return frontMatterValue{}, false
	}
	switch n.Kind {
	case yaml.ScalarNode:
		v := frontMatterValue{kind: frontMatterScalar}
		if n.ShortTag() != "!!null" {
			v.text = n.Value
		}
		return v, true
	case yaml.MappingNode:
		v := frontMatterValue{
			kind:   frontMatterMapping,
			keys:   make([]string, 0, len(n.Content)/2),
			values: make([]frontMatterValue, 0, len(n.Content)/2),
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i]
			if key.Kind != yaml.ScalarNode {
				return v, false
			}
			value, ok := convertFrontMatter(n.Content[i+1], depth+1)
			if !ok {
				return v, false
			}
			v.keys = append(v.keys, key.Value)
			v.values = append(v.values, value)
		}
		return v, true
	case yaml.SequenceNode:
		v := frontMatterValue{
			kind:   frontMatterSequence,
			values: make([]frontMatterValue, 0, len(n.Content)),
		}
		for _, item := range n.Content {
			value, ok := convertFrontMatter(item, depth+1)
			if !ok {
				return v, false
			}
			v.values = append(v.values, value)
		}
		return v, true
	}
	return frontMatterValue{}, false
}

// maxFrontMatterSize is the offset in the source by which front matter must
// end, see Options.FrontMatter. It bounds the search for the closing line and
// the YAML to parse.
var maxFrontMatterSize = 64 << 10

// frontMatter returns the YAML of the front matter at the start of src, the
// size of the front matter with its delimiter lines, and its number of lines.
// The size is 0 when there is no front matter. The delimiter lines are "---"
// with optional trailing spaces and tabs. src has normalized line endings.
func frontMatter(src []byte) (yaml []byte, size, lines int) {
	start := frontMatterDelimiter(src)
	if start == 0 {
		return nil, 0, 0
	}
	lines = 1
	for pos := start; pos < len(src) && pos < maxFrontMatterSize; {
		lines++
		if end := frontMatterDelimiter(src[pos:]); end > 0 && pos+end <= maxFrontMatterSize {
			return src[start:pos], pos + end, lines
		}
		i := bytes.IndexByte(src[pos:], '\n')
		if i < 0 {
			break
		}
		pos += i + 1
	}
	return nil, 0, 0
}

// frontMatterDelimiter returns the length of the line at the start of b,
// including its line ending, if it is a delimiter line of front matter, and
// otherwise 0.
func frontMatterDelimiter(b []byte) int {
	if !bytes.HasPrefix(b, []byte("---")) {
		return 0
	}
	i := 3
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	switch {
	case i == len(b):
		return i
	case b[i] == '\n':
		return i + 1
	}
	return 0
}

// frontMatter renders the front matter n, which starts the document: a mapping
// as a table with a row for each key and its value, and invalid YAML as a code
// block of n. The HTML goes to the output directly,
// escaped as the sanitizer escapes text, so that post-processing and
// sanitizing skip it, see Result.FrontMatterSize.
func (r *renderer) frontMatter(n *node) {
	out := r.plain[:0]
	if v := &r.p.frontMatter; v.kind == frontMatterMapping {
		out = append(out, "<table>\n<tbody>\n"...)
		for i, key := range v.keys {
			out = append(out, "<tr>\n<th>"...)
			out = appendFrontMatterText(out, key)
			out = append(out, "</th>\n<td>"...)
			out = r.appendFrontMatterValue(out, &v.values[i])
			out = append(out, "</td>\n</tr>\n"...)
		}
		out = append(out, "</tbody>\n</table>\n"...)
	} else {
		out = append(out, `<pre><code class="language-yaml">`...)
		out = appendFrontMatterText(out, string(n.text))
		out = append(out, "</code></pre>\n"...)
	}
	r.out.Write(out)
	r.frontMatterSize = r.out.Len()
	r.plain = out[:0]
}

// appendFrontMatterValue appends v, a value nested in front matter: a scalar
// as text, a mapping as a table with the keys in its head and the values in
// its body, and a sequence as a table of one row. The cells of nested tables
// wrap their content in a div.
func (r *renderer) appendFrontMatterValue(out []byte, v *frontMatterValue) []byte {
	switch v.kind {
	case frontMatterMapping:
		out = append(out, "<table><thead><tr>"...)
		for _, key := range v.keys {
			out = append(out, "<th>"...)
			out = appendFrontMatterText(out, key)
			out = append(out, "</th>"...)
		}
		out = append(out, "</tr></thead>"...)
		return r.appendFrontMatterRow(out, v.values)
	case frontMatterSequence:
		out = append(out, "<table>"...)
		return r.appendFrontMatterRow(out, v.values)
	}
	return appendFrontMatterText(out, v.text)
}

func (r *renderer) appendFrontMatterRow(out []byte, values []frontMatterValue) []byte {
	out = append(out, "<tbody><tr>"...)
	for i := range values {
		out = append(out, "<td><div>"...)
		out = r.appendFrontMatterValue(out, &values[i])
		out = append(out, "</div></td>"...)
	}
	return append(out, "</tr></tbody></table>"...)
}

// appendFrontMatterText appends text as the sanitizer writes it.
func appendFrontMatterText(out []byte, text string) []byte {
	if strings.ContainsAny(text, "\r\x00") {
		text = strings.ReplaceAll(convertNewlines(text), "\x00", "\uFFFD")
	}
	return appendHTMLEscaped(out, text)
}
