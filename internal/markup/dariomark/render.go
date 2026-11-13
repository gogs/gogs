package dariomark

import (
	"bytes"
	"strconv"
	"unicode"
	"unicode/utf8"
)

type attr struct {
	key string
	val []byte
	// escaped reports whether val is HTML already.
	escaped bool
}

// sink receives the rendered document as HTML tokens.
type sink interface {
	// text writes text, which is not escaped yet.
	text(b []byte)
	// entity writes a character reference, given in its HTML form and as
	// text.
	entity(html, text string)
	// start writes a start tag with attributes whose values are not escaped
	// yet.
	start(name string, attrs ...attr)
	end(name string)
	// raw writes raw HTML.
	raw(b []byte)
}

type renderer struct {
	p    *parser
	opts *Options
	out  bytes.Buffer
	s    sink
	html htmlSink

	plain  []byte
	numBuf [24]byte

	headings []Heading
	// slugs counts the uses of heading slugs, see headingID.
	slugs map[string]int
}

func (r *renderer) init(p *parser, opts *Options) {
	r.p = p
	r.opts = opts
	r.out.Reset()
	r.html = htmlSink{out: &r.out, xhtml: opts.XHTML}
	r.headings = r.headings[:0]
	clear(r.slugs)
}

func (r *renderer) renderHTML(doc *node) {
	r.out.Grow(len(r.p.src) + len(r.p.src)/4)
	r.s = &r.html
	r.renderDocument(doc)
}

func (r *renderer) renderDocument(doc *node) {
	r.blocks(doc)
	if len(r.p.footnotes.ordered) > 0 {
		r.renderFootnotes()
	}
}

func (r *renderer) nl() {
	r.s.text(newline)
}

var newline = []byte("\n")

func (r *renderer) blocks(parent *node) {
	for n := parent.first; n != nil; n = n.next {
		r.block(n)
	}
}

func (r *renderer) block(n *node) {
	s := r.s
	switch n.kind {
	case kindParagraph:
		if parent := n.parent; parent != nil && parent.kind == kindListItem && parent.parent.tight {
			r.inlines(n)
			if n.next != nil {
				r.nl()
			}
			return
		}
		s.start("p")
		r.inlines(n)
		if n.parent.kind == kindFootnoteDef && n.next == nil {
			r.backlinks(n.parent)
		}
		s.end("p")
		r.nl()
	case kindHeading:
		tag := headingTags[n.level]
		r.headingStart(n, tag)
		r.inlines(n)
		s.end(tag)
		r.nl()
	case kindThematicBreak:
		s.start("hr")
		r.nl()
	case kindCodeBlock:
		r.codeBlock(n)
	case kindHTMLBlock:
		for i := n.segStart; i < n.segEnd; i++ {
			sg := r.p.segs[i]
			if sg.pad > 0 {
				s.raw(spaces[:sg.pad])
			}
			s.raw(r.replaceNUL(r.p.src[sg.start:sg.eol]))
		}
	case kindBlockquote:
		if n.alert > 0 {
			r.alert(n)
			return
		}
		s.start("blockquote")
		r.nl()
		r.blocks(n)
		s.end("blockquote")
		r.nl()
	case kindList:
		tag := "ul"
		if n.ordered {
			tag = "ol"
			if n.listStart != 1 {
				s.start(tag, attr{"start", strconv.AppendInt(r.numBuf[:0], int64(n.listStart), 10), false})
			} else {
				s.start(tag)
			}
		} else {
			s.start(tag)
		}
		r.nl()
		r.blocks(n)
		s.end(tag)
		r.nl()
	case kindListItem:
		s.start("li")
		f := n.first
		if f != nil && f.kind == kindFootnoteHolder {
			f = f.next
		}
		if f != nil && !(f.kind == kindParagraph && n.parent.tight) {
			r.nl()
		}
		r.blocks(n)
		s.end("li")
		r.nl()
	case kindTable:
		r.table(n)
	}
}

var headingTags = [...]string{"h1", "h1", "h2", "h3", "h4", "h5", "h6"}

// headingStart writes the start tag of the heading n, with an id when
// Options.HeadingIDs is set.
func (r *renderer) headingStart(n *node, tag string) {
	if !r.opts.HeadingIDs {
		r.s.start(tag)
		return
	}
	text := r.replaceNUL(r.appendTextContent(r.plain[:0], n))
	r.plain = text[:0]
	h := Heading{Level: n.level, Text: string(text)}
	if slug := r.headingID(text); slug != "" {
		h.ID = r.opts.HeadingIDPrefix + slug
		r.s.start(tag, attr{"id", []byte(h.ID), false})
	} else {
		r.s.start(tag)
	}
	r.headings = append(r.headings, h)
}

// appendTextContent appends the text of the inline content of n, as a
// browser shows it, without images and footnote references.
func (r *renderer) appendTextContent(b []byte, n *node) []byte {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindText, kindCodeSpan, kindAutoLink:
			b = append(b, c.text...)
		case kindSoftBreak, kindHardBreak:
			b = append(b, ' ')
		case kindTypo:
			_, text := r.opts.Typographer.entity(typo(c.level))
			b = append(b, text...)
		case kindFraction:
			if c.level == 0 {
				_, text := vulgarFraction(c)
				b = append(b, text...)
			} else {
				b = append(b, c.text...)
			}
		case kindRawHTML, kindImage, kindFootnoteRef, kindTaskCheckbox:
		default:
			b = r.appendTextContent(b, c)
		}
	}
	return b
}

// headingID returns the slug of a heading with the given text, as GitHub
// makes it: letters are lowercased, spaces become "-", and characters other
// than letters, digits, "-" and "_" are dropped. A slug that an earlier
// heading has is followed by "-1", "-2" and so on.
func (r *renderer) headingID(text []byte) string {
	b := r.numBuf[:0]
	for len(text) > 0 {
		c, size := utf8.DecodeRune(text)
		text = text[size:]
		switch {
		case c == ' ':
			b = append(b, '-')
		case c == '-' || c == '_' || unicode.IsLetter(c) || unicode.IsDigit(c) || unicode.IsMark(c):
			b = utf8.AppendRune(b, unicode.ToLower(c))
		}
	}
	if len(b) == 0 {
		return ""
	}
	if r.slugs == nil {
		r.slugs = make(map[string]int)
	}
	base := string(b)
	slug := base
	for {
		if _, used := r.slugs[slug]; !used {
			break
		}
		// The count of base only grows, so that each heading takes time
		// linear in the number of headings with the same slug.
		r.slugs[base]++
		slug = base + "-" + strconv.Itoa(r.slugs[base])
	}
	r.slugs[slug] = 0
	return slug
}

var spaces = []byte("                ")

func (r *renderer) codeBlock(n *node) {
	s := r.s
	s.start("pre")
	if lang := r.codeLanguage(n); len(lang) > 0 {
		r.plain = append(append(r.plain[:0], "language-"...), lang...)
		s.start("code", attr{"class", r.plain, false})
	} else {
		s.start("code")
	}
	for i := n.segStart; i < n.segEnd; i++ {
		sg := r.p.segs[i]
		if sg.start == sg.eol && sg.pad == 0 && n.fenced {
			// An empty last line without a line ending.
			continue
		}
		if sg.pad > 0 {
			s.text(spaces[:sg.pad])
		}
		line := r.p.src[sg.start:sg.eol]
		s.text(line)
		if sg.eol == sg.end {
			r.nl()
		}
	}
	s.end("code")
	s.end("pre")
	r.nl()
}

// codeLanguage returns the first word of the info string of a code block.
func (r *renderer) codeLanguage(n *node) []byte {
	info := n.text
	if len(info) == 0 {
		return nil
	}
	if i := bytes.IndexByte(info, ' '); i >= 0 {
		info = info[:i]
	}
	return r.p.unescapeString(info)
}

func (r *renderer) table(t *node) {
	s := r.s
	s.start("table")
	r.nl()
	for row := t.first; row != nil; row = row.next {
		if row.header {
			s.start("thead")
			r.nl()
		} else if row.prev != nil && row.prev.header {
			s.start("tbody")
			r.nl()
		}
		s.start("tr")
		r.nl()
		tag := "td"
		if row.header {
			tag = "th"
		}
		cells := 0
		for c := row.first; c != nil; c = c.next {
			cells++
			switch c.align {
			case alignLeft:
				s.start(tag, attr{"align", alignLeftValue, false})
			case alignCenter:
				s.start(tag, attr{"align", alignCenterValue, false})
			case alignRight:
				s.start(tag, attr{"align", alignRightValue, false})
			default:
				s.start(tag)
			}
			r.inlines(c)
			s.end(tag)
			r.nl()
		}
		// Short rows are filled with empty cells without alignment, as long
		// as the document has budget for them. Otherwise a wide header and
		// many short rows would make the output quadratic in the input.
		for ; cells < len(t.aligns) && r.p.padBudget > 0; cells++ {
			r.p.padBudget--
			s.start(tag)
			s.end(tag)
			r.nl()
		}
		s.end("tr")
		r.nl()
		if row.header {
			s.end("thead")
			r.nl()
		} else if row.next == nil {
			s.end("tbody")
			r.nl()
		}
	}
	s.end("table")
	r.nl()
}

var (
	alignLeftValue   = []byte("left")
	alignCenterValue = []byte("center")
	alignRightValue  = []byte("right")
)

func (r *renderer) inlines(parent *node) {
	for n := parent.first; n != nil; n = n.next {
		r.inline(n)
	}
}

func (r *renderer) inline(n *node) {
	s := r.s
	switch n.kind {
	case kindText:
		s.text(n.text)
	case kindSoftBreak:
		if r.opts.HardWraps {
			s.start("br")
		}
		r.nl()
	case kindHardBreak:
		s.start("br")
		r.nl()
	case kindCodeSpan:
		s.start("code")
		s.text(n.text)
		s.end("code")
	case kindEmphasis:
		s.start("em")
		r.inlines(n)
		s.end("em")
	case kindStrong:
		s.start("strong")
		r.inlines(n)
		s.end("strong")
	case kindStrikethrough:
		s.start("del")
		r.inlines(n)
		s.end("del")
	case kindLink:
		href := r.urlEscape(n.dest)
		if n.hasTitle {
			s.start("a", attr{"href", href, false}, attr{"title", n.title, false})
		} else {
			s.start("a", attr{"href", href, false})
		}
		r.inlines(n)
		s.end("a")
	case kindImage:
		href := r.urlEscape(n.dest)
		alt := r.plainText(n)
		if n.hasTitle {
			s.start("img", attr{"src", href, false}, attr{"alt", alt, true}, attr{"title", n.title, false})
		} else {
			s.start("img", attr{"src", href, false}, attr{"alt", alt, true})
		}
	case kindAutoLink:
		r.autoLink(n)
	case kindRawHTML:
		s.raw(n.text)
	case kindFootnoteRef:
		r.footnoteRef(n)
	case kindTypo:
		s.entity(r.opts.Typographer.entity(typo(n.level)))
	case kindFraction:
		if n.level == 0 {
			s.entity(vulgarFraction(n))
			break
		}
		s.start("sup")
		s.text(n.text[:n.level])
		s.end("sup")
		s.entity("&frasl;", "\u2044")
		s.start("sub")
		s.text(n.text[len(n.text)-countDigitsBack(n.text):])
		s.end("sub")
	case kindTaskCheckbox:
		if n.checked {
			s.start("input", attr{"checked", nil, false}, attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
		} else {
			s.start("input", attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
		}
		s.text(spaces[:1])
	}
}

var checkboxValue = []byte("checkbox")

func (r *renderer) autoLink(n *node) {
	s := r.s
	if n.plain {
		s.text(r.p.unescapeString(n.text))
		return
	}
	if n.autoLink != autoLinkEmail && r.opts.ShortenAutoLink != nil {
		if html, ok := r.opts.ShortenAutoLink(n.dest); ok {
			s.raw(html)
			return
		}
	}
	dest := n.dest
	if n.autoLink == autoLinkEmail && !hasPrefixFold(dest, "mailto:") {
		r.plain = append(append(r.plain[:0], "mailto:"...), dest...)
		dest = r.plain
	}
	s.start("a", attr{"href", r.urlEscape(dest), false})
	s.text(n.text)
	s.end("a")
}

// plainText returns the text content of n as HTML, used for image
// descriptions.
func (r *renderer) plainText(n *node) []byte {
	start := len(r.p.buf)
	r.appendPlain(n)
	return r.p.buf[start:len(r.p.buf):len(r.p.buf)]
}

func (r *renderer) appendPlain(n *node) {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindAutoLink:
			// Autolinks in angle brackets have no text in descriptions.
			if !c.angle {
				r.p.buf = appendEscapedText(r.p.buf, c.text)
			}
		case kindText:
			r.p.buf = appendEscapedText(r.p.buf, c.text)
		case kindCodeSpan:
			r.p.buf = appendEscapedText(r.p.buf, c.title)
		case kindSoftBreak, kindHardBreak:
			// Line breaks are rendered as in the text, unescaped.
			if c.kind == kindHardBreak || r.opts.HardWraps {
				if r.opts.XHTML {
					r.p.buf = append(r.p.buf, "<br />"...)
				} else {
					r.p.buf = append(r.p.buf, "<br>"...)
				}
			}
			r.p.buf = append(r.p.buf, '\n')
		case kindTypo:
			html, _ := r.opts.Typographer.entity(typo(c.level))
			r.p.buf = append(r.p.buf, html...)
		case kindFraction:
			// Descriptions keep fractions as written.
			r.p.buf = appendEscapedText(r.p.buf, c.text)
		default:
			r.appendPlain(c)
		}
	}
}

// urlEscape percent-encodes the characters of a URL that are not allowed.
func (r *renderer) urlEscape(dest []byte) []byte {
	needs := false
	for i := 0; i < len(dest); i++ {
		c := dest[i]
		if !urlSafe[c] && !(c == '%' && i+2 < len(dest) && isHex(dest[i+1]) && isHex(dest[i+2])) {
			needs = true
			break
		}
	}
	if !needs {
		return dest
	}
	start := len(r.p.buf)
	for i := 0; i < len(dest); i++ {
		c := dest[i]
		if urlSafe[c] || c == '%' && i+2 < len(dest) && isHex(dest[i+1]) && isHex(dest[i+2]) {
			r.p.buf = append(r.p.buf, c)
		} else {
			r.p.buf = append(r.p.buf, '%', hexDigits[c>>4], hexDigits[c&15])
		}
	}
	return r.p.buf[start:len(r.p.buf):len(r.p.buf)]
}

const hexDigits = "0123456789ABCDEF"

func isHex(c byte) bool {
	return isASCIIDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}

var urlSafe = func() (t [256]bool) {
	for c := 'a'; c <= 'z'; c++ {
		t[c] = true
	}
	for c := 'A'; c <= 'Z'; c++ {
		t[c] = true
	}
	for c := '0'; c <= '9'; c++ {
		t[c] = true
	}
	for _, c := range []byte("!#$&'()*+,-./:;=?@_~") {
		t[c] = true
	}
	return t
}()

func (r *renderer) footnoteID(kind string, fn *footnote, ref int) []byte {
	b := append(r.numBuf[:0], r.opts.FootnoteIDPrefix...)
	b = append(b, kind...)
	if ref > 0 {
		b = strconv.AppendInt(b, int64(ref), 10)
	}
	b = append(b, ':')
	return strconv.AppendInt(b, int64(fn.number), 10)
}

func (r *renderer) footnoteRef(n *node) {
	s := r.s
	fn := n.footnote
	start := len(r.p.buf)
	r.p.buf = append(r.p.buf, r.footnoteID("fnref", fn, n.refIndex)...)
	id := r.p.buf[start:len(r.p.buf):len(r.p.buf)]
	start = len(r.p.buf)
	r.p.buf = append(append(r.p.buf, '#'), r.footnoteID("fn", fn, 0)...)
	href := r.p.buf[start:len(r.p.buf):len(r.p.buf)]
	s.start("sup", attr{"id", id, false})
	s.start("a", attr{"href", href, false}, attr{"class", footnoteRefClass, false}, attr{"role", noteRefRole, false})
	s.text(strconv.AppendInt(r.numBuf[:0], int64(fn.number), 10))
	s.end("a")
	s.end("sup")
}

var (
	footnoteRefClass     = []byte("footnote-ref")
	footnoteBackrefClass = []byte("footnote-backref")
	noteRefRole          = []byte("doc-noteref")
	backlinkRole         = []byte("doc-backlink")
	footnotesClass       = []byte("footnotes")
	endnotesRole         = []byte("doc-endnotes")
)

func (r *renderer) backlinks(def *node) {
	s := r.s
	fn := def.footnote
	for i := 0; i < fn.refs; i++ {
		s.entity("&#160;", "\u00a0")
		start := len(r.p.buf)
		r.p.buf = append(append(r.p.buf, '#'), r.footnoteID("fnref", fn, i)...)
		s.start("a", attr{"href", r.p.buf[start:len(r.p.buf):len(r.p.buf)], false}, attr{"class", footnoteBackrefClass, false}, attr{"role", backlinkRole, false})
		s.entity("&#x21a9;&#xfe0e;", "\u21a9\ufe0e")
		s.end("a")
	}
}

func (r *renderer) renderFootnotes() {
	s := r.s
	s.start("div", attr{"class", footnotesClass, false}, attr{"role", endnotesRole, false})
	r.nl()
	s.start("hr")
	r.nl()
	s.start("ol")
	r.nl()
	for _, fn := range r.p.footnotes.ordered {
		def := fn.def
		def.footnote = fn
		start := len(r.p.buf)
		r.p.buf = append(r.p.buf, r.footnoteID("fn", fn, 0)...)
		s.start("li", attr{"id", r.p.buf[start:len(r.p.buf):len(r.p.buf)], false})
		r.nl()
		r.blocks(def)
		if def.last == nil || def.last.kind != kindParagraph {
			r.backlinks(def)
		}
		s.end("li")
		r.nl()
	}
	s.end("ol")
	r.nl()
	s.end("div")
	r.nl()
}

// htmlSink serializes tokens to HTML.
type htmlSink struct {
	out   *bytes.Buffer
	xhtml bool
}

func (h *htmlSink) text(b []byte) {
	writeEscaped(h.out, b)
}

func (h *htmlSink) entity(html, _ string) {
	h.out.WriteString(html)
}

func (h *htmlSink) start(name string, attrs ...attr) {
	out := h.out
	out.WriteByte('<')
	out.WriteString(name)
	for _, a := range attrs {
		out.WriteByte(' ')
		out.WriteString(a.key)
		out.WriteString(`="`)
		if a.escaped {
			out.Write(a.val)
		} else {
			writeEscaped(out, a.val)
		}
		out.WriteByte('"')
	}
	if h.xhtml && isVoid(name) {
		out.WriteString(" />")
	} else {
		out.WriteByte('>')
	}
}

func isVoid(name string) bool {
	switch name {
	case "br", "hr", "img", "input":
		return true
	}
	return false
}

func (h *htmlSink) end(name string) {
	h.out.WriteString("</")
	h.out.WriteString(name)
	h.out.WriteByte('>')
}

func (h *htmlSink) raw(b []byte) {
	h.out.Write(b)
}

// writeEscaped writes b with "&", "<", ">" and '"' escaped.
func writeEscaped(out *bytes.Buffer, b []byte) {
	start := 0
	for i, c := range b {
		var esc string
		switch c {
		case '&':
			esc = "&amp;"
		case '<':
			esc = "&lt;"
		case '>':
			esc = "&gt;"
		case '"':
			esc = "&quot;"
		case 0:
			esc = "\uFFFD"
		default:
			continue
		}
		out.Write(b[start:i])
		out.WriteString(esc)
		start = i + 1
	}
	out.Write(b[start:])
}

// appendEscapedText appends b to dst escaped as writeEscaped does.
func appendEscapedText(dst, b []byte) []byte {
	for _, c := range b {
		switch c {
		case '&':
			dst = append(dst, "&amp;"...)
		case '<':
			dst = append(dst, "&lt;"...)
		case '>':
			dst = append(dst, "&gt;"...)
		case '"':
			dst = append(dst, "&quot;"...)
		case 0:
			dst = append(dst, "\uFFFD"...)
		default:
			dst = append(dst, c)
		}
	}
	return dst
}

// replaceNUL returns b with NUL characters replaced by U+FFFD.
func (r *renderer) replaceNUL(b []byte) []byte {
	if bytes.IndexByte(b, 0) < 0 {
		return b
	}
	return bytes.ReplaceAll(b, []byte{0}, []byte("\uFFFD"))
}
