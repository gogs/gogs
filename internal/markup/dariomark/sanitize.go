package dariomark

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// Sanitizer describes the parts of the sanitizer policy that depend on the
// rendered content, see Policy.Sanitizer. Render emulates the rest of the
// policy, which is the Policy of NewPolicy for Markdown: the allowlist for
// user content with classes for code languages, alerts, footnotes and task lists, and
// attributes for task list items and checkboxes, footnote roles and ordered
// list start numbers.
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
	out *outBuf
	san *Sanitizer
	// run is the text since the last tag, unescaped.
	run []byte
	// verbatim is the depth inside a, code and pre elements, whose content
	// postProcess leaves alone.
	verbatim int
	// code is the depth inside code and pre elements, whose text keeps its
	// emoji shortcodes.
	code  int
	emoji emojiText
	// dropped are the names of the dropped start tags whose end tags the
	// sanitizer skips, see sanitizeState.stack.
	dropped []string
	failed  bool
	attrs   []html.Attribute
	// clean is a buffer for the attributes that emitStart keeps.
	clean []html.Attribute
	buf   []byte
	// escaped is the escaped run, see flush.
	escaped []byte
}

func (r *renderer) renderSanitized(render func()) ([]byte, bool) {
	if r.opts.Sanitizer == nil {
		return nil, false
	}
	r.out.Grow(len(r.p.src) + len(r.p.src)/4 + min(len(r.p.src), 2048) + 32)
	// The sink keeps its buffers from the previous render.
	s := &r.san
	*s = sanitizeSink{
		r: r, out: &r.out, san: r.opts.Sanitizer, emoji: emojiText{names: r.opts.Emoji},
		run: s.run[:0], dropped: s.dropped[:0], attrs: s.attrs[:0], clean: s.clean[:0], buf: s.buf[:0], escaped: s.escaped[:0],
	}
	r.s = sink{san: s}
	r.sanitizing = true
	render()
	s.flush()
	if s.failed || r.stop {
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
	if name == "code" || name == "pre" {
		s.code++
	}
	if s.verbatim > 0 {
		if !isNoEndTag(name) {
			s.verbatim++
		}
		if name == "img" && s.code == 0 {
			attrs = s.r.linkedImageAttrs(attrs)
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
	if name == "code" || name == "pre" {
		s.code--
	}
	if s.verbatim > 0 {
		s.verbatim--
	}
	s.emitEnd(name)
}

func (s *sanitizeSink) wrapImage(attrs []html.Attribute) {
	t := html.Token{Type: html.StartTagToken, Data: "img", Attr: attrs}
	var b outBuf
	if !s.r.imageLink(&b, t) {
		s.emitStart("img", attrs)
		return
	}
	s.fragment(b.Bytes())
}

// flush handles the text since the last tag as postProcess does: outside
// links, code and preformatted text, special references become links, and
// outside code and preformatted text, emoji shortcodes become emoji.
func (s *sanitizeSink) flush() {
	if len(s.run) == 0 {
		return
	}
	if bytes.IndexByte(s.run, '\r') >= 0 {
		s.run = []byte(convertNewlines(string(s.run)))
	}
	s.escaped = appendHTMLEscaped(s.escaped[:0], s.run)
	s.run = s.run[:0]
	s.emoji.startNode()
	if s.code > 0 {
		s.out.Write(s.escaped)
		return
	}
	if s.verbatim > 0 {
		s.emoji.write(s.out, s.escaped)
		return
	}
	text := s.escaped
	if len(text) <= specialLinksPiece {
		s.specialLinks(text)
		return
	}
	// Long text is passed to Options.SpecialLinks in pieces, so that the
	// links of each piece take little memory. See Options.SpecialLinks for
	// where pieces end.
	start, http := 0, false
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\n':
			http = false
			if i+1-start >= specialLinksPiece && (i+1 == len(text) || !startsReference(text[i+1])) {
				s.specialLinks(text[start : i+1])
				start = i + 1
			}
		case ' ':
			if !http && i+1-start >= specialLinksPiece {
				s.specialLinks(text[start : i+1])
				start = i + 1
			}
		case 'h':
			http = http || bytes.HasPrefix(text[i:], []byte("http"))
		}
	}
	s.specialLinks(text[start:])
}

// specialLinksPiece is the length from which text is passed to
// Options.SpecialLinks in pieces.
var specialLinksPiece = 32 << 10

// startsReference reports whether c may start a special reference. A piece of
// text passed to Options.SpecialLinks does not start with such a character.
func startsReference(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.' || c == '#'
}

// specialLinks writes the escaped text with special references rendered as
// links, as the sanitizer would write the HTML of AppendSpecialLinks, and
// emoji shortcodes replaced. The links split the text into text nodes.
func (s *sanitizeSink) specialLinks(text []byte) {
	f := s.r.opts.SpecialLinks
	if f == nil {
		s.emoji.write(s.out, text)
		return
	}
	last := 0
	for _, l := range f(text) {
		s.emoji.write(s.out, text[last:l.Start])
		s.emoji.startNode()
		last = l.End
		s.attrs = append(s.attrs[:0], html.Attribute{Key: "href", Val: unescapeHTML(l.Href)})
		s.emitStart("a", s.attrs)
		if l.Code {
			s.emitStart("code", nil)
		}
		s.out.Write(l.Text)
		if l.Code {
			s.emitEnd("code")
		}
		s.emitEnd("a")
	}
	s.emoji.write(s.out, text[last:])
}

// unescapeHTML returns the text of the escaped HTML b.
func unescapeHTML(b []byte) string {
	if bytes.IndexByte(b, '&') < 0 {
		return string(b)
	}
	return html.UnescapeString(string(b))
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

// emitStart writes a start tag as the sanitizer does. Its attribute checks
// must be kept in sync with NewPolicy.
func (s *sanitizeSink) emitStart(name string, attrs []html.Attribute) {
	if !sanitizedElement(name) {
		s.failed = true
		return
	}
	clean := s.clean[:0]
	href := false
	for _, a := range attrs {
		ok := false
		switch a.Key {
		case "id":
			ok = hasID([]byte(a.Val))
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
			ok = (name == "td" || name == "th") && isCellAlign([]byte(a.Val))
		case "start":
			ok = name == "ol" && isDigits([]byte(a.Val))
		case "type":
			ok = name == "input" && a.Val == "checkbox"
		case "checked", "disabled":
			ok = name == "input"
		case "data-source-lines":
			ok = name == "li" && isSourceLines([]byte(a.Val))
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
	s.clean = clean[:0]
	if len(clean) == 0 && (name == "a" || name == "img" || name == "input") {
		s.dropped = append(s.dropped, name)
		return
	}
	out := s.out
	out.writeByte('<')
	out.WriteString(name)
	for _, a := range clean {
		out.writeByte(' ')
		out.WriteString(a.Key)
		out.WriteString(`="`)
		s.buf = appendHTMLEscaped(s.buf[:0], a.Val)
		out.Write(s.buf)
		out.writeByte('"')
	}
	out.writeByte('>')
}

// emitEnd writes an end tag as the sanitizer does.
func (s *sanitizeSink) emitEnd(name string) {
	if n := len(s.dropped); n > 0 && s.dropped[n-1] == name {
		s.dropped = s.dropped[:n-1]
		return
	}
	if !sanitizedElement(name) {
		s.failed = true
		return
	}
	s.out.WriteString("</")
	s.out.WriteString(name)
	s.out.writeByte('>')
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

// The tables below, with the attribute checks in emitStart, copy the
// sanitizer policy for the HTML that Render produces. They must be kept in
// sync with NewPolicy, whose checks emitStart shares.

// sanitizedElement reports whether name is an element that Render produces
// and the sanitizer allows.
func sanitizedElement(name string) bool {
	switch name {
	case "a", "blockquote", "br", "code", "del", "div", "em",
		"h1", "h2", "h3", "h4", "h5", "h6", "hr", "img",
		"input", "li", "ol", "p", "pre", "strong", "sub", "sup",
		"table", "tbody", "td", "th", "thead", "tr", "ul":
		return true
	}
	return false
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
func appendHTMLEscaped[T string | []byte](dst []byte, b T) []byte {
	start := 0
	for i := 0; i < len(b); i++ {
		if !needsHTMLEscape[b[i]] {
			continue
		}
		var esc string
		switch b[i] {
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

var needsHTMLEscape = withSpecial([256]bool{}, "&'<>\"\r")

// convertNewlines replaces "\r\n" and "\r" with "\n", as the tokenizer does.
func convertNewlines(s string) string {
	if strings.IndexByte(s, '\r') < 0 {
		return s
	}
	return strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
}
