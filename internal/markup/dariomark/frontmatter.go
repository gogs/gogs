package dariomark

import (
	"bytes"
	"strings"
)

// FrontMatterKind is the kind of a FrontMatterValue.
type FrontMatterKind uint8

const (
	// FrontMatterNone is YAML that is not front matter, e.g., because it is
	// empty or is not a mapping. The document renders as if it had none.
	FrontMatterNone FrontMatterKind = iota
	// FrontMatterInvalid is YAML that is not valid. The front matter renders
	// as a YAML code block.
	FrontMatterInvalid
	FrontMatterScalar
	FrontMatterMapping
	FrontMatterSequence
)

// FrontMatterValue is a value of YAML front matter, see Options.FrontMatter.
// The front matter is a mapping, FrontMatterNone or FrontMatterInvalid.
type FrontMatterValue struct {
	Kind FrontMatterKind
	// Text is the text of a scalar, which is empty for null.
	Text string
	// Keys are the keys of a mapping, as text.
	Keys []string
	// Values are the values of a mapping, in the order of Keys, or the items
	// of a sequence.
	Values []FrontMatterValue
}

// maxFrontMatterSize is the offset in the source by which front matter must
// end, see Options.FrontMatter. It bounds the search for the closing line and
// the YAML that Options.FrontMatter parses.
var maxFrontMatterSize = 64 << 10

// frontMatter returns the YAML of the front matter at the start of src, the
// size of the front matter with its delimiter lines, and its number of lines.
// The size is 0 when there is no front matter. Like GitHub, the delimiter
// lines are "---" with optional trailing spaces and tabs. src has normalized
// line endings.
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

// frontMatter renders the front matter n, which starts the document, as GitHub
// does: a mapping as a table with a row for each key and its value, and
// invalid YAML as a code block of n. The HTML goes to the output directly,
// escaped as the sanitizer escapes text, so that post-processing and
// sanitizing skip it, see Result.FrontMatterSize.
func (r *renderer) frontMatter(n *node) {
	out := r.plain[:0]
	if v := &r.p.frontMatter; v.Kind == FrontMatterMapping {
		out = append(out, "<table>\n<tbody>\n"...)
		for i, key := range v.Keys {
			out = append(out, "<tr>\n<th>"...)
			out = appendFrontMatterText(out, key)
			out = append(out, "</th>\n<td>"...)
			out = r.appendFrontMatterValue(out, &v.Values[i])
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
// its body, and a sequence as a table of one row. Like GitHub, the cells of
// nested tables wrap their content in a div.
func (r *renderer) appendFrontMatterValue(out []byte, v *FrontMatterValue) []byte {
	switch v.Kind {
	case FrontMatterMapping:
		out = append(out, "<table><thead><tr>"...)
		for _, key := range v.Keys {
			out = append(out, "<th>"...)
			out = appendFrontMatterText(out, key)
			out = append(out, "</th>"...)
		}
		out = append(out, "</tr></thead>"...)
		return r.appendFrontMatterRow(out, v.Values)
	case FrontMatterSequence:
		out = append(out, "<table>"...)
		return r.appendFrontMatterRow(out, v.Values)
	}
	return appendFrontMatterText(out, v.Text)
}

func (r *renderer) appendFrontMatterRow(out []byte, values []FrontMatterValue) []byte {
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
