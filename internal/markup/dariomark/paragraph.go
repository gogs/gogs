package dariomark

import (
	"bytes"
	"unicode/utf8"
)

// joinSegs returns the contents of the lines, joined by their line endings.
func (p *parser) joinSegs(segs []seg) []byte {
	if len(segs) == 0 {
		return nil
	}
	contiguous := segs[0].pad == 0
	for i := 1; i < len(segs) && contiguous; i++ {
		contiguous = segs[i].start == segs[i-1].eol && segs[i].pad == 0 && segs[i-1].end <= segs[i-1].eol
	}
	if contiguous {
		return p.src[segs[0].start:segs[len(segs)-1].end]
	}
	start := len(p.buf)
	for i, s := range segs {
		if i > 0 {
			prev := segs[i-1]
			p.buf = append(p.buf, p.src[prev.end:prev.eol]...)
		}
		p.buf = append(p.buf, p.src[s.start:s.end]...)
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}

// joinSegsIndented is joinSegs with the indentation of the lines after the
// first.
func (p *parser) joinSegsIndented(segs []seg) []byte {
	start := len(p.buf)
	for i, s := range segs {
		if i > 0 {
			prev := segs[i-1]
			p.buf = append(p.buf, p.src[prev.end:prev.eol]...)
			// goldmark counts the rest of a partially consumed tab twice.
			p.buf = append(p.buf, spaces[:2*s.wsPad]...)
			p.buf = append(p.buf, p.src[min(s.ws, s.start):s.start]...)
		}
		p.buf = append(p.buf, p.src[s.start:s.end]...)
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}

// paragraphLines strips link reference definitions from the start of the
// lines and returns the remaining lines. With indented, titles keep the
// indentation of their lines.
func (p *parser) paragraphLines(b *node, indented bool) []seg {
	segs := p.segs[b.segStart:b.segEnd]
	if len(segs) == 0 || p.src[segs[0].start] != '[' {
		return segs
	}
	content := p.joinSegs(segs)
	if indented {
		content = p.joinSegsIndented(segs)
	}
	consumed, lines := 0, 0
	for {
		i := consumed
		for i < len(content) && (content[i] == ' ' || content[i] == '\t') {
			i++
		}
		if i >= len(content) || content[i] != '[' || lines >= len(segs) {
			break
		}
		n := p.parseRefDef(content[i:], segs[lines:], indented)
		if n == 0 {
			break
		}
		lines += bytes.Count(content[consumed:i+n], []byte{'\n'})
		consumed = i + n
	}
	if consumed == 0 {
		return segs
	}
	if consumed >= len(content) {
		return nil
	}
	return segs[min(lines, len(segs)):]
}

func (p *parser) finalizeParagraph(b *node) {
	segs := p.paragraphLines(b, true)
	if len(segs) == 0 {
		// Removed paragraphs still make lists loose.
		b.kind = kindRefDefs
		return
	}
	if removed := b.segEnd - b.segStart - len(segs); removed > 0 && b.parent.kind == kindListItem {
		// goldmark keeps the definitions in a list item as an invisible
		// block before the paragraph, which then makes the list loose if a
		// blank line precedes the definitions.
		h := p.arena.new(kindRefDefs)
		h.startLine, h.lastLine = b.startLine, b.startLine+removed-1
		b.insertBefore(h)
	}
	// The text starts with the line at segStart, see inlineParser.lineBefore.
	b.segStart = b.segEnd - len(segs)
	if p.opts.Tables {
		segs = p.extractTable(b, segs, false)
		if len(segs) == 0 {
			b.unlink()
			return
		}
	}
	b.text = p.trimParagraph(p.joinSegs(segs))
}

func (p *parser) trimParagraph(content []byte) []byte {
	return bytes.TrimRight(content, " \t\r\n")
}

func (p *parser) trySetextHeading(container *node) int {
	if !p.allClosed {
		// The underline cannot continue a paragraph lazily.
		return startNone
	}
	c := p.peek(p.nextNonspace)
	i := p.nextNonspace
	for i < len(p.line) && p.line[i] == c {
		i++
	}
	if !isBlankBytes(p.line[i:]) {
		return startNone
	}
	p.closeUnmatchedBlocks()
	// goldmark parses the definitions before a setext underline without the
	// indentation of their lines.
	segs := p.paragraphLines(container, false)
	first := container.segEnd - len(segs)
	holder := first > container.segStart && len(segs) > 0 && container.parent.kind == kindListItem
	if holder {
		// See finalizeParagraph.
		h := p.arena.new(kindRefDefs)
		h.startLine, h.lastLine = container.startLine, container.startLine+first-container.segStart-1
		container.insertBefore(h)
	}
	table := false
	if p.opts.Tables && len(segs) > 0 {
		n := len(segs)
		segs = p.extractTable(container, segs, true)
		table = len(segs) != n
	}
	if len(segs) == 0 {
		// Without content, the underline is paragraph text, unless it is a
		// thematic break.
		if !holder && container.segEnd > container.segStart && container.parent.kind == kindListItem {
			// See finalizeParagraph.
			h := p.arena.new(kindRefDefs)
			h.startLine, h.lastLine = container.startLine, p.lineNumber-1
			container.insertBefore(h)
			container.blankBefore = false
		}
		container.segStart = len(p.segs)
		container.segEnd = len(p.segs)
		container.startLine = p.lineNumber
		p.emptiedParagraph = true
		return startNone
	}
	h := p.arena.new(kindHeading)
	h.startLine = container.startLine
	h.lastLine = container.lastLine
	if c == '=' {
		h.level = 1
	} else {
		h.level = 2
	}
	if table {
		// Only the last space or tab is removed from the lines before a
		// table.
		h.text = p.joinSegs(segs)
		if n := len(h.text); n > 0 && (h.text[n-1] == ' ' || h.text[n-1] == '\t') {
			h.text = h.text[:n-1]
			h.textEOL = true
		}
	} else {
		h.text = p.trimParagraph(p.joinSegs(segs))
	}
	h.setext = true
	h.blankBefore = container.blankBefore
	h.segStart, h.segEnd = first, first+len(segs)
	container.insertAfter(h)
	container.unlink()
	p.tip = h.parent
	p.touch(h)
	p.advanceOffset(len(p.line)-p.offset, false)
	return startLeaf
}

// parseRefDef parses a link reference definition at the start of b and
// returns its length including the line ending, or 0. The lines of b start
// with segs[0], and indented is as for paragraphLines.
func (p *parser) parseRefDef(b []byte, segs []seg, indented bool) int {
	label, i := scanLinkLabel(b, 0)
	if i < 0 || i >= len(b) || b[i] != ':' {
		return 0
	}
	i = skipSpaceNewline(b, i+1)
	dest, j, ok := scanLinkDest(b, i)
	if !ok || (j == i && (i >= len(b) || b[i] != '<')) {
		return 0
	}
	afterDest := j
	// Try with a title first.
	k := skipSpaceNewline(b, j)
	var title []byte
	hasTitle := false
	end := -1
	if k > j && k < len(b) {
		if t, e, ok := scanLinkTitle(b, k); ok {
			if eol, ok := lineEndAfterSpace(b, e); ok {
				title, hasTitle, end = t, true, eol
			} else if bytes.IndexByte(b[j:k], '\n') >= 0 {
				// A title on a later line that is followed by text still
				// counts, but only the first line of the definition is
				// consumed.
				title, hasTitle, end = t, true, bytes.IndexByte(b, '\n')+1
			}
		}
	}
	if end < 0 {
		eol, ok := lineEndAfterSpace(b, afterDest)
		if !ok {
			return 0
		}
		end = eol
	}
	key := normalizeLabel(label)
	if key == "" {
		return 0
	}
	if p.refs == nil {
		p.refs = make(map[string]linkRef)
	}
	if _, exists := p.refs[key]; !exists {
		ref := linkRef{dest: p.unescapeDest(dest), rawDest: dest}
		if hasTitle {
			// goldmark starts the title with the rest of a partially
			// consumed tab on its first line.
			if line := bytes.Count(b[:k], []byte{'\n'}); indented && line < len(segs) && segs[line].wsPad > 0 {
				title = append(spaces[:segs[line].wsPad:segs[line].wsPad], title...)
			}
			ref.title = p.unescapeString(title)
			if ref.title == nil {
				ref.title = []byte{}
			}
		}
		p.refs[key] = ref
	}
	return end
}

// lineEndAfterSpace skips spaces and tabs and returns the offset after the
// line ending, if nothing else follows on the line.
func lineEndAfterSpace(b []byte, i int) (int, bool) {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t') {
		i++
	}
	if i >= len(b) {
		return i, true
	}
	if b[i] == '\n' {
		return i + 1, true
	}
	if b[i] == '\r' && i+1 < len(b) && b[i+1] == '\n' {
		return i + 2, true
	}
	return 0, false
}

// skipSpaceNewline skips spaces, tabs and up to one line ending.
func skipSpaceNewline(b []byte, i int) int {
	return skipTagSpace(b, i)
}

// scanLinkLabel scans a link label starting at b[i] == '[' and returns its
// contents and the offset after the closing bracket, or -1.
func scanLinkLabel(b []byte, i int) ([]byte, int) {
	if i >= len(b) || b[i] != '[' {
		return nil, -1
	}
	start := i + 1
	nonSpace := false
	for j := start; j < len(b) && j-start <= 999; j++ {
		switch c := b[j]; c {
		case '\\':
			if j+1 < len(b) && isASCIIPunct(b[j+1]) {
				j++
				nonSpace = true
			} else {
				nonSpace = true
			}
		case '[':
			return nil, -1
		case ']':
			if !nonSpace {
				return nil, -1
			}
			return b[start:j], j + 1
		case ' ', '\t', '\n', '\r':
		default:
			nonSpace = true
		}
	}
	return nil, -1
}

// scanLinkDest scans a link destination at b[i] and returns its raw
// contents and the offset after it.
func scanLinkDest(b []byte, i int) ([]byte, int, bool) {
	if i < len(b) && b[i] == '<' {
		for j := i + 1; j < len(b); j++ {
			switch b[j] {
			case '\\':
				if j+1 < len(b) && isASCIIPunct(b[j+1]) {
					j++
				}
			case '\n', '\r', '<':
				// An unescaped "<" is not allowed in the destination, which
				// also keeps the scan of runs like "[a](<[a](<" linear.
				return nil, 0, false
			case '>':
				return b[i+1 : j], j + 1, true
			}
		}
		return nil, 0, false
	}
	depth := 0
	j := i
loop:
	for j < len(b) {
		c := b[j]
		switch {
		case c == '\\' && j+1 < len(b) && isASCIIPunct(b[j+1]):
			j += 2
			continue
		case c == '(':
			depth++
			if depth > 32 {
				return nil, 0, false
			}
		case c == ')':
			if depth == 0 {
				break loop
			}
			depth--
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			// Other control characters are allowed.
			break loop
		}
		j++
	}
	// Unbalanced opening parentheses are allowed.
	return b[i:j], j, true
}

// scanLinkTitle scans a link title at b[i] and returns its raw contents and
// the offset after it.
func scanLinkTitle(b []byte, i int) ([]byte, int, bool) {
	if i >= len(b) {
		return nil, 0, false
	}
	open := b[i]
	closer := open
	switch open {
	case '"', '\'':
	case '(':
		closer = ')'
	default:
		return nil, 0, false
	}
	for j := i + 1; j < len(b); j++ {
		c := b[j]
		switch {
		case c == '\\' && j+1 < len(b) && isASCIIPunct(b[j+1]):
			j++
		case c == closer:
			return b[i+1 : j], j + 1, true
		case open == '(' && c == '(':
			return nil, 0, false
		case c == '\n':
			// A title cannot contain a blank line.
			k := j + 1
			for k < len(b) && (b[k] == ' ' || b[k] == '\t') {
				k++
			}
			if k >= len(b) || b[k] == '\n' || (b[k] == '\r' && k+1 < len(b) && b[k+1] == '\n') {
				return nil, 0, false
			}
		}
	}
	return nil, 0, false
}

// normalizeLabel returns the key of a link label: case folded, with
// whitespace collapsed and trimmed.
func normalizeLabel(label []byte) string {
	var buf [64]byte
	out := buf[:0]
	space := false
	for i := 0; i < len(label); {
		c := label[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			space = true
			i++
			continue
		}
		if space && len(out) > 0 {
			out = append(out, ' ')
		}
		space = false
		if c < utf8.RuneSelf {
			out = append(out, toLowerASCII(c))
			i++
			continue
		}
		r, size := utf8.DecodeRune(label[i:])
		if f, ok := caseFolding[r]; ok {
			out = append(out, f...)
		} else {
			out = utf8.AppendRune(out, r)
		}
		i += size
	}
	return string(out)
}
