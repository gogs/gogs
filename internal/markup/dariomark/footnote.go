package dariomark

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
}

func (s *footnoteSet) reset() {
	s.inlined = false
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
	s.defs = append(s.defs, d)
	if s.byLabel == nil {
		s.byLabel = make(map[string]*footnote)
	}
	if key := footnoteKey(d.text); s.byLabel[key] == nil {
		s.byLabel[key] = &footnote{def: d}
	}
}

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
// at "[" or at "!" followed by "[".
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
	ref.footnote = fn
	ref.refIndex = fn.refs
	fn.refs++
	if ip.pos == ip.lineStart && ip.textStart == i {
		ip.lineStart = end
	}
	ip.flushText(i)
	ip.parent.appendChild(ref)
	ip.pos, ip.textStart, ip.boundary = end, end, end
	return true
}

// footnote returns the footnote with the label, or nil.
func (p *parser) footnote(label []byte) *footnote {
	return p.footnotes.byLabel[footnoteKey(label)]
}

// detachFootnoteDefs removes the footnote definitions from the tree after
// their contents have been parsed.
func (p *parser) detachFootnoteDefs() {
	if p.footnotes.nestedFirst() {
		p.footnotes.defs[0].insertBefore(p.arena.new(kindFootnoteHolder))
	}
	for _, d := range p.footnotes.defs {
		d.unlink()
	}
}

// nestedFirst reports whether the first definition to close is nested in
// another definition.
func (s *footnoteSet) nestedFirst() bool {
	if len(s.defs) == 0 {
		return false
	}
	for a := s.defs[0].parent; a != nil; a = a.parent {
		if a.kind == kindFootnoteDef {
			return true
		}
	}
	return false
}
