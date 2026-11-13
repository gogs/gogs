package dariomark

type kind uint8

const (
	kindDocument kind = iota
	kindParagraph
	kindHeading
	kindThematicBreak
	kindCodeBlock
	kindHTMLBlock
	kindBlockquote
	kindList
	kindListItem
	kindTable
	kindTableRow
	kindTableCell
	kindFootnoteDef
	// kindRefDefs is a paragraph that only contained link reference
	// definitions. It is not rendered.
	kindRefDefs

	kindText
	kindSoftBreak
	kindHardBreak
	kindCodeSpan
	kindEmphasis
	kindStrong
	kindStrikethrough
	kindLink
	kindImage
	kindAutoLink
	kindRawHTML
	kindFootnoteRef
	kindTaskCheckbox
	// kindTypo is typographic punctuation. Its level holds the typo.
	kindTypo
	// kindFraction is a fraction. Its text holds the source, and its level the
	// length of the numerator, or 0 for a vulgar fraction like "½".
	kindFraction
)

type autoLinkType uint8

const (
	autoLinkURL autoLinkType = iota
	autoLinkEmail
	autoLinkWWW
)

type alignment uint8

const (
	alignNone alignment = iota
	alignLeft
	alignCenter
	alignRight
)

// seg is a line of a leaf block. pad is the number of spaces left over from a
// partially consumed tab, which precede src[start:end].
type seg struct {
	start, end int32 // content, without the line ending
	eol        int32 // end of the line, including the line ending
	pad        int8
	indent     int8 // columns of indentation removed from paragraph lines
	// lazy reports whether the line is a lazy continuation line.
	lazy bool
}

type node struct {
	parent, first, last, prev, next *node

	// text holds the literal content of inline nodes, the info string of code
	// blocks, and the label of autolinks.
	text []byte
	// ext holds the fields of links, images, autolinks, tables and
	// footnotes, which few nodes need.
	ext *nodeExt

	startLine int32
	lastLine  int32
	// segStart and segEnd index the lines of leaf blocks in parser.segs.
	segStart, segEnd int32

	level int32 // heading level, HTML block type, number of emphasis delimiters

	// Code blocks
	fenceLen    int32
	fenceOffset int32

	// Lists and list items
	listStart int32
	// markerOffset is the indentation of a list item marker, or the source
	// offset of the marker line of an alert.
	markerOffset int32
	padding      int32

	kind kind
	open bool
	// parsed reports whether the inline contents of a leaf block are
	// parsed, and persistent whether they are kept after rendering it.
	parsed, persistent bool

	fenced    bool
	fenceChar byte
	// fenceClosed reports whether a fenced code block, or an HTML block of
	// types 1 to 5, ended at its end condition.
	fenceClosed bool

	ordered    bool
	bulletChar byte
	delimiter  byte
	tight      bool

	align  alignment
	header bool
	// tableRows reports whether a paragraph has a table delimiter row, which
	// makes its next lines table rows.
	tableRows bool
	// noBlankBefore reports whether a blank line before a table that took
	// all lines of a paragraph is ignored when deciding whether a list is
	// loose.
	noBlankBefore bool
	// blankBefore reports whether a blank line, or the start of the input,
	// precedes a paragraph.
	blankBefore bool

	autoLink autoLinkType
	// angle reports whether an autolink is enclosed in angle brackets.
	angle bool
	// plain reports whether an autolink is rendered as text.
	plain  bool
	setext bool
	// textEOL reports whether a trailing space or tab was removed from text,
	// which still counts for the typographer.
	textEOL bool
	checked bool
	// alert is the alertKind of a blockquote, or 0.
	alert uint8
}

type nodeExt struct {
	// dest and title hold link, image and autolink destinations. rawDest is
	// the destination of links before escapes and entities are resolved.
	dest, rawDest, title []byte
	hasTitle             bool

	// aligns are the column alignments of a table.
	aligns []alignment

	footnote *footnote
	refIndex int

	// next links freed records, see arena.freeTree.
	next *nodeExt
}

func (n *node) appendChild(c *node) {
	c.parent = n
	c.next = nil
	c.prev = n.last
	if n.last != nil {
		n.last.next = c
	} else {
		n.first = c
	}
	n.last = c
}

func (n *node) insertAfter(c *node) {
	c.parent = n.parent
	c.prev = n
	c.next = n.next
	if n.next != nil {
		n.next.prev = c
	} else if n.parent != nil {
		n.parent.last = c
	}
	n.next = c
}

func (n *node) insertBefore(c *node) {
	c.parent = n.parent
	c.next = n
	c.prev = n.prev
	if n.prev != nil {
		n.prev.next = c
	} else if n.parent != nil {
		n.parent.first = c
	}
	n.prev = c
}

func (n *node) unlink() {
	if n.prev != nil {
		n.prev.next = n.next
	} else if n.parent != nil {
		n.parent.first = n.next
	}
	if n.next != nil {
		n.next.prev = n.prev
	} else if n.parent != nil {
		n.parent.last = n.prev
	}
	n.parent, n.prev, n.next = nil, nil, nil
}

const arenaChunk = 256

// arena allocates nodes and their ext records in chunks that are reused
// across renders.
type arena struct {
	chunks [][]node
	chunk  int
	used   int
	// free holds nodes freed by freeTree, linked by next.
	free *node
	// allocs counts the nodes allocated.
	allocs int

	exts     [][]nodeExt
	extChunk int
	extUsed  int
	freeExt  *nodeExt
}

type arenaMark struct {
	chunk, used, extChunk, extUsed int
	free                           *node
	freeExt                        *nodeExt
}

// mark returns a mark to pass to release. Until then, nodes are not taken
// from the nodes freed before, which release makes available again.
func (a *arena) mark() arenaMark {
	m := arenaMark{a.chunk, a.used, a.extChunk, a.extUsed, a.free, a.freeExt}
	a.free, a.freeExt = nil, nil
	return m
}

// release frees the nodes and ext records allocated since m. Nodes freed by
// freeTree since m are among them.
func (a *arena) release(m arenaMark) {
	a.free = m.free
	a.freeExt = m.freeExt
	for i := m.chunk; i <= a.chunk && i < len(a.chunks); i++ {
		start, end := 0, arenaChunk
		if i == m.chunk {
			start = m.used
		}
		if i == a.chunk {
			end = a.used
		}
		clear(a.chunks[i][start:end])
	}
	a.chunk, a.used = m.chunk, m.used
	for i := m.extChunk; i <= a.extChunk && i < len(a.exts); i++ {
		start, end := 0, arenaChunk
		if i == m.extChunk {
			start = m.extUsed
		}
		if i == a.extChunk {
			end = a.extUsed
		}
		clear(a.exts[i][start:end])
	}
	a.extChunk, a.extUsed = m.extChunk, m.extUsed
}

func (a *arena) new(k kind) *node {
	a.allocs++
	if n := a.free; n != nil {
		a.free = n.next
		*n = node{kind: k}
		return n
	}
	if a.chunk >= len(a.chunks) {
		a.chunks = append(a.chunks, make([]node, arenaChunk))
	}
	n := &a.chunks[a.chunk][a.used]
	a.used++
	if a.used == arenaChunk {
		a.chunk++
		a.used = 0
	}
	*n = node{kind: k}
	return n
}

// newExt gives n an ext record.
func (a *arena) newExt(n *node) *nodeExt {
	if e := a.freeExt; e != nil {
		a.freeExt = e.next
		*e = nodeExt{}
		n.ext = e
		return e
	}
	if a.extChunk >= len(a.exts) {
		a.exts = append(a.exts, make([]nodeExt, arenaChunk))
	}
	e := &a.exts[a.extChunk][a.extUsed]
	a.extUsed++
	if a.extUsed == arenaChunk {
		a.extChunk++
		a.extUsed = 0
	}
	*e = nodeExt{}
	n.ext = e
	return e
}

// maxPooledChunks is the number of chunks of each kind a pooled arena or
// stack keeps, about 1 MB, so that rendering a huge document does not keep
// its memory alive.
const maxPooledChunks = 32

func (a *arena) reset() {
	// Drop references held by used nodes so that pooled arenas do not keep
	// rendered sources alive.
	for i := 0; i <= a.chunk && i < len(a.chunks); i++ {
		end := arenaChunk
		if i == a.chunk {
			end = a.used
		}
		clear(a.chunks[i][:end])
	}
	for i := 0; i <= a.extChunk && i < len(a.exts); i++ {
		end := arenaChunk
		if i == a.extChunk {
			end = a.extUsed
		}
		clear(a.exts[i][:end])
	}
	if len(a.chunks) > maxPooledChunks {
		clear(a.chunks[maxPooledChunks:])
		a.chunks = a.chunks[:maxPooledChunks:maxPooledChunks]
	}
	if len(a.exts) > maxPooledChunks {
		clear(a.exts[maxPooledChunks:])
		a.exts = a.exts[:maxPooledChunks:maxPooledChunks]
	}
	a.chunk, a.used, a.extChunk, a.extUsed = 0, 0, 0, 0
	a.free, a.freeExt = nil, nil
}

// freeTree frees n and its descendants for reuse by new.
func (a *arena) freeTree(n *node) {
	for c := n.first; c != nil; {
		next := c.next
		a.freeTree(c)
		c = next
	}
	if e := n.ext; e != nil {
		*e = nodeExt{next: a.freeExt}
		a.freeExt = e
	}
	*n = node{next: a.free}
	a.free = n
}

// stackChunk is the number of entries in a chunk of a stack.
const stackChunk = 1024

// stack is a stack that grows in chunks, so that growing it neither copies
// its entries nor leaves garbage behind.
type stack[T any] struct {
	chunks [][]T
	n      int
}

func (s *stack[T]) len() int {
	return s.n
}

func (s *stack[T]) push(v T) {
	c := s.n / stackChunk
	if c == len(s.chunks) {
		s.chunks = append(s.chunks, make([]T, stackChunk))
	}
	s.chunks[c][s.n%stackChunk] = v
	s.n++
}

func (s *stack[T]) at(i int) *T {
	return &s.chunks[i/stackChunk][i%stackChunk]
}

// truncate removes the entries from the index n on.
func (s *stack[T]) truncate(n int) {
	s.n = n
}

// reset empties s and drops the chunks beyond those that a pooled parser
// keeps.
func (s *stack[T]) reset() {
	s.n = 0
	if len(s.chunks) > maxPooledChunks {
		clear(s.chunks[maxPooledChunks:])
		s.chunks = s.chunks[:maxPooledChunks:maxPooledChunks]
	}
}
