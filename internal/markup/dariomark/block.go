package dariomark

import (
	"bytes"
)

// maxNesting is the maximum depth of nested block quotes, list items and
// footnote definitions. Deeper markers are parsed as text.
const maxNesting = 100

type linkRef struct {
	// rawDest is dest before backslash escapes and entities are resolved.
	dest, rawDest, title []byte
}

type parser struct {
	opts *Options
	src  []byte
	segs []seg
	// buf holds bytes that are not slices of src, e.g., paragraph contents
	// that span container prefixes.
	buf   []byte
	arena arena

	doc, tip, oldtip     *node
	lastMatchedContainer *node
	allClosed            bool

	line                 []byte // current line without its line ending
	lineStart            int    // offset of line in src
	lineEOL              int    // offset after the line ending
	lineNumber           int
	offset, column       int
	nextNonspace         int
	nextNonspaceColumn   int
	indent               int
	blank                bool
	partiallyConsumedTab bool
	// curBlankIn is the container in which the current line counts as blank,
	// or nil. prevBlankIn is the same for the previous line.
	prevBlankIn, curBlankIn *node
	// emptiedParagraph reports whether a table took all lines of the
	// paragraph that the current line would underline, which lets the line
	// start an empty list item.
	emptiedParagraph bool
	depth            int

	// crlf holds a copy of the source with line endings normalized, see
	// normalizeLineEndings.
	crlf []byte

	refs      map[string]linkRef
	footnotes footnoteSet

	hasRawHTML bool
	// rawAnchor reports whether the inline contents parsed so far of the
	// current leaf are inside a raw HTML anchor, see walkLinks.
	rawAnchor bool
	// refBudget is the number of bytes of reference destinations and titles
	// that the rest of the document may expand, see spendRefBudget.
	refBudget int
	// joined is the last relative link destination joined with
	// Options.LinkBase, from joinedRaw, see walkLinks. It is in buf.
	joinedRaw, joined []byte
	// padBudget is the number of empty cells that the rest of the document
	// may add to fill short table rows.
	padBudget int

	inl inlineParser
	// r renders the document. It is part of the parser so that its
	// buffers are reused.
	r renderer
}

// Pooled parsers drop buffers and maps above these sizes, about 1 MB, so
// that rendering a huge document does not keep its memory alive.
const (
	maxPooledBytes = 1 << 20
	maxPooledSegs  = 1 << 16
	maxPooledRefs  = 1 << 12
)

func (p *parser) reset() {
	p.r.release()
	p.arena.reset()
	p.segs = p.segs[:0]
	if cap(p.segs) > maxPooledSegs {
		p.segs = nil
	}
	p.buf = p.buf[:0]
	p.joinedRaw, p.joined = nil, nil
	if cap(p.buf) > maxPooledBytes {
		p.buf = nil
	}
	p.src = nil
	p.crlf = p.crlf[:0]
	if cap(p.crlf) > maxPooledBytes {
		p.crlf = nil
	}
	p.opts = nil
	p.doc, p.tip, p.oldtip, p.lastMatchedContainer = nil, nil, nil, nil
	p.prevBlankIn, p.curBlankIn = nil, nil
	p.line = nil
	p.depth = 0
	// Cleared maps keep their size.
	if len(p.refs) > maxPooledRefs {
		p.refs = nil
	}
	clear(p.refs)
	if len(p.footnotes.byLabel) > maxPooledRefs {
		p.footnotes.byLabel = nil
	}
	p.footnotes.reset()
	p.hasRawHTML = false
	p.inl.reset()
}

// store copies b into the parser's buffer and returns the copy.
func (p *parser) store(b []byte) []byte {
	start := len(p.buf)
	p.buf = append(p.buf, b...)
	return p.buf[start:len(p.buf):len(p.buf)]
}

// normalizeLineEndings returns src with every "\r\n" and "\r" replaced by
// "\n". The three are line endings (CommonMark 0.31.2, section 2.1), which
// are rendered as "\n".
func (p *parser) normalizeLineEndings(src []byte) []byte {
	i := bytes.IndexByte(src, '\r')
	if i < 0 {
		return src
	}
	out := append(p.crlf[:0], src[:i]...)
	for ; i < len(src); i++ {
		c := src[i]
		if c == '\r' {
			c = '\n'
			if i+1 < len(src) && src[i+1] == '\n' {
				i++
			}
		}
		out = append(out, c)
	}
	p.crlf = out
	return out
}

// parse parses src into a document tree. If flush is not nil, it is called
// after each line while the first child of the document is closed, and once
// at the end. Parsing stops when it returns false.
func (p *parser) parse(src []byte, flush func() bool) *node {
	src = p.normalizeLineEndings(src)
	p.src = src
	p.refBudget = max(10*len(src), 1<<20)
	p.padBudget = len(src)
	p.doc = p.arena.new(kindDocument)
	p.doc.open = true
	p.tip = p.doc
	p.lineNumber = 0
	// The start of the input counts as a blank line.
	p.curBlankIn = p.doc
	for pos := 0; pos < len(src); {
		end := bytes.IndexByte(src[pos:], '\n')
		var eol int
		if end < 0 {
			end = len(src)
			eol = end
		} else {
			end += pos
			eol = end + 1
		}
		contentEnd := end
		if contentEnd > pos && src[contentEnd-1] == '\r' && eol > end {
			contentEnd--
		}
		p.lineStart = pos
		p.lineEOL = eol
		p.line = src[pos:contentEnd]
		p.lineNumber++
		p.incorporateLine()
		pos = eol
		if flush != nil && p.doc.first != nil && !p.doc.first.open && !flush() {
			return p.doc
		}
	}
	for p.tip != nil {
		p.finalize(p.tip)
	}
	if flush != nil {
		flush()
	}
	if p.opts.Footnotes {
		p.detachFootnoteDefs()
	}
	return p.doc
}

// tabWidth returns the number of columns of a tab at column col. Tab stops
// are every four columns (CommonMark 0.31.2, section 2.2).
func tabWidth(col int) int {
	return 4 - col%4
}

func (p *parser) findNextNonspace() {
	i, cols := p.offset, p.column
scan:
	for i < len(p.line) {
		switch p.line[i] {
		case ' ':
			cols++
		case '\t':
			cols += tabWidth(cols)
		default:
			break scan
		}
		i++
	}
	p.blank = i >= len(p.line)
	p.nextNonspace = i
	p.nextNonspaceColumn = cols
	p.indent = cols - p.column
}

func (p *parser) advanceNextNonspace() {
	p.offset = p.nextNonspace
	p.column = p.nextNonspaceColumn
	p.partiallyConsumedTab = false
}

// advanceOffset advances by count bytes, or by count columns if columns is
// true, in which case tabs may be partially consumed.
func (p *parser) advanceOffset(count int, columns bool) {
	for count > 0 && p.offset < len(p.line) {
		c := p.line[p.offset]
		if c == '\t' {
			toTab := tabWidth(p.column)
			if columns {
				p.partiallyConsumedTab = toTab > count
				n := min(toTab, count)
				p.column += n
				if !p.partiallyConsumedTab {
					p.offset++
				}
				count -= n
			} else {
				p.partiallyConsumedTab = false
				p.column += toTab
				p.offset++
				count--
			}
		} else {
			p.partiallyConsumedTab = false
			p.offset++
			p.column++
			count--
		}
	}
}

func (p *parser) peek(i int) byte {
	if i < len(p.line) {
		return p.line[i]
	}
	return 0
}

// isParagraph reports whether b is a paragraph that has not become a table.
// The rows of a table end at any block start (GFM 0.29, section 4.10),
// including those that cannot interrupt a paragraph.
func isParagraph(b *node) bool {
	return b.kind == kindParagraph && !b.tableRows
}

func acceptsLines(k kind) bool {
	return k == kindParagraph || k == kindCodeBlock || k == kindHTMLBlock
}

func canContain(parent kind, child kind) bool {
	switch parent {
	case kindDocument, kindBlockquote, kindListItem, kindFootnoteDef:
		return child != kindListItem
	case kindList:
		return child == kindListItem
	}
	return false
}

func (p *parser) addChild(k kind) *node {
	for !canContain(p.tip.kind, k) {
		p.finalize(p.tip)
	}
	n := p.arena.new(k)
	n.open = true
	n.startLine = int32(p.lineNumber)
	n.segStart = int32(len(p.segs))
	n.segEnd = int32(len(p.segs))
	p.tip.appendChild(n)
	p.tip = n
	if k == kindBlockquote || k == kindListItem || k == kindFootnoteDef {
		p.depth++
	}
	p.touch(n)
	return n
}

// touch records that the current line belongs to b and its ancestors, which
// decides whether lists are loose.
func (p *parser) touch(b *node) {
	for ; b != nil && b.lastLine < int32(p.lineNumber); b = b.parent {
		b.lastLine = int32(p.lineNumber)
	}
}

// addLine adds the rest of the current line to the tip.
func (p *parser) addLine() {
	s := seg{start: int32(p.lineStart + p.offset), end: int32(p.lineStart + len(p.line)), eol: int32(p.lineEOL)}
	if p.partiallyConsumedTab {
		s.start++
		s.pad = int8(tabWidth(p.column))
	}
	if s.start > s.end {
		s.start = s.end
	}
	s.indent = int8(min(p.indent, 127))
	p.segs = append(p.segs, s)
	p.tip.segEnd = int32(len(p.segs))
}

const (
	continueMatched = iota
	continueFailed
	continueDone
)

func (p *parser) continueBlock(n *node) int {
	switch n.kind {
	case kindBlockquote:
		if p.indent <= 3 && p.peek(p.nextNonspace) == '>' {
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if c := p.peek(p.offset); c == ' ' || c == '\t' {
				p.advanceOffset(1, true)
			}
			return continueMatched
		}
		return continueFailed
	case kindListItem:
		if p.blank {
			if n.first == nil {
				// A list item can begin with at most one blank line.
				return continueFailed
			}
			p.advanceNextNonspace()
			return continueMatched
		}
		if p.indent >= int(n.markerOffset+n.padding) {
			p.advanceOffset(int(n.markerOffset+n.padding), true)
			return continueMatched
		}
		return continueFailed
	case kindFootnoteDef:
		if p.blank {
			// The whitespace stays for the code blocks in the definition.
			return continueMatched
		}
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return continueMatched
		}
		return continueFailed
	case kindHeading, kindThematicBreak:
		return continueFailed
	case kindCodeBlock:
		if n.fenced {
			if p.indent <= 3 && p.peek(p.nextNonspace) == n.fenceChar {
				i := p.nextNonspace
				for i < len(p.line) && p.line[i] == n.fenceChar {
					i++
				}
				if i-p.nextNonspace >= int(n.fenceLen) && isBlankBytes(p.line[i:]) {
					n.fenceClosed = true
					p.finalize(n)
					return continueDone
				}
			}
			for i := n.fenceOffset; i > 0; i-- {
				if c := p.peek(p.offset); c != ' ' && c != '\t' {
					break
				}
				p.advanceOffset(1, true)
			}
			return continueMatched
		}
		if p.indent >= 4 {
			p.advanceOffset(4, true)
			return continueMatched
		}
		if p.blank {
			p.advanceNextNonspace()
			return continueMatched
		}
		return continueFailed
	case kindHTMLBlock:
		if p.blank && (n.level == 6 || n.level == 7) {
			return continueFailed
		}
		return continueMatched
	case kindParagraph:
		if p.blank {
			return continueFailed
		}
		return continueMatched
	}
	return continueMatched
}

func (p *parser) incorporateLine() {
	p.offset, p.column = 0, 0
	p.blank = false
	p.partiallyConsumedTab = false
	p.prevBlankIn, p.curBlankIn = p.curBlankIn, nil
	p.emptiedParagraph = false
	p.oldtip = p.tip
	var marked *node

	container := p.doc
	for container.last != nil && container.last.open {
		container = container.last
		p.findNextNonspace()
		switch p.continueBlock(container) {
		case continueFailed:
			container = container.parent
			goto matched
		case continueDone:
			p.touch(container)
			return
		}
		if container.kind == kindBlockquote {
			marked = container
		}
	}
matched:
	if isBlankBytes(p.line[min(p.offset, len(p.line)):]) {
		// A blank line in a blockquote only separates the blocks in that
		// blockquote: the blockquote marker is on the line, so it is not a
		// blank line for the containers around it.
		p.curBlankIn = p.doc
		if marked != nil {
			p.curBlankIn = marked
		}
	}
	p.allClosed = container == p.oldtip
	p.lastMatchedContainer = container

	matchedLeaf := container.kind != kindParagraph && acceptsLines(container.kind)
	for !matchedLeaf {
		p.findNextNonspace()
		if p.indent < 4 && !maybeSpecial(p.peek(p.nextNonspace)) {
			p.advanceNextNonspace()
			break
		}
		res := p.tryBlockStarts(container)
		if res == startNone {
			p.advanceNextNonspace()
			break
		}
		container = p.tip
		if res == startLeaf {
			matchedLeaf = true
		}
	}

	if !p.allClosed && !p.blank && p.tip.kind == kindParagraph && !p.tip.tableRows {
		// Lazy paragraph continuation. Table rows cannot be lazy.
		p.addLine()
		p.segs[len(p.segs)-1].lazy = true
		p.touch(p.tip)
		return
	}

	p.closeUnmatchedBlocks()
	t := container.kind
	if p.blank {
		if marked != nil {
			p.touch(marked)
		}
		if (t == kindCodeBlock && container.fenced) || t == kindHTMLBlock {
			p.touch(container)
		}
	}

	switch {
	case t == kindCodeBlock && container.fenced && container.startLine == int32(p.lineNumber):
		// The opening fence is not content.
	case t == kindParagraph && container.tableRows && p.emptyTableRow(p.line[p.offset:]):
		// A row without cells ends the table, and starts a paragraph.
		p.finalize(container)
		p.advanceNextNonspace()
		p.addChild(kindParagraph)
		p.addLine()
	case acceptsLines(t):
		if t == kindParagraph && p.opts.Tables && !container.tableRows && container.segEnd-container.segStart >= 1 {
			container.tableRows = p.isDelimiterRow(p.segs[container.segEnd-1], p.line[p.offset:])
		}
		if p.blank && (t == kindCodeBlock && container.fenced || t == kindHTMLBlock) {
			// The line is content, not a blank line between blocks.
			p.curBlankIn = nil
		}
		p.addLine()
		if !p.blank {
			p.touch(container)
		}
		if t == kindHTMLBlock && container.level >= 1 && container.level <= 5 &&
			htmlBlockEnds(int(container.level), p.line[min(p.offset, len(p.line)):]) {
			container.fenceClosed = true
			p.finalize(container)
		}
	case p.offset < len(p.line) && !p.blank:
		para := p.addChild(kindParagraph)
		// A blank line counts only inside the containers of the paragraph.
		for a := para.parent; a != nil && p.prevBlankIn != nil; a = a.parent {
			if a == p.prevBlankIn {
				para.blankBefore = true
				break
			}
		}
		p.advanceNextNonspace()
		p.addLine()
	}
}

func (p *parser) closeUnmatchedBlocks() {
	if !p.allClosed {
		for p.oldtip != p.lastMatchedContainer {
			parent := p.oldtip.parent
			p.finalize(p.oldtip)
			p.oldtip = parent
		}
		p.allClosed = true
	}
}

func maybeSpecial(c byte) bool {
	switch c {
	case '>', '#', '`', '~', '<', '*', '+', '-', '_', '=', '[', '|', ':',
		'0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return true
	}
	return false
}

func isBlankBytes(b []byte) bool {
	for _, c := range b {
		if c != ' ' && c != '\t' {
			return false
		}
	}
	return true
}

const (
	startNone = iota
	startContainer
	startLeaf
)

func (p *parser) tryBlockStarts(container *node) int {
	indented := p.indent >= 4
	c := p.peek(p.nextNonspace)

	if !indented {
		switch c {
		case '>':
			if p.depth >= maxNesting {
				break
			}
			p.advanceNextNonspace()
			p.advanceOffset(1, false)
			if c := p.peek(p.offset); c == ' ' || c == '\t' {
				p.advanceOffset(1, true)
			}
			p.closeUnmatchedBlocks()
			b := p.addChild(kindBlockquote)
			if p.opts.Alerts && b.parent == p.doc && !p.partiallyConsumedTab {
				if t := alertType(p.line[p.offset:]); t > 0 {
					b.alert = uint8(t)
					b.markerOffset = int32(p.lineStart + p.offset)
				}
			}
			return startContainer
		case '#':
			if r := p.tryATXHeading(); r != startNone {
				return r
			}
		case '`', '~':
			if r := p.tryFencedCode(); r != startNone {
				return r
			}
		case '<':
			if r := p.tryHTMLBlock(container); r != startNone {
				return r
			}
		case '[':
			if p.opts.Footnotes && p.depth < maxNesting {
				if r := p.tryFootnoteDef(); r != startNone {
					return r
				}
			}
		}
		if (c == '=' || c == '-') && isParagraph(container) {
			if r := p.trySetextHeading(container); r != startNone {
				return r
			}
		}
		if c == '*' || c == '-' || c == '_' {
			if isThematicBreak(p.line[p.nextNonspace:]) {
				p.closeUnmatchedBlocks()
				p.addChild(kindThematicBreak)
				p.advanceOffset(len(p.line)-p.offset, false)
				return startLeaf
			}
		}
	}
	if (!indented || container.kind == kindList) && p.depth < maxNesting {
		if r := p.tryListItem(container); r != startNone {
			return r
		}
	}
	if indented && !isParagraph(p.tip) && !p.blank {
		p.advanceOffset(4, true)
		p.closeUnmatchedBlocks()
		p.addChild(kindCodeBlock)
		return startLeaf
	}
	return startNone
}

func (p *parser) tryATXHeading() int {
	i := p.nextNonspace
	level := 0
	for i < len(p.line) && p.line[i] == '#' {
		level++
		i++
	}
	if level > 6 || (i < len(p.line) && p.line[i] != ' ' && p.line[i] != '\t') {
		return startNone
	}
	p.advanceNextNonspace()
	p.advanceOffset(level, false)
	p.closeUnmatchedBlocks()
	h := p.addChild(kindHeading)
	h.level = int32(level)
	content := p.line[p.offset:]
	start := p.lineStart + p.offset
	content = bytes.TrimRight(content, " \t")
	end := len(content)
	j := end
	for j > 0 && content[j-1] == '#' {
		j--
	}
	if j < end && (j == 0 || content[j-1] == ' ' || content[j-1] == '\t') {
		end = j
	}
	content = bytes.TrimRight(content[:end], " \t")
	lead := len(content) - len(bytes.TrimLeft(content, " \t"))
	p.segs = append(p.segs, seg{start: int32(start + lead), end: int32(start + len(content)), eol: int32(p.lineEOL)})
	h.segEnd = int32(len(p.segs))
	p.advanceOffset(len(p.line)-p.offset, false)
	return startLeaf
}

func (p *parser) tryFencedCode() int {
	c := p.peek(p.nextNonspace)
	i := p.nextNonspace
	for i < len(p.line) && p.line[i] == c {
		i++
	}
	n := i - p.nextNonspace
	if n < 3 {
		return startNone
	}
	info := p.line[i:]
	if c == '`' && bytes.IndexByte(info, '`') >= 0 {
		return startNone
	}
	p.closeUnmatchedBlocks()
	b := p.addChild(kindCodeBlock)
	b.fenced = true
	b.fenceChar = c
	b.fenceLen = int32(n)
	b.fenceOffset = int32(p.indent)
	b.text = bytes.Trim(info, " \t")
	p.advanceNextNonspace()
	p.advanceOffset(len(p.line)-p.offset, false)
	return startLeaf
}

func isThematicBreak(b []byte) bool {
	var c byte
	n := 0
	for _, x := range b {
		switch {
		case x == ' ' || x == '\t':
		case n == 0 && (x == '*' || x == '-' || x == '_'):
			c = x
			n++
		case x == c:
			n++
		default:
			return false
		}
	}
	return n >= 3
}

func (p *parser) tryListItem(container *node) int {
	if p.indent >= 4 {
		return startNone
	}
	rest := p.line[p.nextNonspace:]
	if len(rest) == 0 {
		return startNone
	}
	var ordered bool
	var bullet, delim byte
	start := 0
	markerLen := 0
	c := rest[0]
	switch {
	case c == '*' || c == '+' || c == '-':
		bullet = c
		markerLen = 1
	case c >= '0' && c <= '9':
		i := 0
		for i < len(rest) && i < 10 && rest[i] >= '0' && rest[i] <= '9' {
			start = start*10 + int(rest[i]-'0')
			i++
		}
		if i > 9 || i >= len(rest) || (rest[i] != '.' && rest[i] != ')') {
			return startNone
		}
		if isParagraph(container) && start != 1 {
			return startNone
		}
		ordered = true
		delim = rest[i]
		markerLen = i + 1
	default:
		return startNone
	}
	if markerLen < len(rest) && rest[markerLen] != ' ' && rest[markerLen] != '\t' {
		return startNone
	}
	if isParagraph(container) && !p.emptiedParagraph && isBlankBytes(rest[markerLen:]) {
		return startNone
	}

	markerOffset := p.indent
	p.findNextNonspace()
	p.advanceNextNonspace()
	p.advanceOffset(markerLen, true)
	spacesStartCol, spacesStartOffset := p.column, p.offset
	for {
		p.advanceOffset(1, true)
		c := p.peek(p.offset)
		if !(p.column-spacesStartCol < 5 && (c == ' ' || c == '\t')) {
			break
		}
	}
	blankItem := p.offset >= len(p.line)
	spacesAfterMarker := p.column - spacesStartCol
	var padding int
	if spacesAfterMarker >= 5 || spacesAfterMarker < 1 || blankItem {
		padding = markerLen + 1
		p.column = spacesStartCol
		p.offset = spacesStartOffset
		p.partiallyConsumedTab = false
		if c := p.peek(p.offset); c == ' ' || c == '\t' {
			p.advanceOffset(1, true)
		}
	} else {
		padding = markerLen + spacesAfterMarker
	}

	p.closeUnmatchedBlocks()
	if p.tip.kind != kindList || p.tip.ordered != ordered || p.tip.bulletChar != bullet || p.tip.delimiter != delim {
		l := p.addChild(kindList)
		l.ordered = ordered
		l.bulletChar = bullet
		l.delimiter = delim
		l.listStart = int32(start)
		l.tight = true
	}
	item := p.addChild(kindListItem)
	item.markerOffset = int32(markerOffset)
	item.padding = int32(padding)
	return startContainer
}

func (p *parser) finalize(b *node) {
	parent := b.parent
	b.open = false
	switch b.kind {
	case kindBlockquote:
		p.depth--
		if b.alert > 0 {
			p.finalizeAlert(b)
		}
	case kindListItem:
		p.depth--
	case kindFootnoteDef:
		p.depth--
		p.finalizeFootnoteDef(b)
	case kindParagraph:
		p.finalizeParagraph(b)
	case kindCodeBlock:
		p.finalizeCodeBlock(b)
	case kindList:
		p.finalizeList(b)
	case kindHTMLBlock:
		p.hasRawHTML = true
	}
	p.tip = parent
}

func (p *parser) finalizeCodeBlock(b *node) {
	if b.fenced {
		return
	}
	for b.segEnd > b.segStart {
		s := p.segs[b.segEnd-1]
		if !isBlankBytes(p.src[s.start:s.end]) {
			break
		}
		b.segEnd--
	}
}

// finalizeList decides whether the list is loose: its items are separated by
// blank lines, or an item directly contains two blocks with a blank line
// between them.
func (p *parser) finalizeList(l *node) {
	l.tight = !p.blankBetweenChildren(l)
	for item := l.first; item != nil && l.tight; item = item.next {
		l.tight = !p.blankBetweenChildren(item)
	}
}

// blankBetweenChildren reports whether a blank line separates two children
// of b. Link reference definitions are blocks too (CommonMark 0.31.2,
// section 4.7), so their kindRefDefs nodes count.
func (p *parser) blankBetweenChildren(b *node) bool {
	var prev *node
	prevLast := 0
	for c := b.first; c != nil; c = c.next {
		if c.kind == kindFootnoteDef {
			// Blank lines before a footnote definition do not count, and its
			// lines count as the lines of the previous block.
			prevLast = max(prevLast, int(c.lastLine))
			continue
		}
		if prev != nil && (int(c.startLine) > prevLast+1 && !c.noBlankBefore || c.blankBefore) {
			return true
		}
		prev, prevLast = c, int(c.lastLine)
	}
	return false
}
