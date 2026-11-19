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

// sink receives the rendered document as HTML tokens, which go to the
// sanitizing sink when it is set, else to the HTML sink. It is a struct
// rather than an interface so that the common HTML path makes direct calls.
type sink struct {
	html *htmlSink
	san  *sanitizeSink
}

// text writes text, which is not escaped yet.
func (s sink) text(b []byte) {
	if s.san != nil {
		s.san.text(b)
		return
	}
	s.html.text(b)
}

// entity writes a character reference, given in its HTML form and as text.
func (s sink) entity(html, text string) {
	if s.san != nil {
		s.san.entity(html, text)
		return
	}
	s.html.entity(html, text)
}

// start writes a start tag with attributes whose values are not escaped yet.
func (s sink) start(name string, attrs ...attr) {
	if s.san != nil {
		s.san.start(name, attrs...)
		return
	}
	s.html.start(name, attrs...)
}

func (s sink) end(name string) {
	if s.san != nil {
		s.san.end(name)
		return
	}
	s.html.end(name)
}

// raw writes raw HTML.
func (s sink) raw(b []byte) {
	if s.san != nil {
		s.san.raw(b)
		return
	}
	s.html.raw(b)
}

type renderer struct {
	p    *parser
	opts *Options
	out  outBuf
	s    sink
	html htmlSink
	san  sanitizeSink

	plain []byte
	// tokText is a buffer for text tokens, see tokenText.
	tokText []byte
	// code is a buffer for the content of code spans, see codeContent.
	code      []byte
	numBuf    [24]byte
	listAttrs [2]attr

	// headings are the headings rendered so far, whose text and id are in
	// headingText, see headingList.
	headings    []headingRef
	headingText []byte
	slug        []byte
	// slugs counts the uses of heading slugs, see headingID.
	slugs map[string]int

	// frontMatterSize is the size of the HTML of front matter at the start of
	// the output, see Result.FrontMatterSize.
	frontMatterSize int

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
	r.out = outBuf{}
	r.html = htmlSink{out: &r.out, xhtml: opts.XHTML}
	r.headings = r.headings[:0]
	r.headingText = r.headingText[:0]
	clear(r.slugs)
	r.frontMatterSize = 0
	r.sanitizing, r.stop = false, false
	if r.stream == nil {
		r.stream = r.renderChunk
	}
}

// release drops the references of r to the last render, and its buffers
// above the sizes that pooled parsers keep.
func (r *renderer) release() {
	r.p, r.opts, r.s = nil, nil, sink{}
	r.out = outBuf{}
	r.html = htmlSink{}
	r.san = sanitizeSink{run: r.san.run[:0], dropped: r.san.dropped[:0], attrs: r.san.attrs[:0], clean: r.san.clean[:0], buf: r.san.buf[:0], escaped: r.san.escaped[:0]}
	if cap(r.san.run) > maxPooledBytes || cap(r.san.escaped) > maxPooledBytes || cap(r.san.buf) > maxPooledBytes {
		r.san = sanitizeSink{}
	}
	if cap(r.plain) > maxPooledBytes {
		r.plain = nil
	}
	if cap(r.code) > maxPooledBytes {
		r.code = nil
	}
	if cap(r.tokText) > maxPooledBytes {
		r.tokText = nil
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
	// Short documents have more markup per byte of source.
	r.out.Grow(len(r.p.src) + len(r.p.src)/4 + min(len(r.p.src), 2048))
	r.s = sink{html: &r.html}
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

// plainLeaf renders the inline contents of the paragraph or table cell n
// without parsing them into nodes, and reports whether it did. It does so
// when they are only text, line breaks, code spans and simple emphasis, as is common in
// comments and documents written by AI agents. The output is what the inline
// parser and renderer write for the same contents.
func (r *renderer) plainLeaf(n *node) bool {
	opts := r.opts
	if n.parsed || opts.Typographer.Enabled {
		return false
	}
	src := n.text
	if n.kind == kindHeading && !n.setext && n.segEnd > n.segStart {
		s := r.p.segs[n.segStart]
		src = r.p.src[s.start:s.end]
	}
	special := &plainSpecial
	if opts.Linkify {
		special = &plainSpecialLinkify
	}
	if (opts.RelativeLinks || opts.TextAutoLinksInRawAnchors) && bytes.IndexByte(src, '[') >= 0 {
		// See parser.walkLinks.
		return false
	}
	r.p.footnoteDefsBefore(n)
	if r.sanitizing && r.p.hasRawHTML {
		// A footnote definition has raw HTML, which stops the sanitizing
		// render, see renderer.parseLeaf.
		return false
	}
	out := &r.out
	start := len(out.buf)
	inCell := n.kind == kindTableCell
	// text is the start of the text not written yet, and noCloser has a bit
	// for each length of backtick run that has no closing run left.
	text, from, box := 0, 0, boxNone
	if opts.TaskLists && len(src) > 0 && src[0] == '[' {
		// See inlineParser.taskCheckbox.
		text, box = r.taskCheckbox(n, src)
		from = text
		if box == boxChecked {
			r.html.start("input", attr{"checked", nil, false}, attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
			out.writeByte(' ')
		} else if box == boxUnchecked {
			r.html.start("input", attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
			out.writeByte(' ')
		}
	}
	var noCloser uint64
	// open is the length of the run of asterisks that opened emphasis, or 0.
	open := 0
	for i := from; i < len(src); {
		c := src[i]
		if !special[c] {
			i++
			continue
		}
		switch c {
		case '>', '"', 0:
			out.Write(src[text:i])
			out.WriteString(escapes[c])
			i++
			text = i
		case '.', ':':
			// See inlineParser.tryLinkify.
			at, n, ok := plainAutoLink(src, i, text, opts)
			if !ok {
				out.buf = out.buf[:start]
				return false
			}
			if n == 0 {
				i++
				continue
			}
			out.Write(src[text:at])
			bufLen := len(r.p.buf)
			out.WriteString(`<a href="`)
			writeEscaped(out, r.autoLinkDest(src[at:at+n], c == '.'))
			out.WriteString(`">`)
			r.p.buf = r.p.buf[:bufLen]
			writeEscaped(out, src[at:at+n])
			out.WriteString("</a>")
			i = at + n
			text = i
		case '\n':
			// See inlineParser.lineEnding.
			end := i
			if end > text && src[end-1] == '\r' {
				end--
			}
			spaces := 0
			for end-spaces > text && src[end-spaces-1] == ' ' {
				spaces++
			}
			hard := spaces >= 2
			for end-spaces > text && (src[end-spaces-1] == ' ' || src[end-spaces-1] == '\t') {
				spaces++
			}
			out.Write(src[text : end-spaces])
			if hard || opts.HardWraps {
				r.html.start("br")
			}
			out.writeByte('\n')
			i++
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			text = i
		case '`':
			// See inlineParser.codeSpan.
			end := i + 1
			for end < len(src) && src[end] == '`' {
				end++
			}
			k := end - i
			if k >= 64 {
				out.buf = out.buf[:start]
				return false
			}
			closer := -1
			if noCloser&(1<<k) == 0 {
				closer = scanBackticks(src, k, end)
			}
			if closer < 0 {
				// The run is literal text, and so are later runs of its length.
				noCloser |= 1 << k
				i = end
				continue
			}
			out.Write(src[text:i])
			out.WriteString("<code>")
			writeEscaped(out, r.codeContent(src[end:closer], inCell))
			out.WriteString("</code>")
			i = closer + k
			text = i
		case '[':
			// See inlineParser.closeBracket. Only inline links whose text
			// is plain and whose destination needs no unescaping are handled.
			n := plainLink(src, i, opts)
			if n == 0 {
				out.buf = out.buf[:start]
				return false
			}
			close := i + 1 + bytes.IndexByte(src[i+1:], ']')
			out.Write(src[text:i])
			bufLen := len(r.p.buf)
			out.WriteString(`<a href="`)
			writeEscaped(out, r.urlEscape(src[close+2:i+n-1]))
			out.WriteString(`">`)
			r.p.buf = r.p.buf[:bufLen]
			writeEscaped(out, src[i+1:close])
			out.WriteString("</a>")
			i += n
			text = i
		case '*':
			// Pairs of runs of one or two asterisks, with nothing else
			// between them that could be a delimiter, are emphasis and strong
			// emphasis. The delimiter algorithm matches them as written.
			end := i + 1
			for end < len(src) && src[end] == '*' {
				end++
			}
			k := end - i
			before, okBefore := flankClass(src, i-1)
			after, okAfter := flankClass(src, end)
			if k > 2 || !okBefore || !okAfter {
				out.buf = out.buf[:start]
				return false
			}
			left := after != flankSpace && (after != flankPunct || before != flankOther)
			right := before != flankSpace && (before != flankPunct || after != flankOther)
			tag := emTags[k-1]
			switch {
			case open == 0 && left:
				out.Write(src[text:i])
				out.writeByte('<')
				out.WriteString(tag)
				out.writeByte('>')
				open = k
			case open == k && right:
				out.Write(src[text:i])
				out.WriteString("</")
				out.WriteString(tag)
				out.writeByte('>')
				open = 0
			default:
				out.buf = out.buf[:start]
				return false
			}
			i = end
			text = i
		default:
			out.buf = out.buf[:start]
			return false
		}
	}
	if open != 0 {
		out.buf = out.buf[:start]
		return false
	}
	out.Write(src[text:])
	if san := r.s.san; san != nil {
		// The HTML written above only showed that the contents are plain.
		out.buf = out.buf[:start]
		r.plainLeafSanitized(src, inCell, from, box, san)
	}
	return true
}

const (
	boxNone = iota
	boxUnchecked
	boxChecked
)

// taskCheckbox returns where the text of the leaf n, whose contents src
// start with "[", starts after a task list item marker, and the checkbox the
// marker renders, see inlineParser.taskCheckbox.
func (r *renderer) taskCheckbox(n *node, src []byte) (int, int) {
	item := n.parent
	if n.kind != kindParagraph || item == nil || item.kind != kindListItem || item.first != n {
		return 0, boxNone
	}
	task, checked := r.p.taskItem(item)
	if !task {
		return 0, boxNone
	}
	next := 3
	for next < len(src) && (src[next] == ' ' || src[next] == '\t' || src[next] == '\n' || src[next] == '\r') {
		next++
	}
	if checked {
		return next, boxChecked
	}
	return next, boxUnchecked
}

// plainLeafSanitized passes the contents src of a leaf, which plainLeaf
// found plain, to the sanitizing sink s with the calls that the inline
// renderer would make.
func (r *renderer) plainLeafSanitized(src []byte, inCell bool, text, box int, s *sanitizeSink) {
	opts := r.opts
	switch box {
	case boxChecked:
		s.start("input", attr{"checked", nil, false}, attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
		s.text(spaces[:1])
	case boxUnchecked:
		s.start("input", attr{"disabled", nil, false}, attr{"type", checkboxValue, false})
		s.text(spaces[:1])
	}
	var noCloser uint64
	open := 0
	for i := text; i < len(src); {
		switch src[i] {
		case '\n':
			end := i
			if end > text && src[end-1] == '\r' {
				end--
			}
			spaces := 0
			for end-spaces > text && src[end-spaces-1] == ' ' {
				spaces++
			}
			hard := spaces >= 2
			for end-spaces > text && (src[end-spaces-1] == ' ' || src[end-spaces-1] == '\t') {
				spaces++
			}
			s.text(src[text : end-spaces])
			if hard || opts.HardWraps {
				s.start("br")
			}
			s.text(newline)
			i++
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			text = i
		case '`':
			end := i + 1
			for end < len(src) && src[end] == '`' {
				end++
			}
			k := end - i
			closer := -1
			if noCloser&(1<<k) == 0 {
				closer = scanBackticks(src, k, end)
			}
			if closer < 0 {
				noCloser |= 1 << k
				i = end
				continue
			}
			s.text(src[text:i])
			s.start("code")
			s.text(r.codeContent(src[end:closer], inCell))
			s.end("code")
			i = closer + k
			text = i
		case '.', ':':
			if !opts.Linkify {
				i++
				continue
			}
			at, n, _ := plainAutoLink(src, i, text, opts)
			if n == 0 {
				i++
				continue
			}
			s.text(src[text:at])
			bufLen := len(r.p.buf)
			s.start("a", attr{"href", r.autoLinkDest(src[at:at+n], src[i] == '.'), false})
			r.p.buf = r.p.buf[:bufLen]
			s.text(src[at : at+n])
			s.end("a")
			i = at + n
			text = i
		case '[':
			n := plainLink(src, i, opts)
			close := i + 1 + bytes.IndexByte(src[i+1:], ']')
			s.text(src[text:i])
			bufLen := len(r.p.buf)
			s.start("a", attr{"href", r.urlEscape(src[close+2 : i+n-1]), false})
			r.p.buf = r.p.buf[:bufLen]
			s.text(src[i+1 : close])
			s.end("a")
			i += n
			text = i
		case '*':
			end := i + 1
			for end < len(src) && src[end] == '*' {
				end++
			}
			s.text(src[text:i])
			if open == 0 {
				open = end - i
				s.start(emTags[open-1])
			} else {
				s.end(emTags[open-1])
				open = 0
			}
			i = end
			text = i
		default:
			i++
		}
	}
	s.text(src[text:])
}

// plainAutoLink returns the start and length of the extended autolink that
// the inline parser finds for the "." or ":" at src[i], with nothing written
// from text on, or a length of 0 if there is none. It reports false when
// plainLeaf must leave the contents to the inline parser.
func plainAutoLink(src []byte, i, text int, opts *Options) (at, n int, ok bool) {
	var scheme bool
	switch {
	case src[i] == '.' && i >= 3 && string(src[i-3:i]) == "www":
		at = i - 3
	case src[i] == ':' && bytes.HasPrefix(src[i+1:], []byte("//")):
		scheme = true
		switch {
		case i >= 5 && string(src[i-5:i]) == "https":
			at = i - 5
		case i >= 4 && string(src[i-4:i]) == "http":
			at = i - 4
		case i >= 3 && string(src[i-3:i]) == "ftp":
			at = i - 3
		default:
			return 0, 0, !opts.LinkifyAfterPunct
		}
	default:
		return 0, 0, true
	}
	if opts.LinkifyAfterPunct || opts.LinkifyURL != nil || opts.ShortenAutoLink != nil || at < text {
		return 0, 0, false
	}
	if at > 0 {
		switch src[at-1] {
		case ' ', '\t', '\n', '*', '_', '~', '(':
		default:
			return 0, 0, true
		}
	}
	if !scheme {
		return at, scanWWW(src[at:]), true
	}
	if m := linkifyURL(src[at:]); m > 0 {
		return at, trimLinkTail(src[at : at+m]), true
	}
	return 0, 0, true
}

// autoLinkDest returns the escaped destination of the extended autolink
// link, which starts with "www." if www is true, see renderer.autoLink.
func (r *renderer) autoLinkDest(link []byte, www bool) []byte {
	if !www {
		return r.urlEscape(link)
	}
	r.plain = append(append(r.plain[:0], "http://"...), link...)
	return r.urlEscape(r.plain)
}

// plainLink returns the length of the inline link at src[i], which is "[",
// if plainLeaf can render it, or 0. Its text must contain no character that
// plainLeaf handles and no autolink, and its destination must not be empty
// and must contain no whitespace, parentheses, "<", "\\" or "&", so that it
// has no title and needs no unescaping.
func plainLink(src []byte, i int, opts *Options) int {
	if i > 0 && src[i-1] == '!' {
		return 0
	}
	j := i + 1
	for ; j < len(src) && src[j] != ']'; j++ {
		if c := src[j]; plainSpecial[c] && c != '>' && c != '"' && c != 0 {
			return 0
		}
	}
	if j >= len(src) || j+1 >= len(src) || src[j+1] != '(' {
		return 0
	}
	label := src[i+1 : j]
	if len(label) > 0 && label[0] == '^' {
		return 0
	}
	if opts.Linkify && (bytes.IndexByte(label, '@') >= 0 || bytes.Contains(label, []byte("www.")) || bytes.Contains(label, []byte("://"))) {
		return 0
	}
	k := j + 2
	for ; k < len(src) && src[k] != ')'; k++ {
		switch c := src[k]; {
		case c <= ' ', c == '(', c == '<', c == '\\', c == '&', c == 0x7f:
			return 0
		}
	}
	if k >= len(src) || k == j+2 {
		return 0
	}
	return k + 1 - i
}

// escapes are the escapes of the characters that writeEscaped escapes and
// plainLeaf does not leave to the inline parser.
var escapes = [256]string{'>': "&gt;", '"': "&quot;", 0: "\uFFFD"}

var emTags = [2]string{"em", "strong"}

const (
	flankSpace = iota
	flankPunct
	flankOther
)

// flankClass returns the class of src[i] for deciding whether a delimiter
// run is left or right flanking (CommonMark 0.31.2, section 6.2): Unicode
// whitespace, which the ends of src count as, punctuation or other. It only
// classifies ASCII, and reports false for other characters.
func flankClass(src []byte, i int) (int, bool) {
	if i < 0 || i >= len(src) {
		return flankSpace, true
	}
	switch c := src[i]; {
	case c >= utf8.RuneSelf:
		return 0, false
	case c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r':
		return flankSpace, true
	case isASCIIPunct(c):
		return flankPunct, true
	}
	return flankOther, true
}

// plainSpecial marks the characters that plainLeaf handles, or for which it
// leaves the contents to the inline parser.
var (
	plainSpecial = withSpecial([256]bool{}, "\n\\`*_[]<&~>\"\x00")
	// plainSpecialLinkify adds the characters of autolinks: "@" of email
	// addresses, "." after "www" and ":" before "//".
	plainSpecialLinkify = withSpecial(plainSpecial, "@.:")
)

// scanBackticks returns the start of the first maximal run of n backticks in
// src at or after pos, which does not start within a run, or -1.
func scanBackticks(src []byte, n, pos int) int {
	for pos < len(src) {
		i := bytes.IndexByte(src[pos:], '`')
		if i < 0 {
			return -1
		}
		start := pos + i
		end := start + 1
		for end < len(src) && src[end] == '`' {
			end++
		}
		if end-start == n {
			return start
		}
		pos = end
	}
	return -1
}

// codeContent returns the content of a code span, as
// inlineParser.codeContent does.
func (r *renderer) codeContent(b []byte, inCell bool) []byte {
	if bytes.IndexByte(b, '\n') >= 0 || inCell && bytes.Contains(b, []byte(`\|`)) {
		out := r.code[:0]
		for i := 0; i < len(b); i++ {
			c := b[i]
			if inCell && c == '\\' && i+1 < len(b) && b[i+1] == '|' {
				continue
			}
			if c == '\n' {
				c = ' '
			}
			out = append(out, c)
		}
		r.code = out
		b = out
	}
	if len(b) >= 2 && b[0] == ' ' && b[len(b)-1] == ' ' {
		for _, c := range b {
			if c != ' ' {
				return b[1 : len(b)-1]
			}
		}
	}
	return b
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
		plain := r.plainLeaf(n)
		var m inlineMark
		if !plain {
			var ok bool
			if m, ok = r.parseLeaf(n, true); !ok {
				return
			}
			r.inlines(n)
		}
		if tight {
			if n.next != nil {
				r.nl()
			}
			if !plain {
				r.p.releaseLeaf(n, m)
			}
			return
		}
		if n.parent.kind == kindFootnoteDef && n.next == nil {
			r.backlinks(n.parent)
		}
		s.end("p")
		r.nl()
		if !plain {
			r.p.releaseLeaf(n, m)
		}
	case kindHeading:
		tag := headingTags[n.level]
		if !r.opts.HeadingIDs {
			s.start(tag)
			if r.plainLeaf(n) {
				s.end(tag)
				r.nl()
				return
			}
		}
		m, ok := r.parseLeaf(n, false)
		if !ok {
			return
		}
		if r.opts.HeadingIDs {
			r.headingStart(n, tag)
		}
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
	case kindFrontMatter:
		r.frontMatter(n)
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
	line = trimRightSpace(line, true)
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
	if !r.plainLeaf(c) {
		m, ok := r.parseLeaf(c, true)
		if !ok {
			return false
		}
		r.inlines(c)
		r.p.releaseLeaf(c, m)
	}
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
	out   *outBuf
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
	out.writeByte('<')
	out.WriteString(name)
	for _, a := range attrs {
		out.writeByte(' ')
		out.WriteString(a.key)
		out.WriteString(`="`)
		if a.escaped {
			out.Write(a.val)
		} else {
			writeEscaped(out, a.val)
		}
		out.writeByte('"')
	}
	if h.xhtml && isVoid(name) {
		out.WriteString(" />")
	} else {
		out.writeByte('>')
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
	h.out.writeByte('>')
}

func (h *htmlSink) raw(b []byte) {
	h.out.Write(b)
}

// writeEscaped writes b with "&", "<", ">" and '"' escaped.
func writeEscaped(out *outBuf, b []byte) {
	start := 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !needsEscape[c] {
			continue
		}
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

var needsEscape = withSpecial([256]bool{}, "&<>\"\x00")

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

// outBuf is an append-only output buffer. Its methods are small enough to be
// inlined, which matters for the many short writes of rendering.
type outBuf struct {
	buf []byte
}

func (o *outBuf) Write(b []byte)          { o.buf = append(o.buf, b...) }
func (o *outBuf) WriteString(s string)    { o.buf = append(o.buf, s...) }
func (o *outBuf) writeByte(c byte)        { o.buf = append(o.buf, c) }
func (o *outBuf) Len() int                { return len(o.buf) }
func (o *outBuf) Bytes() []byte           { return o.buf }
func (o *outBuf) AvailableBuffer() []byte { return o.buf[len(o.buf):] }

// Grow makes room for n more bytes.
func (o *outBuf) Grow(n int) {
	if cap(o.buf)-len(o.buf) < n {
		o.buf = append(o.buf[:len(o.buf):len(o.buf)], make([]byte, n)...)[:len(o.buf)]
	}
}
