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
	if c == '~' && (n > 2 || start > 0 && src[start-1] == '~') {
		// A tilde run directly after an escaped tilde never forms
		// strikethrough.
		canOpen, canClose = false, false
	}

	t := ip.p.arena.new(kindText)
	t.text = src[start:end]
	lineStart := ip.lineStart
	ip.appendNode(t, end)
	if c == '~' && n > 2 {
		// Such tildes are text for lineStartBackslash.
		ip.lineStart = lineStart
	}
	if canOpen || canClose {
		ip.pushDelim(delimiter{node: t, char: c, count: n, origCount: n, canOpen: canOpen, canClose: canClose})
	}
	return true
}

func (ip *inlineParser) pushDelim(d delimiter) {
	d.prev = ip.delimTop
	d.next = -1
	ip.delims = append(ip.delims, d)
	i := len(ip.delims) - 1
	if ip.delimTop >= 0 {
		ip.delims[ip.delimTop].next = i
	}
	ip.delimTop = i
}

func (ip *inlineParser) removeDelim(i int) {
	d := &ip.delims[i]
	if d.prev >= 0 {
		ip.delims[d.prev].next = d.next
	}
	if d.next >= 0 {
		ip.delims[d.next].prev = d.prev
	} else {
		ip.delimTop = d.prev
	}
	d.prev, d.next = -2, -2
}

// processEmphasis matches the delimiters above bottom, an index in the
// delimiter stack or -1, into emphasis and strikethrough.
func (ip *inlineParser) processEmphasis(bottom int) {
	// Find the first delimiter above bottom.
	cur := ip.delimTop
	first := -1
	for cur > bottom {
		first = cur
		cur = ip.delims[cur].prev
	}
	if first < 0 {
		return
	}
	// openersBottom is indexed by character, by whether the closer can open,
	// and by the length of the closer modulo 3. Delimiter indices grow with
	// the stack, so bounds are compared by index: a bound may have been
	// removed from the stack since it was set.
	var openersBottom [3][2][3]int
	for a := range openersBottom {
		for b := range openersBottom[a] {
			for c := range openersBottom[a][b] {
				openersBottom[a][b][c] = bottom
			}
		}
	}
	charIndex := func(c byte) int {
		switch c {
		case '*':
			return 0
		case '_':
			return 1
		}
		return 2
	}

	closer := first
	for closer >= 0 {
		cd := &ip.delims[closer]
		if !cd.canClose {
			closer = cd.next
			continue
		}
		ci := charIndex(cd.char)
		oi := 0
		if cd.canOpen {
			oi = 1
		}
		mod := cd.origCount % 3
		found := false
		opener := cd.prev
		for opener > bottom && opener > openersBottom[ci][oi][mod] {
			od := &ip.delims[opener]
			if od.char == cd.char && od.canOpen {
				oddMatch := (cd.canOpen || od.canClose) && cd.origCount%3 != 0 &&
					(od.origCount+cd.origCount)%3 == 0
				found = !oddMatch
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

		od := &ip.delims[opener]
		use := 1
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
		od.node.text = od.node.text[:len(od.node.text)-use]
		cd.node.text = cd.node.text[:len(cd.node.text)-use]

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
			next := ip.delims[d].next
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
