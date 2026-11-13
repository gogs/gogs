package dariomark

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Sanitizer describes the parts of the sanitizer policy that depend on the
// rendered content. Render emulates the rest of the policy of Gogs, which is
// the bluemonday UGC policy with classes for code languages, alerts and
// footnotes, and attributes for task list checkboxes.
type Sanitizer struct {
	// AllowURLScheme reports whether the policy allows the URL, which has a
	// scheme.
	AllowURLScheme func(u *url.URL) bool
	// Text matches the allowed values of alt and title attributes.
	Text *regexp.Regexp
	// CodeClass matches the allowed values of class attributes of code
	// elements.
	CodeClass *regexp.Regexp
}

// sanitizeSink post-processes and sanitizes a document without raw HTML,
// producing what postProcess followed by the sanitizer would. It gives up,
// setting failed, on HTML that it does not handle.
type sanitizeSink struct {
	r   *renderer
	out *bytes.Buffer
	san *Sanitizer
	// run is the text since the last tag, unescaped.
	run []byte
	// verbatim is the depth inside a, code and pre elements, whose content
	// postProcess leaves alone.
	verbatim int
	// dropped are the names of the dropped start tags whose end tags the
	// sanitizer skips, see bluemonday's closingTagToSkipStack.
	dropped []string
	failed  bool
	attrs   []html.Attribute
	buf     []byte
}

func (r *renderer) renderSanitized(doc *node) ([]byte, bool) {
	g := r.opts.Gogs
	if g.Sanitizer == nil {
		return nil, false
	}
	r.out.Grow(len(r.p.src) + len(r.p.src)/4)
	s := &sanitizeSink{r: r, out: &r.out, san: g.Sanitizer}
	r.s = s
	r.renderDocument(doc)
	s.flush()
	if s.failed {
		return nil, false
	}
	return r.out.Bytes(), true
}

func (s *sanitizeSink) text(b []byte) {
	s.run = append(s.run, s.r.replaceNUL(b)...)
}

func (s *sanitizeSink) entity(_, text string) {
	s.run = append(s.run, text...)
}

func (s *sanitizeSink) start(name string, attrs ...attr) {
	s.flush()
	s.attrs = s.attrs[:0]
	for _, a := range attrs {
		v := string(s.r.replaceNUL(a.val))
		if a.escaped {
			v = html.UnescapeString(v)
		}
		s.attrs = append(s.attrs, html.Attribute{Key: a.key, Val: convertNewlines(v)})
	}
	s.startTag(name, s.attrs)
}

func (s *sanitizeSink) end(name string) {
	s.flush()
	s.endTag(name)
}

// raw handles the HTML of shortened autolinks by tokenizing it.
func (s *sanitizeSink) raw(b []byte) {
	s.tokens(b, func(t *html.Token) {
		switch t.Type {
		case html.TextToken:
			s.run = append(s.run, t.Data...)
		case html.StartTagToken:
			s.flush()
			s.startTag(t.Data, t.Attr)
		case html.EndTagToken:
			s.flush()
			s.endTag(t.Data)
		default:
			s.failed = true
		}
	})
}

// startTag handles a start tag as postProcess does.
func (s *sanitizeSink) startTag(name string, attrs []html.Attribute) {
	if s.verbatim > 0 {
		if !isNoEndTag(name) {
			s.verbatim++
		}
		s.emitStart(name, attrs)
		return
	}
	switch name {
	case "img":
		s.wrapImage(attrs)
	case "a", "code", "pre":
		s.verbatim = 1
		s.emitStart(name, attrs)
	default:
		s.emitStart(name, attrs)
	}
}

func (s *sanitizeSink) endTag(name string) {
	if s.verbatim > 0 {
		s.verbatim--
	}
	s.emitEnd(name)
}

func (s *sanitizeSink) wrapImage(attrs []html.Attribute) {
	t := html.Token{Type: html.StartTagToken, Data: "img", Attr: attrs}
	var b bytes.Buffer
	if !s.r.imageLink(&b, t) {
		s.emitStart("img", attrs)
		return
	}
	s.fragment(b.Bytes())
}

// flush handles the text since the last tag as postProcess does: outside
// links, code and preformatted text, special references become links.
func (s *sanitizeSink) flush() {
	if len(s.run) == 0 {
		return
	}
	s.run = []byte(convertNewlines(string(s.run)))
	s.buf = appendHTMLEscaped(s.buf[:0], s.run)
	s.run = s.run[:0]
	if s.verbatim == 0 {
		if out := s.r.specialLinks(s.buf); !bytes.Equal(out, s.buf) {
			s.fragment(out)
			return
		}
	}
	s.out.Write(s.buf)
}

// fragment sanitizes HTML that postProcess wrote without tokenizing it.
func (s *sanitizeSink) fragment(b []byte) {
	s.tokens(b, func(t *html.Token) {
		switch t.Type {
		case html.TextToken:
			s.out.Write(appendHTMLEscaped(s.buf[:0], []byte(t.Data)))
		case html.StartTagToken:
			s.emitStart(t.Data, t.Attr)
		case html.EndTagToken:
			s.emitEnd(t.Data)
		default:
			s.failed = true
		}
	})
}

// tokens calls f with the tokens of b. It fails unless b is tokenized
// completely, so that the tokenizer is in the same state before and after.
func (s *sanitizeSink) tokens(b []byte, f func(t *html.Token)) {
	z := html.NewTokenizer(bytes.NewReader(b))
	n := 0
	for z.Next() != html.ErrorToken {
		n += len(z.Raw())
		t := z.Token()
		f(&t)
	}
	if n != len(b) {
		s.failed = true
	}
}

// emitStart writes a start tag as the sanitizer does.
func (s *sanitizeSink) emitStart(name string, attrs []html.Attribute) {
	if !sanitizedElements[name] {
		s.failed = true
		return
	}
	clean := attrs[:0:0]
	href := false
	for _, a := range attrs {
		ok := false
		switch a.Key {
		case "id":
			ok = idPattern.MatchString(a.Val)
		case "title":
			ok = s.san.Text.MatchString(a.Val)
		case "href":
			if name != "a" {
				s.failed = true
				return
			}
			a.Val, ok = s.validURL(a.Val)
			href = href || ok
		case "src":
			if name != "img" {
				s.failed = true
				return
			}
			a.Val, ok = s.validURL(a.Val)
		case "alt":
			if name != "img" {
				s.failed = true
				return
			}
			ok = s.san.Text.MatchString(a.Val)
		case "class":
			switch name {
			case "code":
				ok = s.san.CodeClass.MatchString(a.Val)
			case "a", "div", "li", "p", "ul", "ol":
				ok = allowedClasses[a.Val] == name || a.Val == string(containsTaskListClass) && name == "ol"
				if !ok {
					s.failed = true
					return
				}
			}
		case "role":
			ok = (name == "a" || name == "div") && allowedRoles[a.Val]
			if !ok {
				s.failed = true
				return
			}
		case "align":
			ok = (name == "td" || name == "th") && cellAlignPattern.MatchString(a.Val)
		case "start":
			ok = name == "ol" && listStartPattern.MatchString(a.Val)
		case "type":
			ok = name == "input" && a.Val == "checkbox"
		case "checked", "disabled":
			ok = name == "input"
		case "data-source-lines":
			ok = name == "li" && sourceLinesPattern.MatchString(a.Val)
		default:
			s.failed = true
			return
		}
		if ok {
			clean = append(clean, a)
		}
	}
	if href {
		clean = append(clean, html.Attribute{Key: "rel", Val: "nofollow"})
	}
	if len(clean) == 0 && (name == "a" || name == "img" || name == "input") {
		s.dropped = append(s.dropped, name)
		return
	}
	out := s.out
	out.WriteByte('<')
	out.WriteString(name)
	for _, a := range clean {
		out.WriteByte(' ')
		out.WriteString(a.Key)
		out.WriteString(`="`)
		s.buf = appendHTMLEscaped(s.buf[:0], []byte(a.Val))
		out.Write(s.buf)
		out.WriteByte('"')
	}
	out.WriteByte('>')
}

// emitEnd writes an end tag as the sanitizer does.
func (s *sanitizeSink) emitEnd(name string) {
	if n := len(s.dropped); n > 0 && s.dropped[n-1] == name {
		s.dropped = s.dropped[:n-1]
		return
	}
	if !sanitizedElements[name] {
		s.failed = true
		return
	}
	s.out.WriteString("</")
	s.out.WriteString(name)
	s.out.WriteByte('>')
}

// validURL validates and normalizes a URL as the sanitizer does.
func (s *sanitizeSink) validURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if strings.ContainsAny(raw, " \t\n") {
		if !strings.HasPrefix(raw, "data:") {
			return "", false
		}
		if m := dataURIBase64Prefix.FindString(raw); m != "" {
			raw = m + strings.ReplaceAll(strings.ReplaceAll(raw[len(m):], "\r", ""), "\n", "")
		}
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	if u.Scheme != "" && !s.san.AllowURLScheme(u) {
		return "", false
	}
	v := u.String()
	return v, v != ""
}

var (
	dataURIBase64Prefix = regexp.MustCompile(`^data:[^,]*;base64,`)
	idPattern           = regexp.MustCompile(`[a-zA-Z0-9\:\-_\.]+`)
	cellAlignPattern    = regexp.MustCompile(`(?i)^(center|justify|left|right|char)$`)
	sourceLinesPattern  = regexp.MustCompile(`^[0-9]+-[0-9]+$`)
	listStartPattern    = regexp.MustCompile(`^[0-9]+$`)
)

// sanitizedElements are the elements that Render produces and the sanitizer
// allows.
var sanitizedElements = map[string]bool{
	"a": true, "blockquote": true, "br": true, "code": true, "del": true, "div": true, "em": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "hr": true, "img": true,
	"input": true, "li": true, "ol": true, "p": true, "pre": true, "strong": true, "sub": true, "sup": true,
	"table": true, "tbody": true, "td": true, "th": true, "thead": true, "tr": true, "ul": true,
}

// allowedClasses are the classes that Render produces, by element.
var allowedClasses = func() map[string]string {
	m := map[string]string{
		"footnote-ref":         "a",
		"footnote-backref":     "a",
		"footnotes":            "div",
		"markdown-alert-title": "p",
		"contains-task-list":   "ul",
		"task-list-item":       "li",
	}
	for _, k := range alertKinds[1:] {
		m[k.class] = "div"
	}
	return m
}()

var allowedRoles = map[string]bool{"doc-noteref": true, "doc-backlink": true, "doc-endnotes": true}

// appendHTMLEscaped appends b to dst escaped as html.EscapeString does.
func appendHTMLEscaped(dst, b []byte) []byte {
	start := 0
	for i, c := range b {
		var esc string
		switch c {
		case '&':
			esc = "&amp;"
		case '\'':
			esc = "&#39;"
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '"':
			esc = "&#34;"
		case '\r':
			esc = "&#13;"
		default:
			continue
		}
		dst = append(dst, b[start:i]...)
		dst = append(dst, esc...)
		start = i + 1
	}
	return append(dst, b[start:]...)
}

// convertNewlines replaces "\r\n" and "\r" with "\n", as the tokenizer does.
func convertNewlines(s string) string {
	if strings.IndexByte(s, '\r') < 0 {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
