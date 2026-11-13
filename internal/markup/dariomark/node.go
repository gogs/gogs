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
	// kindFootnoteHolder takes the place of a footnote definition nested in
	// another one when it was the first to close. It is not rendered.
	kindFootnoteHolder

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
	// kindTypo is typographic punctuation; level holds its typo.
	kindTypo
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
	tab        bool // a tab precedes the content, see parser.tabIndent
	// ws is where the whitespace before the content of a paragraph line
	// starts, after the containers and after a partially consumed tab, whose
	// remaining columns are wsPad.
	ws    int32
	wsPad int8
}

type node struct {
	kind                            kind
	parent, first, last, prev, next *node

	open      bool
	startLine int
	lastLine  int

	// text holds the literal content of inline nodes, the info string of code
	// blocks, and the label of autolinks.
	text []byte
	// dest and title hold link, image and autolink destinations. rawDest is
	// the destination of links before escapes and entities are resolved.
	dest, rawDest, title []byte
	hasTitle             bool

	// segStart and segEnd index the lines of leaf blocks in parser.segs.
	segStart, segEnd int

	level int // heading level, HTML block type, number of emphasis delimiters

	// Code blocks
	fenced      bool
	fenceChar   byte
	fenceLen    int
	fenceOffset int
	// fenceClosed reports whether a fenced code block, or an HTML block of
	// types 1 to 5, ended at its end condition.
	fenceClosed bool

	// Lists and list items
	ordered    bool
	bulletChar byte
	delimiter  byte
	listStart  int
	tight      bool
	// markerOffset is the indentation of a list item marker, or the source
	// offset of the marker line of an alert.
	markerOffset int
	padding      int

	// Tables
	aligns []alignment
	align  alignment
	header bool
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
	alert int

	// Footnotes
	footnote *footnote
	refIndex int
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

// arena allocates nodes in chunks that are reused across renders.
type arena struct {
	chunks [][]node
	chunk  int
	used   int
}

func (a *arena) new(k kind) *node {
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
	a.chunk, a.used = 0, 0
}
