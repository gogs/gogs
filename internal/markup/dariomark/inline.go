package dariomark

import (
	"bytes"
	"unicode/utf8"
)

// delimiter is a delimiter run on the delimiter stack. Runs of "*", "_" and
// "~" that can only open have no node until they match: they stay in the
// pending text, at the offset start, see materialize.
type delimiter struct {
	node             *node
	start            int32
	count, origCount int32
	prev, next       int32
	// prevSame and nextSame link the delimiters of the same character, see
	// processEmphasis.
	prevSame, nextSame int32
	char               byte
	canOpen, canClose  bool
}

// bracket is an opening bracket on the bracket stack. Its text is not a node
// of its own: it is in the pending text until a text node takes it, see
// bracketText.
type bracket struct {
	start       int32 // offset of "[" or "!["
	delimBottom int32 // the top of the delimiter stack when the bracket was pushed
}

// bracketText records that the brackets from the index first on, up to the
// next group, are in node, a text node that starts at the offset start.
// The node keeps its start when it is split.
type bracketText struct {
	first, start int32
	node         *node
}

type backtickRuns struct {
	built bool
	// positions of maximal backtick runs by length, and the next unused index.
	byLen map[int][]int
	next  map[int]int
}

type inlineParser struct {
	p      *parser
	src    []byte
	pos    int
	parent *node
	// textStart is the start of the text that has not been added as a node.
	textStart int

	delims   stack[delimiter]
	delimTop int32
	// openers counts the delimiters on the stack that can open, by
	// delimIndex. delimFirst is the bottom of the stack, or -1.
	openers    [5]int
	delimFirst int32
	// sameTop is the top delimiter of each character, by delimIndex, or -1.
	sameTop [5]int32

	// stream, when not nil, renders finished inline nodes of the leaf before
	// its end, see streamFlush. streamAllocs and streamPos are the node
	// count of the arena and the position at the last flush.
	stream       func(chunk *node)
	streamAllocs int
	streamPos    int
	brackets     stack[bracket]
	// bracketText holds the text nodes of the brackets below the index
	// unflushed. The brackets from unflushed on are still in the pending text.
	bracketText []bracketText
	unflushed   int
	// inactiveBelow is the number of brackets at the bottom of the stack
	// that cannot open links any more, because they would contain a link.
	inactiveBelow int
	// lazyFrom, if not -1, is the start of pending text that comes before
	// textStart, which keeps brackets and line endings in the text without
	// nodes of their own. See keepPending.
	lazyFrom  int
	html      htmlScanner
	backticks backtickRuns
	inCell    bool
	special   *[256]bool
	linkify   bool
	// boundary is the position where a linkified link may start, e.g., after
	// an inline node.
	boundary int

	cells    [][]byte
	rowCells [][]byte
	aligns   []alignment
	// scratch is a buffer for the text of adjacent text nodes.
	scratch []byte
}

func (ip *inlineParser) reset() {
	ip.p = nil
	ip.src = nil
	ip.parent = nil
	ip.delims.reset()
	ip.brackets.reset()
	clear(ip.bracketText)
	ip.bracketText = ip.bracketText[:0]
	if cap(ip.bracketText) > maxPooledSegs {
		ip.bracketText = nil
	}
	clear(ip.cells)
	ip.cells = ip.cells[:0]
	if cap(ip.scratch) > maxPooledBytes {
		ip.scratch = nil
	}
}

var inlineSpecial = func() (t [256]bool) {
	for _, c := range []byte("\n\\`*_[]!<&~") {
		t[c] = true
	}
	return t
}()

// The inline contents of leaf blocks are parsed while rendering, one leaf at
// a time, and dropped after the leaf is rendered, so that only the inline
// nodes of one leaf exist at a time. See parseLeaf and releaseLeaf.

// inlineMark is the state of the arena and the buffer before the inline
// contents of a leaf were parsed.
type inlineMark struct {
	arena arenaMark
	buf   int
}

// parseLeaf parses the inline contents of the leaf block n, unless they were
// parsed already, and returns the mark to pass to releaseLeaf. When stream is
// not nil, it may receive finished parts of the contents before the end, see
// inlineParser.streamFlush.
func (p *parser) parseLeaf(n *node, stream func(chunk *node)) inlineMark {
	if p.opts.Footnotes && !p.footnotes.inlined && len(p.footnotes.defs) > 0 && int(n.startLine) > p.footnotes.firstLine {
		p.parseFootnoteDefs()
	}
	m := inlineMark{arena: p.arena.mark(), buf: len(p.buf)}
	if n.parsed {
		return m
	}
	n.parsed = true
	p.rawAnchor = false
	p.inl.stream = stream
	switch n.kind {
	case kindParagraph:
		p.parseInlines(n, n.text, false)
	case kindHeading:
		text := n.text
		if !n.setext && n.segEnd > n.segStart {
			s := p.segs[n.segStart]
			text = p.src[s.start:s.end]
		}
		p.parseInlines(n, text, false)
	case kindTableCell:
		p.parseInlines(n, n.text, true)
	}
	p.inl.stream = nil
	if p.opts.RelativeLinks || p.opts.TextAutoLinksInRawAnchors {
		p.walkLinks(n, &p.rawAnchor)
	}
	return m
}

// releaseLeaf drops the inline contents of the leaf block n, which were
// parsed by parseLeaf, unless they belong to a footnote definition.
func (p *parser) releaseLeaf(n *node, m inlineMark) {
	if n.persistent {
		p.arena.free, p.arena.freeExt = m.arena.free, m.arena.freeExt
		return
	}
	n.first, n.last, n.parsed = nil, nil, false
	p.arena.release(m.arena)
	p.buf = p.buf[:m.buf]
	p.joinedRaw, p.joined = nil, nil
}

// parseFootnoteDefs parses the inline contents of all footnote definitions,
// in the order they closed. This happens before the first leaf block after
// the first definition, which decides the footnote numbering. The contents
// of definitions that are rendered are kept until the end of the document.
func (p *parser) parseFootnoteDefs() {
	p.footnotes.inlined = true
	for _, d := range p.footnotes.defs {
		used := p.footnotes.byLabel[footnoteKey(d.text)].def == d
		p.parseLeaves(d, used)
	}
}

func (p *parser) parseLeaves(n *node, keep bool) {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindTableRow:
			// The cells of rows in footnote definitions are parsed with the
			// definitions, which decides the footnote numbering.
			p.rowCells(n, c)
			p.parseLeaves(c, keep)
		case kindParagraph, kindHeading, kindTableCell:
			m := p.parseLeaf(c, nil)
			if keep {
				c.persistent = true
				p.footnotes.leaves = append(p.footnotes.leaves, c)
			}
			p.releaseLeaf(c, m)
		default:
			p.parseLeaves(c, keep)
		}
	}
}

func (p *parser) parseInlines(parent *node, src []byte, inCell bool) {
	ip := &p.inl
	ip.p = p
	ip.src = src
	ip.pos = 0
	ip.textStart = 0
	ip.parent = parent
	ip.inCell = inCell
	ip.delims.truncate(0)
	ip.delimTop, ip.delimFirst = -1, -1
	ip.openers = [5]int{}
	ip.sameTop = [5]int32{-1, -1, -1, -1, -1}
	ip.streamAllocs, ip.streamPos = p.arena.allocs, 0
	ip.brackets.truncate(0)
	clear(ip.bracketText)
	ip.bracketText = ip.bracketText[:0]
	ip.unflushed, ip.inactiveBelow, ip.lazyFrom = 0, 0, -1
	ip.html.reset()
	ip.backticks.built = false
	ip.special = p.specialChars()
	ip.linkify = p.opts.Linkify
	ip.boundary = 0
	if p.opts.TaskLists {
		ip.taskCheckbox()
	}
	ip.parse()
}

func (ip *inlineParser) parse() {
	src := ip.src
	opts := ip.p.opts
	special := ip.special
	for ip.pos < len(src) {
		if ip.stream != nil && ip.p.arena.allocs-ip.streamAllocs >= streamNodes {
			ip.streamFlush()
		}
		c := src[ip.pos]
		if ip.pos == ip.boundary && ip.linkify && isASCIIAlnum(c) && ip.tryLinkify() {
			continue
		}
		if !special[c] {
			ip.pos++
			continue
		}
		start := ip.pos
		handled := false
		switch c {
		case '\n':
			ip.lineEnding()
			handled = true
		case '\\':
			handled = ip.backslash()
		case '`':
			ip.codeSpan()
			handled = true
		case '*', '_':
			handled = ip.delimRun(c)
		case '~':
			if opts.Strikethrough {
				handled = ip.delimRun(c)
			}
		case '[':
			ip.openBracket(false)
			handled = true
		case '!':
			if ip.pos+1 < len(src) && src[ip.pos+1] == '[' {
				ip.openBracket(true)
				handled = true
			}
		case ']':
			handled = ip.closeBracket()
		case '<':
			handled = ip.angle()
		case '&':
			handled = ip.entity()
		}
		if !handled && opts.Typographer.Enabled {
			handled = ip.typographer(c)
		}
		if !handled {
			handled = ip.extension(c)
		}
		if !handled && ip.pos == start {
			ip.pos++
		}
	}
	ip.flushText(len(src))
	ip.processEmphasis(-1)
	if ip.linkify && bytes.IndexByte(src, '@') >= 0 {
		ip.linkifyEmails(ip.parent)
	}
}

// flushText adds the pending text up to end as a text node.
func (ip *inlineParser) flushText(end int) {
	start := ip.textStart
	if ip.lazyFrom >= 0 {
		start, ip.lazyFrom = ip.lazyFrom, -1
	}
	if end > start {
		n := ip.appendText(ip.src[start:end])
		if ip.unflushed < ip.brackets.len() {
			ip.bracketText = append(ip.bracketText, bracketText{first: int32(ip.unflushed), start: int32(start), node: n})
			ip.unflushed = ip.brackets.len()
		}
	}
	ip.textStart = end
}

// keepPending keeps the pending text and what follows it up to the next
// textStart in one text node, instead of ending the text node here.
func (ip *inlineParser) keepPending() {
	if ip.lazyFrom < 0 {
		ip.lazyFrom = ip.textStart
	}
}

func (ip *inlineParser) appendText(b []byte) *node {
	n := ip.p.arena.new(kindText)
	n.text = b
	ip.parent.appendChild(n)
	return n
}

// appendNode flushes the pending text up to the current position, appends n
// and continues the pending text at next.
func (ip *inlineParser) appendNode(n *node, next int) {
	ip.flushText(ip.pos)
	ip.parent.appendChild(n)
	ip.pos = next
	ip.textStart = next
	ip.boundary = next
}

func (ip *inlineParser) lineEnding() {
	src := ip.src
	end := ip.pos
	if end > ip.textStart && src[end-1] == '\r' {
		end--
	}
	spaces := 0
	for end-spaces > ip.textStart && src[end-spaces-1] == ' ' {
		spaces++
	}
	k := kindSoftBreak
	if spaces >= 2 {
		k = kindHardBreak
	}
	// Like GitHub, tabs before a line ending are removed with the spaces.
	for end-spaces > ip.textStart && (src[end-spaces-1] == ' ' || src[end-spaces-1] == '\t') {
		spaces++
	}
	next := ip.pos + 1
	if k == kindSoftBreak && end-spaces == ip.pos && !ip.p.opts.HardWraps &&
		(next >= len(src) || src[next] != ' ' && src[next] != '\t') {
		// The line ending renders as it is, so it stays in the text.
		ip.keepPending()
	} else {
		ip.flushText(end - spaces)
		ip.parent.appendChild(ip.p.arena.new(k))
	}

	for next < len(src) && (src[next] == ' ' || src[next] == '\t') {
		next++
	}
	ip.pos, ip.textStart, ip.boundary = next, next, next
}

func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

func (ip *inlineParser) backslash() bool {
	src := ip.src
	i := ip.pos + 1
	if i < len(src) && src[i] == '\n' {
		ip.flushText(ip.pos)
		ip.parent.appendChild(ip.p.arena.new(kindHardBreak))
		next := i + 1
		for next < len(src) && (src[next] == ' ' || src[next] == '\t') {
			next++
		}
		ip.pos, ip.textStart, ip.boundary = next, next, next
		return true
	}
	if i < len(src) && isASCIIPunct(src[i]) {
		ip.flushText(ip.pos)
		// The escaped character starts the next text node.
		ip.textStart = i
		ip.pos = i + 1
		if c := src[i]; c == '*' || c == '_' || c == '~' || c == '(' {
			// Extended autolinks may follow these characters, see tryLinkify.
			ip.boundary = ip.pos
		}
		return true
	}
	return false
}

func (ip *inlineParser) buildBackticks() {
	b := &ip.backticks
	b.built = true
	if b.byLen == nil {
		b.byLen = make(map[int][]int)
		b.next = make(map[int]int)
	}
	for k, v := range b.byLen {
		b.byLen[k] = v[:0]
	}
	clear(b.next)
	src := ip.src
	for i := 0; i < len(src); {
		if src[i] != '`' {
			j := bytes.IndexByte(src[i:], '`')
			if j < 0 {
				break
			}
			i += j
			continue
		}
		j := i
		for j < len(src) && src[j] == '`' {
			j++
		}
		b.byLen[j-i] = append(b.byLen[j-i], i)
		i = j
	}
}

// findBackticks returns the start of the first maximal run of n backticks at
// or after pos, or -1.
func (ip *inlineParser) findBackticks(n, pos int) int {
	b := &ip.backticks
	if !b.built {
		ip.buildBackticks()
	}
	list := b.byLen[n]
	k := b.next[n]
	for k < len(list) && list[k] < pos {
		k++
	}
	b.next[n] = k
	if k < len(list) {
		return list[k]
	}
	return -1
}

func (ip *inlineParser) codeSpan() {
	src := ip.src
	start := ip.pos
	end := start
	for end < len(src) && src[end] == '`' {
		end++
	}
	n := end - start
	closer := ip.findBackticks(n, end)
	if closer < 0 {
		// The whole run is literal text.
		ip.pos = end
		ip.unmatchedBackticks(end)
		return
	}
	code := ip.p.arena.new(kindCodeSpan)
	code.text = ip.codeContent(src[end:closer])
	ip.appendNode(code, closer+n)
}

// codeContent returns the content of a code span (CommonMark 0.31.2,
// section 6.1).
func (ip *inlineParser) codeContent(b []byte) []byte {
	if bytes.IndexByte(b, '\n') >= 0 || (ip.inCell && bytes.Contains(b, []byte(`\|`))) {
		out := ip.p.buf[len(ip.p.buf):]
		for i := 0; i < len(b); i++ {
			if ip.inCell && b[i] == '\\' && i+1 < len(b) && b[i+1] == '|' {
				continue
			}
			out = append(out, b[i])
		}
		for i, c := range out {
			if c == '\n' {
				out[i] = ' '
			}
		}
		ip.p.buf = append(ip.p.buf, out...)
		b = ip.p.buf[len(ip.p.buf)-len(out):]
	}
	if len(b) >= 2 && b[0] == ' ' && b[len(b)-1] == ' ' {
		// One space is stripped from both sides, unless the content is
		// only spaces.
		for _, c := range b {
			if c != ' ' {
				return b[1 : len(b)-1]
			}
		}
	}
	return b
}

func (ip *inlineParser) angle() bool {
	src := ip.src
	if n, ok := scanAutolinkURI(src[ip.pos:]); ok {
		link := ip.p.arena.new(kindAutoLink)
		link.text = src[ip.pos+1 : ip.pos+n-1]
		link.autoLink = autoLinkURL
		link.angle = true
		ip.appendNode(link, ip.pos+n)
		return true
	}
	if n, ok := scanAutolinkEmail(src[ip.pos:]); ok {
		link := ip.p.arena.new(kindAutoLink)
		link.text = src[ip.pos+1 : ip.pos+n-1]
		link.autoLink = autoLinkEmail
		link.angle = true
		ip.appendNode(link, ip.pos+n)
		return true
	}
	if n := ip.html.scan(src, ip.pos); n > 0 {
		h := ip.p.arena.new(kindRawHTML)
		h.text = src[ip.pos : ip.pos+n]
		ip.p.hasRawHTML = true
		ip.appendNode(h, ip.pos+n)
		return true
	}
	return false
}

func scanAutolinkURI(b []byte) (int, bool) {
	if len(b) < 4 || b[0] != '<' || !isASCIILetter(b[1]) {
		return 0, false
	}
	i := 2
	for i < len(b) && i-1 <= 32 && (isASCIIAlnum(b[i]) || b[i] == '+' || b[i] == '.' || b[i] == '-') {
		i++
	}
	if i-1 < 2 || i-1 > 32 || i >= len(b) || b[i] != ':' {
		return 0, false
	}
	for i++; i < len(b); i++ {
		c := b[i]
		if c == '>' {
			return i + 1, true
		}
		if c <= ' ' || c == '<' || c == 0x7f {
			return 0, false
		}
	}
	return 0, false
}

func isEmailLocal(c byte) bool {
	return isASCIIAlnum(c) || bytes.IndexByte([]byte(".!#$%&'*+/=?^_`{|}~-"), c) >= 0
}

func scanAutolinkEmail(b []byte) (int, bool) {
	if len(b) < 2 || b[0] != '<' {
		return 0, false
	}
	i := 1
	for i < len(b) && isEmailLocal(b[i]) {
		i++
	}
	if i == 1 || i >= len(b) || b[i] != '@' {
		return 0, false
	}
	i++
	for {
		// A label of letters, digits and hyphens that neither starts nor ends
		// with a hyphen.
		s := i
		for i < len(b) && i-s < 63 && (isASCIIAlnum(b[i]) || b[i] == '-') {
			i++
		}
		if i == s || b[s] == '-' || b[i-1] == '-' {
			return 0, false
		}
		if i >= len(b) {
			return 0, false
		}
		switch b[i] {
		case '>':
			return i + 1, true
		case '.':
			i++
		default:
			return 0, false
		}
	}
}

func (ip *inlineParser) entity() bool {
	r, n := decodeEntity(ip.src[ip.pos:])
	if n == 0 {
		return false
	}
	t := ip.p.arena.new(kindText)
	t.text = r
	ip.appendNode(t, ip.pos+n)
	// Entities do not end a text run for linkify.
	ip.boundary = -1
	return true
}

var replacementChar = []byte("\uFFFD")

// decodeEntity decodes the entity or numeric character reference at the
// start of b and returns its replacement and length.
func decodeEntity(b []byte) ([]byte, int) {
	return decodeEntityOctal(b, false)
}

// decodeEntityOctal decodes like decodeEntity. If octal is true, decimal
// numeric character references with a leading zero are octal, and invalid
// octal numbers are replaced with U+FFFD.
func decodeEntityOctal(b []byte, octal bool) ([]byte, int) {
	if len(b) < 3 || b[0] != '&' {
		return nil, 0
	}
	if b[1] == '#' {
		i := 2
		hex := false
		if i < len(b) && (b[i] == 'x' || b[i] == 'X') {
			hex = true
			i++
		}
		s := i
		r := 0
		for i < len(b) {
			c := b[i]
			var d int
			switch {
			case isASCIIDigit(c):
				d = int(c - '0')
			case hex && 'a' <= c && c <= 'f':
				d = int(c-'a') + 10
			case hex && 'A' <= c && c <= 'F':
				d = int(c-'A') + 10
			default:
				d = -1
			}
			if d < 0 {
				break
			}
			if hex {
				r = r*16 + d
			} else {
				r = r*10 + d
			}
			i++
			// Like GitHub, up to 8 digits are read, more than CommonMark
			// allows.
			if i-s > 8 {
				return nil, 0
			}
		}
		if i == s || i >= len(b) || b[i] != ';' {
			return nil, 0
		}
		if octal && !hex && b[s] == '0' {
			r = 0
			for _, c := range b[s:i] {
				if c > '7' {
					r = 0
					break
				}
				r = r*8 + int(c-'0')
			}
		}
		if r == 0 || r > utf8.MaxRune || (r >= 0xD800 && r <= 0xDFFF) {
			return replacementChar, i + 1
		}
		return utf8.AppendRune(nil, rune(r)), i + 1
	}
	i := 1
	for i < len(b) && i <= 32 && isASCIIAlnum(b[i]) {
		i++
	}
	if i == 1 || i >= len(b) || b[i] != ';' {
		return nil, 0
	}
	if s, ok := entities[string(b[1:i])]; ok {
		return []byte(s), i + 1
	}
	return nil, 0
}

// unescapeDest resolves backslash escapes and then entities in a link
// destination, see decodeEntityOctal.
func (p *parser) unescapeDest(b []byte) []byte {
	if bytes.IndexByte(b, '\\') >= 0 {
		start := len(p.buf)
		for i := 0; i < len(b); i++ {
			if b[i] == '\\' && i+1 < len(b) && isASCIIPunct(b[i+1]) {
				i++
			}
			p.buf = append(p.buf, b[i])
		}
		b = p.buf[start:len(p.buf):len(p.buf)]
	}
	if bytes.IndexByte(b, '&') < 0 {
		return b
	}
	start := len(p.buf)
	for i := 0; i < len(b); {
		if b[i] == '&' {
			if r, n := decodeEntityOctal(b[i:], true); n > 0 {
				p.buf = append(p.buf, r...)
				i += n
				continue
			}
		}
		p.buf = append(p.buf, b[i])
		i++
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}

// unescapeString resolves backslash escapes and entities in b.
func (p *parser) unescapeString(b []byte) []byte {
	if bytes.IndexByte(b, '\\') < 0 && bytes.IndexByte(b, '&') < 0 {
		return b
	}
	start := len(p.buf)
	for i := 0; i < len(b); {
		c := b[i]
		if c == '\\' && i+1 < len(b) && isASCIIPunct(b[i+1]) {
			p.buf = append(p.buf, b[i+1])
			i += 2
			continue
		}
		if c == '&' {
			if r, n := decodeEntity(b[i:]); n > 0 {
				p.buf = append(p.buf, r...)
				i += n
				continue
			}
		}
		p.buf = append(p.buf, c)
		i++
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}

// streamNodes is the number of nodes allocated for a leaf after which
// streamFlush renders the finished ones.
const streamNodes = 1024

// streamFlush renders and frees the children of the leaf that are finished:
// those before the earliest open bracket or delimiter, which a link or
// emphasis may still take. A run of text nodes before that point stays,
// because email autolinks are found in runs of text nodes after parsing, see
// linkifyEmails. Only the end of such a run that may still be part of an
// email address stays when the run ends the children.
func (ip *inlineParser) streamFlush() {
	ip.streamAllocs = ip.p.arena.allocs
	bound := ip.pos
	var boundNode *node
	if ip.brackets.len() > 0 {
		bound = int(ip.brackets.at(0).start)
	}
	if ip.delimFirst >= 0 {
		if d := ip.delims.at(int(ip.delimFirst)); int(d.start) < bound {
			bound, boundNode = int(d.start), d.node
		}
	}
	parent := ip.parent
	var cut *node
	for c := parent.first; c != nil; c = c.next {
		if c == boundNode || c.kind == kindText && ip.isSrcText(c) && ip.srcStart(c)+len(c.text) > bound {
			cut = c
			break
		}
	}
	// The run of text nodes before cut, or at the end.
	runEnd := parent.last
	if cut != nil {
		runEnd = cut.prev
	}
	run := runEnd
	for run != nil && run.kind == kindText && run.prev != nil && run.prev.kind == kindText {
		run = run.prev
	}
	if run != nil && run.kind != kindText {
		run = nil
	}
	last := runEnd
	if run != nil {
		last = run.prev
		if cut == nil {
			last = ip.splitEmailTail(run, runEnd)
		}
	}
	if last == nil {
		return
	}
	holder := ip.p.arena.new(kindParagraph)
	for c := parent.first; ; {
		next := c.next
		c.unlink()
		holder.appendChild(c)
		if c == last {
			break
		}
		c = next
	}
	if ip.linkify {
		ip.linkifyEmails(holder)
	}
	if ip.p.opts.RelativeLinks || ip.p.opts.TextAutoLinksInRawAnchors {
		ip.p.walkLinks(holder, &ip.p.rawAnchor)
	}
	ip.stream(holder)
	ip.p.arena.freeTree(holder)
	ip.streamPos = ip.pos
}

// splitEmailTail splits the text of the text nodes from first to last, which
// end the children, into the part that may still become part of an email
// address, which stays as one text node, and the part before it, which is
// returned as one text node, or nil if there is none. The run is only
// copied when it is shorter than the input parsed since the last flush, so
// that copying stays linear.
func (ip *inlineParser) splitEmailTail(first, last *node) *node {
	var text []byte
	if first == last {
		text = first.text
	} else {
		n := 0
		for c := first; ; c = c.next {
			n += len(c.text)
			if c == last {
				break
			}
		}
		if n > ip.pos-ip.streamPos {
			return first.prev
		}
		start := len(ip.p.buf)
		for c := first; ; c = c.next {
			ip.p.buf = append(ip.p.buf, c.text...)
			if c == last {
				break
			}
		}
		text = ip.p.buf[start:len(ip.p.buf):len(ip.p.buf)]
	}
	k := len(text)
	for k > 0 && (emailLocal[text[k-1]] || text[k-1] == '@') {
		k--
	}
	prev := first.prev
	if k == 0 && first == last {
		return prev
	}
	// Replace the run by the two parts.
	for c := first; ; {
		next := c.next
		c.unlink()
		ip.p.arena.freeTree(c)
		if c == last {
			break
		}
		c = next
	}
	var head *node
	if k > 0 {
		head = ip.p.arena.new(kindText)
		head.text = text[:k]
		ip.parent.appendChild(head)
	}
	if k < len(text) {
		tail := ip.p.arena.new(kindText)
		tail.text = text[k:]
		ip.parent.appendChild(tail)
	}
	if head != nil {
		return head
	}
	return prev
}
