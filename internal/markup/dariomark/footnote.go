package dariomark

import (
	"bytes"
)

type footnote struct {
	def    *node
	number int // 0 until referenced
	refs   int
}

type footnoteSet struct {
	byLabel map[string]*footnote
	ordered []*footnote
	defs    []*node
	inlined bool
	// firstLine is the first line of the first definition, see
	// parser.parseLeaf, and seen whether there is one.
	firstLine int
	seen      bool
	// leaves are the leaf blocks of definitions whose inline contents are
	// kept, see parser.parseFootnoteDefs.
	leaves []*node
}

func (s *footnoteSet) reset() {
	s.inlined = false
	s.seen = false
	clear(s.leaves)
	s.leaves = s.leaves[:0]
	clear(s.byLabel)
	clear(s.ordered)
	s.ordered = s.ordered[:0]
	clear(s.defs)
	s.defs = s.defs[:0]
}

// scanFootnoteLabel scans "[^label]" at b[i] and returns the label and the
// offset after the closing bracket, or -1.
func scanFootnoteLabel(b []byte, i int) ([]byte, int) {
	if i+3 >= len(b) || b[i] != '[' || b[i+1] != '^' {
		return nil, -1
	}
	start := i + 2
	for j := start; j < len(b) && j-start < 1000; j++ {
		switch b[j] {
		case '\\':
			if j+1 < len(b) && b[j+1] != '\n' && b[j+1] != '\r' {
				j++
			}
		case ']':
			if j == start {
				return nil, -1
			}
			return b[start:j], j + 1
		case '\n', '\r':
			return nil, -1
		}
	}
	return nil, -1
}

func (p *parser) tryFootnoteDef() int {
	label, end := scanFootnoteLabel(p.line, p.nextNonspace)
	if end < 0 || end >= len(p.line) || p.line[end] != ':' {
		return startNone
	}
	p.advanceNextNonspace()
	p.advanceOffset(end+1-p.offset, false)
	p.closeUnmatchedBlocks()
	d := p.addChild(kindFootnoteDef)
	d.text = label
	return startContainer
}

func (p *parser) finalizeFootnoteDef(d *node) {
	s := &p.footnotes
	if !s.seen || int(d.startLine) < s.firstLine {
		s.firstLine = int(d.startLine)
	}
	s.seen = true
	if s.byLabel == nil {
		s.byLabel = make(map[string]*footnote)
	}
	key := footnoteKey(d.text)
	if s.byLabel[key] != nil && d.parent.kind != kindListItem && !p.mayContainRefs(d) {
		// Only the first definition of a label is rendered, and parsing the
		// contents of a later one only matters for the references in it.
		// In list items, definitions still count for blankBetweenChildren.
		p.dropBlock(d)
		return
	}
	s.defs = append(s.defs, d)
	if s.byLabel[key] == nil {
		s.byLabel[key] = &footnote{def: d}
	}
}

// mayContainRefs reports whether the lines of the block b, which is the
// last block that has lines, contain a "[", or b contains a footnote
// definition.
func (p *parser) mayContainRefs(b *node) bool {
	if containsKind(b, kindFootnoteDef) {
		return true
	}
	for _, s := range p.segs[b.segStart:] {
		if bytes.IndexByte(p.src[s.start:s.end], '[') >= 0 {
			return true
		}
	}
	return false
}

func containsKind(b *node, k kind) bool {
	for c := b.first; c != nil; c = c.next {
		if c.kind == k || containsKind(c, k) {
			return true
		}
	}
	return false
}

// dropBlock removes the closed block b from the tree and frees it.
func (p *parser) dropBlock(b *node) {
	for _, blankIn := range []**node{&p.prevBlankIn, &p.curBlankIn} {
		for a := *blankIn; a != nil; a = a.parent {
			if a == b {
				// The freed node may be reused for an open container, which
				// must not match.
				*blankIn = &droppedBlock
				break
			}
		}
	}
	b.unlink()
	p.arena.freeTree(b)
}

// droppedBlock stands for a block that dropBlock freed.
var droppedBlock node

// footnoteKey returns the key of a footnote label, which is normalized like
// the label of a link reference definition. Labels of only whitespace stay
// as they are.
func footnoteKey(label []byte) string {
	if key := normalizeLabel(label); key != "" {
		return key
	}
	return string(label)
}

// footnoteRef parses a footnote reference at the current position, which is
// at "[" or at "!" followed by "[", and reports whether there is one.
func (ip *inlineParser) footnoteRef() bool {
	src := ip.src
	i := ip.pos
	if src[i] == '!' {
		i++
	}
	label, end := scanFootnoteLabel(src, i)
	if end < 0 {
		return false
	}
	fn := ip.p.footnote(label)
	if fn == nil {
		return false
	}
	if fn.number == 0 {
		ip.p.footnotes.ordered = append(ip.p.footnotes.ordered, fn)
		fn.number = len(ip.p.footnotes.ordered)
	}
	ref := ip.p.arena.new(kindFootnoteRef)
	e := ip.p.arena.newExt(ref)
	e.footnote = fn
	e.refIndex = fn.refs
	fn.refs++
	ip.flushText(i)
	ip.parent.appendChild(ref)
	ip.pos, ip.textStart, ip.boundary = end, end, end
	return true
}

// resetInline undoes the effects of parsing inline contents on the
// footnotes, so that the document can be rendered again.
func (s *footnoteSet) resetInline() {
	for _, fn := range s.ordered {
		fn.number, fn.refs = 0, 0
	}
	clear(s.ordered)
	s.ordered = s.ordered[:0]
	for _, n := range s.leaves {
		n.first, n.last, n.parsed, n.persistent = nil, nil, false, false
	}
	clear(s.leaves)
	s.leaves = s.leaves[:0]
	s.inlined = false
}

// footnote returns the footnote with the label, or nil.
func (p *parser) footnote(label []byte) *footnote {
	return p.footnotes.byLabel[footnoteKey(label)]
}

// detachFootnoteDefs removes the footnote definitions from the tree after
// their contents have been parsed.
func (p *parser) detachFootnoteDefs() {
	for _, d := range p.footnotes.defs {
		d.unlink()
	}
}
