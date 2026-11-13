package dariomark

import (
	"unicode"
	"unicode/utf8"
)

// isUnicodeWhitespace reports whether r is a Unicode whitespace character as
// defined by CommonMark.
func isUnicodeWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return r >= 0x80 && unicode.Is(unicode.Zs, r)
}

// isUnicodePunct reports whether r is a Unicode punctuation character as
// defined by CommonMark: a character in the P or S general categories.
func isUnicodePunct(r rune) bool {
	if r < 0x80 {
		return isASCIIPunct(byte(r))
	}
	return unicode.In(r, unicode.P, unicode.S)
}

// runeBefore returns the rune before src[i], or '\n' at the start.
func runeBefore(src []byte, i int) rune {
	if i <= 0 {
		return '\n'
	}
	r, _ := utf8.DecodeLastRune(src[:i])
	return r
}

// runeAt returns the rune at src[i], or '\n' at the end.
func runeAt(src []byte, i int) rune {
	if i >= len(src) {
		return '\n'
	}
	r, _ := utf8.DecodeRune(src[i:])
	return r
}

func (ip *inlineParser) delimRun(c byte) bool {
	src := ip.src
	start := ip.pos
	end := start
	for end < len(src) && src[end] == c {
		end++
	}
	n := end - start
	before := ip.sourceRuneBefore(start)
	after := runeAt(src, end)
	afterWS, afterP := isUnicodeWhitespace(after), isUnicodePunct(after)
	beforeWS, beforeP := isUnicodeWhitespace(before), isUnicodePunct(before)
	leftFlanking := !afterWS && (!afterP || beforeWS || beforeP)
	rightFlanking := !beforeWS && (!beforeP || afterWS || afterP)
	var canOpen, canClose bool
	if c == '_' {
		canOpen = leftFlanking && (!rightFlanking || beforeP)
		canClose = rightFlanking && (!leftFlanking || afterP)
	} else {
		canOpen = leftFlanking
		canClose = rightFlanking
	}
	if c == '~' && n > 2 {
		canOpen, canClose = false, false
	}

	d := delimiter{start: int32(start), char: c, count: int32(n), origCount: int32(n), canOpen: canOpen, canClose: canClose}
	if canClose && ip.openers[delimIndex(c)] > 0 {
		// The run may close emphasis, so it gets a node of its own.
		t := ip.p.arena.new(kindText)
		t.text = src[start:end]
		ip.appendNode(t, end)
		d.node = t
	} else {
		ip.keepPending()
		ip.pos, ip.textStart, ip.boundary = end, end, end
	}
	if canOpen || canClose {
		ip.pushDelim(d)
	}
	return true
}

// delimIndex returns the index of the delimiter character c in the per
// character tables of the delimiter stack.
func delimIndex(c byte) int {
	switch c {
	case '*':
		return 0
	case '_':
		return 1
	case '~':
		return 2
	case '\'':
		return 3
	}
	return 4
}

// materialize gives the delimiter d, which has no node yet, a text node of
// its own, by splitting the text node that holds it. That node is a sibling
// before closer, which is a node of a delimiter that d matches.
func (ip *inlineParser) materialize(d *delimiter, closer *node) bool {
	start, end := int(d.start), int(d.start+d.origCount)
	for n := closer.prev; n != nil; n = n.prev {
		if n.kind != kindText || !ip.isSrcText(n) {
			continue
		}
		s := ip.srcStart(n)
		if s > start {
			continue
		}
		e := s + len(n.text)
		if e < end {
			return false
		}
		if e > end {
			a := ip.p.arena.new(kindText)
			a.text = ip.src[end:e]
			n.insertAfter(a)
		}
		if s < start {
			b := ip.p.arena.new(kindText)
			b.text = ip.src[s:start]
			n.insertBefore(b)
		}
		n.text = ip.src[start:end]
		d.node = n
		return true
	}
	return false
}

// isSrcText reports whether the text of the text node t is a slice of the
// source of the inline content, rather than, e.g., a decoded entity.
func (ip *inlineParser) isSrcText(t *node) bool {
	if len(t.text) == 0 || cap(t.text) > cap(ip.src) {
		return false
	}
	start := cap(ip.src) - cap(t.text)
	return start+len(t.text) <= len(ip.src) && &ip.src[start] == &t.text[0]
}

// srcStart returns the offset of the text of t in the source, see isSrcText.
func (ip *inlineParser) srcStart(t *node) int {
	return cap(ip.src) - cap(t.text)
}

func (ip *inlineParser) pushDelim(d delimiter) {
	if d.canOpen {
		ip.openers[delimIndex(d.char)]++
	}
	if ip.delimTop < 0 {
		if ip.brackets.len() == 0 {
			// Nothing refers to the removed delimiters, whose indices are
			// reused.
			ip.delims.truncate(0)
		}
		ip.delimFirst = int32(ip.delims.len())
	}
	d.prev = ip.delimTop
	d.next = -1
	ip.delims.push(d)
	i := int32(ip.delims.len() - 1)
	if ip.delimTop >= 0 {
		ip.delims.at(int(ip.delimTop)).next = i
	}
	ip.delimTop = i
}

func (ip *inlineParser) removeDelim(i int32) {
	d := ip.delims.at(int(i))
	if d.canOpen {
		ip.openers[delimIndex(d.char)]--
	}
	if d.prev >= 0 {
		ip.delims.at(int(d.prev)).next = d.next
	} else {
		ip.delimFirst = d.next
	}
	if d.next >= 0 {
		ip.delims.at(int(d.next)).prev = d.prev
	} else {
		ip.delimTop = d.prev
	}
	d.prev, d.next = -2, -2
}

// processEmphasis matches the delimiters above bottom, an index in the
// delimiter stack or -1, into emphasis and strikethrough.
func (ip *inlineParser) processEmphasis(bottom int32) {
	// Find the first delimiter above bottom.
	cur := ip.delimTop
	first := int32(-1)
	for cur > bottom {
		first = cur
		cur = ip.delims.at(int(cur)).prev
	}
	if first < 0 {
		return
	}
	// openersBottom is indexed by character, by whether the closer can open,
	// and by the length of the closer modulo 3. Delimiter indices grow with
	// the stack, so bounds are compared by index: a bound may have been
	// removed from the stack since it was set.
	var openersBottom [5][2][3]int32
	for a := range openersBottom {
		for b := range openersBottom[a] {
			for c := range openersBottom[a][b] {
				openersBottom[a][b][c] = bottom
			}
		}
	}
	closer := first
	for closer >= 0 {
		cd := ip.delims.at(int(closer))
		if !cd.canClose {
			closer = cd.next
			continue
		}
		ci := delimIndex(cd.char)
		oi := 0
		if cd.canOpen {
			oi = 1
		}
		mod := cd.origCount % 3
		found := false
		opener := cd.prev
		quote := cd.char == '\'' || cd.char == '"'
		for opener > bottom && opener > openersBottom[ci][oi][mod] {
			od := ip.delims.at(int(opener))
			// Strikethrough pairs tildes of the same length (GFM 0.29,
			// section 6.5).
			if od.char == cd.char && od.canOpen && (cd.char != '~' || od.origCount == cd.origCount) {
				oddMatch := !quote && (cd.canOpen || od.canClose) && cd.origCount%3 != 0 &&
					(od.origCount+cd.origCount)%3 == 0
				found = !oddMatch && cd.node != nil && (od.node != nil || ip.materialize(od, cd.node))
				if found {
					break
				}
			}
			opener = od.prev
		}
		if !found {
			openersBottom[ci][oi][mod] = cd.prev
			next := cd.next
			if !cd.canOpen {
				ip.removeDelim(closer)
			}
			closer = next
			continue
		}

		od := ip.delims.at(int(opener))
		if quote {
			// Quotes pair without affecting the delimiters between them.
			if cd.char == '"' {
				od.node.level, cd.node.level = int32(typoLeftDouble), int32(typoRightDouble)
			} else {
				od.node.level, cd.node.level = int32(typoLeftSingle), int32(typoRightSingle)
			}
			next := cd.next
			ip.removeDelim(opener)
			ip.removeDelim(closer)
			closer = next
			continue
		}
		use := int32(1)
		if od.count >= 2 && cd.count >= 2 {
			use = 2
		}
		k := kindEmphasis
		switch {
		case cd.char == '~':
			k = kindStrikethrough
		case use == 2:
			k = kindStrong
		}
		od.count -= use
		cd.count -= use
		od.node.text = od.node.text[:len(od.node.text)-int(use)]
		cd.node.text = cd.node.text[:len(cd.node.text)-int(use)]

		emph := ip.p.arena.new(k)
		emph.level = use
		// Move the nodes between the opener and the closer into emph.
		for n := od.node.next; n != nil && n != cd.node; {
			next := n.next
			n.unlink()
			emph.appendChild(n)
			n = next
		}
		od.node.insertAfter(emph)

		// Remove the delimiters between the opener and the closer.
		for d := od.next; d >= 0 && d != closer; {
			next := ip.delims.at(int(d)).next
			ip.removeDelim(d)
			d = next
		}
		if od.count == 0 {
			od.node.unlink()
			ip.removeDelim(opener)
		}
		if cd.count == 0 {
			next := cd.next
			cd.node.unlink()
			ip.removeDelim(closer)
			closer = next
		}
	}
	// Remove the remaining delimiters above bottom.
	for ip.delimTop > bottom {
		ip.removeDelim(ip.delimTop)
	}
}
