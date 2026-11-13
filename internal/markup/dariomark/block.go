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
	// wsStart is the offset of the whitespace skipped by the last
	// advanceNextNonspace on this line, after a partially consumed tab, whose
	// remaining columns are wsPad, up to wsEnd. It is -1 when nothing was
	// skipped.
	wsStart, wsPad, wsEnd int
	// curBlankIn is the container in which the current line counts as blank,
	// or nil. prevBlankIn is the same for the previous line.
	prevBlankIn, curBlankIn *node
	// tabOrigin is the column that tab stops are relative to.
	tabOrigin int
	// rawTabItem is the last list item started on the current line, if its
	// marker does not directly follow a tab.
	rawTabItem *node
	// lineTab reports whether a tab precedes the content of the current line
	// after its containers. See tabIndent.
	lineTab bool
	// emptiedParagraph reports whether a table took all lines of the
	// paragraph that the current line would underline, which lets the line
	// start an empty list item.
	emptiedParagraph bool
	depth            int

	refs      map[string]linkRef
	footnotes footnoteSet

	hasRawHTML bool
	// refBudget is the number of bytes of reference destinations and titles
	// that the rest of the document may expand, see spendRefBudget.
	refBudget int
	// padBudget is the number of empty cells that the rest of the document
	// may add to fill short table rows.
	padBudget int

	inl inlineParser
}

func (p *parser) reset() {
	p.arena.reset()
	p.segs = p.segs[:0]
	p.buf = p.buf[:0]
	p.src = nil
	p.opts = nil
	p.doc, p.tip, p.oldtip, p.lastMatchedContainer = nil, nil, nil, nil
	p.line = nil
	p.depth = 0
	clear(p.refs)
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

func (p *parser) parse(src []byte) *node {
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
	}
	for p.tip != nil {
		p.finalize(p.tip)
	}
	p.processInlines(p.doc)
	if p.opts.RelativeLinks || p.opts.TextAutoLinksInRawAnchors {
		p.postInline(p.doc)
	}
	if p.opts.Footnotes {
		p.detachFootnoteDefs()
	}
	return p.doc
}

// tabWidth returns the number of columns of a tab at column col.
func (p *parser) tabWidth(col int) int {
	return 4 - (col-p.tabOrigin)%4
}

func (p *parser) findNextNonspace() {
	i, cols := p.offset, p.column
scan:
	for i < len(p.line) {
		switch p.line[i] {
		case ' ':
			cols++
		case '\t':
			cols += p.tabWidth(cols)
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

// tabIndent reports whether a whole tab precedes the next nonspace character.
// Inside containers, such a tab may leave fewer than four columns of
// indentation, and it prevents list items, empty ATX headings after a wide
// tab, setext headings, HTML blocks and table delimiter rows.
func (p *parser) tabIndent() bool {
	i := p.offset
	if p.partiallyConsumedTab {
		i++
	}
	return i < p.nextNonspace && bytes.IndexByte(p.line[i:p.nextNonspace], '\t') >= 0
}

// wideTab reports whether a whole tab of at least two columns precedes the
// next nonspace character.
func (p *parser) wideTab() bool {
	i, col := p.offset, p.column
	if p.partiallyConsumedTab {
		col += p.tabWidth(col)
		i++
	}
	for ; i < p.nextNonspace; i++ {
		if p.line[i] == ' ' {
			col++
			continue
		}
		w := p.tabWidth(col)
		if w >= 2 {
			return true
		}
		col += w
	}
	return false
}

func (p *parser) advanceNextNonspace() {
	if p.wsStart >= 0 && p.offset == p.nextNonspace && p.offset == p.wsEnd {
		// Nothing moved since the last call, keep what it skipped.
		return
	}
	p.wsStart, p.wsPad, p.wsEnd = p.offset, 0, p.nextNonspace
	if p.partiallyConsumedTab {
		p.wsStart++
		p.wsPad = p.tabWidth(p.column)
	}
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
			toTab := p.tabWidth(p.column)
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
	n.startLine = p.lineNumber
	n.segStart = len(p.segs)
	n.segEnd = len(p.segs)
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
	for ; b != nil && b.lastLine < p.lineNumber; b = b.parent {
		b.lastLine = p.lineNumber
	}
}

// addLine adds the rest of the current line to the tip.
func (p *parser) addLine() {
	s := seg{start: int32(p.lineStart + p.offset), end: int32(p.lineStart + len(p.line)), eol: int32(p.lineEOL)}
	if p.partiallyConsumedTab {
		s.start++
		s.pad = int8(p.tabWidth(p.column))
	}
	if s.start > s.end {
		s.start = s.end
	}
	s.indent = int8(min(p.indent, 127))
	s.tab = p.lineTab
	s.ws = s.start
	if p.wsStart >= 0 && p.wsStart <= p.offset {
		s.ws, s.wsPad = int32(p.lineStart+p.wsStart), int8(p.wsPad)
	}
	p.segs = append(p.segs, s)
	p.tip.segEnd = len(p.segs)
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
			if endsWithEmptyItem(n) {
				return continueFailed
			}
			p.advanceNextNonspace()
			return continueMatched
		}
		if p.indent >= n.markerOffset+n.padding {
			if n.first == nil && p.indent < 4 && p.endsEmptyItem(n, p.line[p.nextNonspace:]) {
				return continueFailed
			}
			p.advanceOffset(n.markerOffset+n.padding, true)
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
				if i-p.nextNonspace >= n.fenceLen && isBlankBytes(p.line[i:]) {
					n.fenceClosed = true
					p.finalize(n)
					return continueDone
				}
			}
			if p.blank && p.indent < n.fenceOffset {
				// A blank line with less indentation than the fence keeps
				// its whitespace.
				return continueMatched
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
	p.tabOrigin = 0
	p.rawTabItem = nil
	p.blank = false
	p.partiallyConsumedTab = false
	p.wsStart, p.wsPad = -1, 0
	p.prevBlankIn, p.curBlankIn = p.curBlankIn, nil
	p.lineTab = false
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
			// goldmark treats the line that closes a fenced code block of
			// the document as blank.
			if container.parent == p.doc {
				p.curBlankIn = p.doc
			}
			return
		}
		if container.kind == kindBlockquote {
			marked = container
		}
	}
matched:
	if isBlankBytes(p.line[min(p.offset, len(p.line)):]) {
		// goldmark only counts a blank line in a blockquote for the blocks
		// in that blockquote.
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
		p.lineTab = p.tabIndent()
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

	if !p.allClosed && !p.blank && p.tip.kind == kindParagraph {
		// Lazy paragraph continuation.
		p.addLine()
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
	case t == kindCodeBlock && container.fenced && container.startLine == p.lineNumber:
		// The opening fence is not content.
	case acceptsLines(t):
		p.addLine()
		if !p.blank {
			p.touch(container)
		}
		if t == kindHTMLBlock && container.level >= 1 && container.level <= 5 &&
			htmlBlockEnds(container.level, p.line[min(p.offset, len(p.line)):]) {
			// Like the closing line of a fenced code block.
			if container.parent == p.doc && container.startLine < p.lineNumber {
				p.curBlankIn = p.doc
			}
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

	tab := p.lineTab
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
					b.alert = t
					b.markerOffset = p.lineStart + p.offset
				}
			}
			return startContainer
		case '#':
			if tab && p.wideTab() && p.lineEOL == p.lineStart+len(p.line) && string(p.line[p.nextNonspace:]) == "#" {
				// A lone "#" after a wide tab on the last line, without a line
				// ending, is paragraph text.
				break
			}
			if r := p.tryATXHeading(); r != startNone {
				return r
			}
		case '`', '~':
			if r := p.tryFencedCode(); r != startNone {
				return r
			}
		case '<':
			if tab {
				break
			}
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
		if (c == '=' || c == '-') && container.kind == kindParagraph && !tab {
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
	if (!indented || container.kind == kindList) && p.depth < maxNesting && !tab {
		if r := p.tryListItem(container); r != startNone {
			return r
		}
	}
	if indented && p.tip.kind != kindParagraph && !p.blank {
		p.advanceOffset(4, true)
		if p.rawTabItem != nil && p.rawTabItem == container && p.tabOrigin%4 == 1 {
			// goldmark keeps the whole tab instead of its remaining columns
			// when the item starts one column after a tab stop, e.g.,
			// ">-\t\tx".
			p.partiallyConsumedTab = false
		}
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
	h.level = level
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
	h.segEnd = len(p.segs)
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
	b.fenceLen = n
	// goldmark counts the indentation of a fence in bytes, except for the
	// rest of a partially consumed tab.
	b.fenceOffset = p.nextNonspace - p.offset
	if p.partiallyConsumedTab {
		b.fenceOffset += p.tabWidth(p.column) - 1
	}
	b.text = bytes.Trim(info, " \t")
	if len(info) == 1 && p.lineStart+len(p.line) == len(p.src) {
		// A one-byte info string directly after the fence on the last line
		// of the input, without a line ending, is ignored, e.g., "```a".
		b.text = nil
	}
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
		if container.kind == kindParagraph && start != 1 {
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
	if container.kind == kindParagraph && !p.emptiedParagraph && isBlankBytes(rest[markerLen:]) {
		return startNone
	}

	markerOffset := p.indent
	// Tab stops after a list marker are relative to the start of the
	// container content.
	origin := p.column
	p.findNextNonspace()
	p.advanceNextNonspace()
	markerStart := p.offset
	p.tabOrigin = origin
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
		c := p.peek(p.offset)
		if c == ' ' {
			// goldmark measures the indented code that follows a space with
			// absolute tab stops.
			p.tabOrigin = 0
		}
		if c == ' ' || c == '\t' {
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
		l.listStart = start
		l.tight = true
	}
	item := p.addChild(kindListItem)
	item.markerOffset = markerOffset
	item.padding = padding
	p.rawTabItem = nil
	if markerStart == 0 || p.line[markerStart-1] != '\t' {
		p.rawTabItem = item
	}
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

// endsEmptyItem reports whether line, which is indented into the empty list
// item, ends the item. goldmark ends it at a list item marker of another type
// than the list, or at a thematic break that starts like a list item marker,
// e.g., "* * *".
func (p *parser) endsEmptyItem(item *node, line []byte) bool {
	i := 0
	var bullet, delim byte
	switch {
	case len(line) > 0 && (line[0] == '-' || line[0] == '*' || line[0] == '+'):
		bullet = line[0]
		i = 1
	default:
		for i < len(line) && i < 9 && line[i] >= '0' && line[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(line) || line[i] != '.' && line[i] != ')' {
			return false
		}
		delim = line[i]
		i++
	}
	if i < len(line) && line[i] != ' ' && line[i] != '\t' {
		return false
	}
	l := item.parent
	return bullet != l.bulletChar || delim != l.delimiter || isThematicBreak(line)
}

// endsWithEmptyItem reports whether the list item is empty, or its last
// child is a list whose last item ends with an empty item. Such an item ends
// at a blank line.
func endsWithEmptyItem(item *node) bool {
	for item.first != nil {
		if item.last.kind != kindList || item.last.last == nil {
			return false
		}
		item = item.last.last
	}
	return true
}

func (p *parser) blankBetweenChildren(b *node) bool {
	var prev *node
	prevLast := 0
	for c := b.first; c != nil; c = c.next {
		if c.kind == kindFootnoteDef {
			// Blank lines before a footnote definition do not count, and its
			// lines count as the lines of the previous block.
			prevLast = max(prevLast, c.lastLine)
			continue
		}
		if prev != nil && (c.startLine > prevLast+1 && !c.noBlankBefore || c.blankBefore || b.kind == kindList && p.endsWithOpenFenceBlank(prev)) {
			return true
		}
		prev, prevLast = c, c.lastLine
	}
	return false
}

// endsWithOpenFenceBlank reports whether item ends with a fenced code block,
// or an HTML block that ends at an end condition, that did not end and whose
// last line is blank. Such an item counts as followed by a blank line.
func (p *parser) endsWithOpenFenceBlank(item *node) bool {
	c := item.last
	if c == nil || c.fenceClosed || c.segEnd == c.segStart ||
		!(c.kind == kindCodeBlock && c.fenced || c.kind == kindHTMLBlock && c.level <= 5) {
		return false
	}
	s := p.segs[c.segEnd-1]
	return isBlankBytes(p.src[s.start:s.end])
}
