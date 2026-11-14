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

	plain     []byte
	numBuf    [24]byte
	listAttrs [2]attr

	// headings are the headings rendered so far, whose text and id are in
	// headingText, see headingList.
	headings    []headingRef
	headingText []byte
	slug        []byte
	// slugs counts the uses of heading slugs, see headingID.
	slugs map[string]int

	// sanitizing reports whether the sink sanitizes, which it cannot do for
	// raw HTML. stop is set when rendering stopped because of raw HTML.
	sanitizing, stop bool
	// stream renders streamed inline contents, see parseLeaf.
	stream func(chunk *node)
}

func (r *renderer) init(p *parser, opts *Options) {
	r.p = p
	r.opts = opts
	// The output of the previous render belongs to its caller.
	r.out = bytes.Buffer{}
	r.html = htmlSink{out: &r.out, xhtml: opts.XHTML}
	r.headings = r.headings[:0]
	r.headingText = r.headingText[:0]
	clear(r.slugs)
	r.sanitizing, r.stop = false, false
	if r.stream == nil {
		r.stream = r.renderChunk
	}
}

// release drops the references of r to the last render, and its buffers
// above the sizes that pooled parsers keep.
func (r *renderer) release() {
	r.p, r.opts, r.s = nil, nil, nil
	r.out = bytes.Buffer{}
	r.html = htmlSink{}
	if cap(r.plain) > maxPooledBytes {
		r.plain = nil
	}
	if cap(r.headings) > maxPooledSegs {
		r.headings = nil
	}
	if cap(r.headingText) > maxPooledBytes {
		r.headingText = nil
	}
	if len(r.slugs) > maxPooledRefs {
		r.slugs = nil
	}
}

// renderChunk renders inline contents that the parser streams, see
// parser.parseLeaf.
func (r *renderer) renderChunk(chunk *node) {
	if r.sanitizing && r.p.hasRawHTML {
		r.stop = true
	}
	if !r.stop {
		r.inlines(chunk)
	}
}

// parseLeaf parses the inline contents of the leaf block n, see
// parser.parseLeaf, streaming them if stream is true. It reports false when
// rendering must stop because the sanitizing sink met raw HTML.
func (r *renderer) parseLeaf(n *node, stream bool) (inlineMark, bool) {
	var f func(chunk *node)
	if stream {
		f = r.stream
	}
	m := r.p.parseLeaf(n, f)
	if r.stop {
		r.p.releaseLeaf(n, m)
		return m, false
	}
	if r.sanitizing && r.p.hasRawHTML {
		r.stop = true
		r.p.releaseLeaf(n, m)
		return m, false
	}
	return m, true
}

func (r *renderer) renderHTML(doc *node) {
	r.out.Grow(len(r.p.src) + len(r.p.src)/4)
	r.s = &r.html
	r.renderDocument(doc)
}

func (r *renderer) renderDocument(doc *node) {
	r.blocks(doc)
	if r.p.opts.Footnotes && !r.p.footnotes.inlined && len(r.p.footnotes.ordered) > 0 {
		r.p.parseFootnoteDefs()
		if r.sanitizing && r.p.hasRawHTML {
			r.stop = true
		}
	}
	if r.stop {
		return
	}
	if len(r.p.footnotes.ordered) > 0 {
		r.renderFootnotes()
	}
}

// renderClosed renders and frees the closed blocks at the start of the
// document, see parser.renderStream. It reports false when rendering
// stopped because the sanitizing sink met raw HTML.
func (r *renderer) renderClosed() bool {
	p := r.p
	for c := p.doc.first; c != nil && !c.open; c = p.doc.first {
		if r.sanitizing && p.hasRawHTML {
			r.stop = true
		}
		if !r.stop {
			r.block(c)
		}
		p.dropBlock(c)
	}
	if p.doc.first == nil {
		// Nothing refers to the lines and stored bytes of the freed blocks.
		p.segs = p.segs[:0]
		p.buf = p.buf[:0]
		p.joinedRaw, p.joined = nil, nil
	} else if k := int(p.doc.first.segStart); k >= stackChunk && 2*k >= len(p.segs) {
		// Drop the lines of the freed blocks. Moving at least as many lines
		// as are kept makes this take linear time overall.
		p.segs = p.segs[:copy(p.segs, p.segs[k:])]
		shiftSegs(p.doc.first, int32(k))
	}
	return !r.stop
}

// shiftSegs subtracts k from the line indices of n and its descendants.
// Nodes without lines, e.g., table rows, keep their zero indices.
func shiftSegs(n *node, k int32) {
	if n.segEnd >= k {
		n.segStart -= k
		n.segEnd -= k
	}
	for c := n.first; c != nil; c = c.next {
		shiftSegs(c, k)
	}
}

func (r *renderer) nl() {
	r.s.text(newline)
}

var newline = []byte("\n")

func (r *renderer) blocks(parent *node) {
	for n := parent.first; n != nil && !r.stop; n = n.next {
		r.block(n)
	}
}

func (r *renderer) block(n *node) {
	s := r.s
	switch n.kind {
	case kindParagraph:
		tight := n.parent != nil && n.parent.kind == kindListItem && n.parent.parent.tight
		if !tight {
			s.start("p")
		}
		m, ok := r.parseLeaf(n, true)
		if !ok {
			return
		}
		if tight {
			r.inlines(n)
			if n.next != nil {
				r.nl()
			}
			r.p.releaseLeaf(n, m)
			return
		}
		r.inlines(n)
		if n.parent.kind == kindFootnoteDef && n.next == nil {
			r.backlinks(n.parent)
		}
		s.end("p")
		r.nl()
		r.p.releaseLeaf(n, m)
	case kindHeading:
		m, ok := r.parseLeaf(n, false)
		if !ok {
			return
		}
		tag := headingTags[n.level]
		r.headingStart(n, tag)
		r.inlines(n)
		s.end(tag)
		r.nl()
		r.p.releaseLeaf(n, m)
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
			if sg.eol == sg.end {
				// The last line of the input has no line ending.
				r.nl()
			}
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
		}
		attrs := r.listAttrs[:0]
		if n.ordered && n.listStart != 1 {
			attrs = append(attrs, attr{"start", strconv.AppendInt(r.numBuf[:0], int64(n.listStart), 10), false})
		}
		if r.p.hasTaskItem(n) {
			attrs = append(attrs, attr{"class", containsTaskListClass, false})
		}
		s.start(tag, attrs...)
		r.nl()
		r.blocks(n)
		s.end(tag)
		r.nl()
	case kindListItem:
		if task, _ := r.p.taskItem(n); task {
			// The source lines let a client reorder or toggle the item by
			// editing the Markdown source.
			lines := strconv.AppendInt(r.numBuf[:0], int64(n.startLine), 10)
			lines = append(lines, '-')
			lines = strconv.AppendInt(lines, int64(n.lastLine), 10)
			s.start("li", attr{"class", taskListItemClass, false}, attr{"data-source-lines", lines, false})
		} else {
			s.start("li")
		}
		f := n.first
		for f != nil && f.kind == kindRefDefs {
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

var (
	containsTaskListClass = []byte("contains-task-list")
	taskListItemClass     = []byte("task-list-item")
)

// taskItem reports whether the list item starts with a task list item
// marker, and whether the marker is checked. The marker is "[ ]", "[x]" or
// "[X]", where a tab may stand for the space, at the start of a paragraph
// that is the first block of the item. It is followed by a space or tab, or
// ends the line.
func (p *parser) taskItem(item *node) (task, checked bool) {
	b := item.first
	if !p.opts.TaskLists || b == nil || b.kind != kindParagraph || b.segEnd == b.segStart {
		return false, false
	}
	s := p.segs[b.segStart]
	line := p.src[s.start:s.end]
	line = bytes.TrimRight(line, " \t\r\n")
	if len(line) < 3 || line[0] != '[' || line[2] != ']' || len(line) > 3 && line[3] != ' ' && line[3] != '\t' {
		return false, false
	}
	switch line[1] {
	case ' ', '\t':
	case 'x', 'X':
		checked = true
	default:
		return false, false
	}
	// Like GitHub, a marker needs text after it on its line, or must be the
	// whole paragraph and followed by a block other than a paragraph, e.g.,
	// a nested list.
	if len(line) == 3 &&
		(b.segEnd-b.segStart > 1 || b.next == nil || b.next.kind == kindParagraph) {
		return false, false
	}
	return true, checked
}

// hasTaskItem reports whether the list n has a task item.
func (p *parser) hasTaskItem(n *node) bool {
	for c := n.first; c != nil; c = c.next {
		if task, _ := p.taskItem(c); task {
			return true
		}
	}
	return false
}

// headingStart writes the start tag of the heading n, with an id when
// Options.HeadingIDs is set.
func (r *renderer) headingStart(n *node, tag string) {
	if !r.opts.HeadingIDs {
		r.s.start(tag)
		return
	}
	text := r.replaceNUL(r.appendTextContent(r.plain[:0], n))
	r.plain = text[:0]
	h := headingRef{level: n.level, text: int32(len(r.headingText))}
	r.headingText = append(r.headingText, text...)
	h.id = int32(len(r.headingText))
	if slug := r.headingID(text); slug != "" {
		r.headingText = append(append(r.headingText, r.opts.HeadingIDPrefix...), slug...)
		r.s.start(tag, attr{"id", r.headingText[h.id:], false})
	} else {
		r.s.start(tag)
	}
	r.headings = append(r.headings, h)
}

// headingRef is a heading whose text and id are in renderer.headingText:
// the text starts at the offset text and ends at id, where the id starts.
type headingRef struct {
	level, text, id int32
}

// headingList returns the headings rendered so far.
func (r *renderer) headingList() []Heading {
	if len(r.headings) == 0 {
		return nil
	}
	all := string(r.headingText)
	list := make([]Heading, len(r.headings))
	for i, h := range r.headings {
		end := len(all)
		if i+1 < len(r.headings) {
			end = int(r.headings[i+1].text)
		}
		list[i] = Heading{Level: int(h.level), Text: all[h.text:h.id], ID: all[h.id:end]}
	}
	return list
}

// appendTextContent appends the text of the inline content of n, as a
// browser shows it, without images and footnote references.
func (r *renderer) appendTextContent(b []byte, n *node) []byte {
	for c := n.first; c != nil; {
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
			if c.first != nil {
				c = c.first
				continue
			}
		}
		c = c.skipIn(n)
	}
	return b
}

// headingID returns the slug of a heading with the given text, as GitHub
// makes it: letters are lowercased, spaces become "-", and characters other
// than letters, marks, digits, "-" and "_" are dropped. A slug that an earlier
// heading has is followed by "-1", "-2" and so on.
func (r *renderer) headingID(text []byte) string {
	b := r.slug[:0]
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
	r.slug = b
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
	if i := bytes.IndexAny(info, " \t"); i >= 0 {
		info = info[:i]
	}
	return r.p.unescapeString(info)
}

func (r *renderer) table(t *node) {
	s := r.s
	s.start("table")
	r.nl()
	for row := t.first; row != nil && !r.stop; row = row.next {
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
		if row.first != nil {
			// The cells of rows in footnote definitions are parsed already.
			for c := row.first; c != nil; c = c.next {
				cells++
				if !r.tableCell(tag, c) {
					return
				}
			}
		} else {
			// The cells are rendered one at a time, so that a wide row does
			// not need a node for each.
			texts := splitTableRow(row.text, r.p.inl.rowCells[:0], len(t.ext.aligns))
			for i, text := range texts {
				m := r.p.arena.mark()
				c := r.p.arena.new(kindTableCell)
				c.header = row.header
				c.align = t.ext.aligns[i]
				c.text = text
				ok := r.tableCell(tag, c)
				r.p.arena.release(m)
				if !ok {
					return
				}
			}
			cells = len(texts)
			clear(texts)
			r.p.inl.rowCells = texts[:0]
		}
		// Short rows are filled with empty cells without alignment, as long
		// as the document has budget for them. Otherwise a wide header and
		// many short rows would make the output quadratic in the input.
		for ; cells < len(t.ext.aligns) && r.p.padBudget > 0; cells++ {
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

// tableCell renders the table cell c. It reports false when rendering must
// stop, see parseLeaf.
func (r *renderer) tableCell(tag string, c *node) bool {
	s := r.s
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
	m, ok := r.parseLeaf(c, true)
	if !ok {
		return false
	}
	r.inlines(c)
	r.p.releaseLeaf(c, m)
	s.end(tag)
	r.nl()
	return true
}

var (
	alignLeftValue   = []byte("left")
	alignCenterValue = []byte("center")
	alignRightValue  = []byte("right")
)

// inlines renders the inline contents of parent. It walks them without
// recursion, so that deeply nested contents, e.g., a run of thousands of "*",
// do not grow the stack.
func (r *renderer) inlines(parent *node) {
	n := parent.first
	for n != nil {
		if r.inlineStart(n) {
			if n.first != nil {
				n = n.first
				continue
			}
			r.inlineEnd(n)
		}
		for n.next == nil {
			n = n.parent
			if n == parent {
				return
			}
			r.inlineEnd(n)
		}
		n = n.next
	}
}

// nestedStrong reports whether n is strong emphasis directly inside strong
// emphasis, which adds nothing, as on GitHub, e.g., "****a****" renders one
// <strong>.
func nestedStrong(n *node) bool {
	return n.kind == kindStrong && n.parent != nil && n.parent.kind == kindStrong
}

// inlineEnd writes the end of the inline node n, whose children are
// rendered, see inlineStart.
func (r *renderer) inlineEnd(n *node) {
	switch n.kind {
	case kindEmphasis:
		r.s.end("em")
	case kindStrong:
		if !nestedStrong(n) {
			r.s.end("strong")
		}
	case kindStrikethrough:
		r.s.end("del")
	case kindLink:
		r.s.end("a")
	}
}

// inlineStart writes the inline node n, or only its start if its children
// are rendered next, which it reports.
func (r *renderer) inlineStart(n *node) bool {
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
		return true
	case kindStrong:
		if !nestedStrong(n) {
			s.start("strong")
		}
		return true
	case kindStrikethrough:
		s.start("del")
		return true
	case kindLink:
		href := r.urlEscape(n.ext.dest)
		if n.ext.hasTitle {
			s.start("a", attr{"href", href, false}, attr{"title", n.ext.title, false})
		} else {
			s.start("a", attr{"href", href, false})
		}
		return true
	case kindImage:
		href := r.urlEscape(n.ext.dest)
		alt := r.plainText(n)
		if n.ext.hasTitle {
			s.start("img", attr{"src", href, false}, attr{"alt", alt, true}, attr{"title", n.ext.title, false})
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
	return false
}

var checkboxValue = []byte("checkbox")

func (r *renderer) autoLink(n *node) {
	s := r.s
	if n.plain {
		s.text(r.p.unescapeString(n.text))
		return
	}
	dest := n.text
	if n.ext != nil {
		// The destination of "www." links has a scheme added.
		dest = n.ext.dest
	}
	if n.autoLink != autoLinkEmail && r.opts.ShortenAutoLink != nil {
		if html, ok := r.opts.ShortenAutoLink(dest); ok {
			s.raw(html)
			return
		}
	}
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
	for c := n.first; c != nil; {
		switch c.kind {
		case kindAutoLink, kindText, kindCodeSpan, kindRawHTML:
			// Raw HTML is text in a description, which is an attribute.
			// Line endings kept in text are spaces, as line breaks are.
			start := len(r.p.buf)
			r.p.buf = appendEscapedText(r.p.buf, c.text)
			for i := start; i < len(r.p.buf); i++ {
				if r.p.buf[i] == '\n' {
					r.p.buf[i] = ' '
				}
			}
		case kindSoftBreak, kindHardBreak:
			// Like GitHub, line breaks in a description are spaces.
			r.p.buf = append(r.p.buf, ' ')
		case kindTypo:
			html, _ := r.opts.Typographer.entity(typo(c.level))
			r.p.buf = append(r.p.buf, html...)
		case kindFraction:
			// Descriptions keep fractions as written.
			r.p.buf = appendEscapedText(r.p.buf, c.text)
		default:
			if c.first != nil {
				c = c.first
				continue
			}
		}
		c = c.skipIn(n)
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
	fn := n.ext.footnote
	start := len(r.p.buf)
	r.p.buf = append(r.p.buf, r.footnoteID("fnref", fn, n.ext.refIndex)...)
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
	fn := def.ext.footnote
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
		r.p.arena.newExt(def).footnote = fn
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
