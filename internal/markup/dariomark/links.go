package dariomark

func (ip *inlineParser) openBracket(image bool) {
	start := ip.pos
	end := start + 1
	if image {
		end++
	}
	if ip.p.opts.Footnotes && ip.footnoteRef() {
		return
	}
	t := ip.p.arena.new(kindText)
	t.text = ip.src[start:end]
	ip.appendNode(t, end)
	ip.brackets = append(ip.brackets, bracket{node: t, pos: end, image: image, active: true, delimBottom: ip.delimTop})
}

func (ip *inlineParser) closeBracket() bool {
	if len(ip.brackets) == 0 {
		ip.mergeLone()
		return false
	}
	src := ip.src
	closePos := ip.pos
	b := ip.brackets[len(ip.brackets)-1]
	if !b.active {
		ip.mergeLone()
		for i := range ip.lone {
			if ip.lone[i].node.next == b.node {
				ip.lone[i].failedBracket = true
			}
		}
		ip.brackets = ip.brackets[:len(ip.brackets)-1]
		return false
	}

	var dest, rawDest, title []byte
	hasTitle := false
	matched := false
	end := closePos + 1

	// Inline link.
	if end < len(src) && src[end] == '(' {
		if d, rd, t, ht, e, ok := ip.parseInlineLinkTail(end + 1); ok {
			dest, rawDest, title, hasTitle, matched, end = d, rd, t, ht, true, e
		}
	}
	if !matched {
		label := src[b.pos:closePos]
		labelEnd := end
		if end < len(src) && src[end] == '[' {
			if l, e := scanLinkLabel(src, end); e >= 0 {
				label, labelEnd = l, e
			} else if e := blankLabelEnd(src, end+1); e >= 0 {
				// A collapsed reference. goldmark also accepts whitespace
				// between the brackets.
				labelEnd = e
			}
		}
		if label != nil && validLabel(label) {
			if ref, ok := ip.p.refs[normalizeLabel(label)]; ok && ip.p.spendRefBudget(ref) {
				dest, rawDest, title, hasTitle, matched, end = ref.dest, ref.rawDest, ref.title, ref.title != nil, true, labelEnd
			}
		}
	}
	if !matched {
		ip.mergeLone()
		for i := range ip.lone {
			if ip.lone[i].node.next == b.node {
				ip.lone[i].failedBracket = true
			}
		}
		ip.brackets = ip.brackets[:len(ip.brackets)-1]
		return false
	}

	ip.mergeLone()
	if closePos == ip.lineStart && ip.textStart == closePos {
		ip.lineStart = end
	}
	ip.flushText(closePos)
	k := kindLink
	if b.image {
		k = kindImage
	}
	link := ip.p.arena.new(k)
	link.dest = dest
	link.rawDest = rawDest
	link.title = title
	link.hasTitle = hasTitle
	for n := b.node.next; n != nil; {
		next := n.next
		n.unlink()
		link.appendChild(n)
		n = next
	}
	ip.parent.appendChild(link)
	ip.processEmphasis(b.delimBottom)
	b.node.unlink()
	ip.brackets = ip.brackets[:len(ip.brackets)-1]
	if !b.image {
		// Links cannot contain other links.
		for i := range ip.brackets {
			if !ip.brackets[i].image {
				ip.brackets[i].active = false
			}
		}
	}
	ip.pos, ip.textStart, ip.boundary = end, end, end
	return true
}

// spendRefBudget reports whether the document may expand another use of ref,
// and accounts for it. Each use copies the destination and title of the
// definition into the output, which would make the output quadratic in the
// input, e.g., for a long destination used by many short references. Past
// the budget, references render as text, like undefined ones.
func (p *parser) spendRefBudget(ref linkRef) bool {
	cost := len(ref.dest) + len(ref.title)
	if cost > p.refBudget {
		return false
	}
	p.refBudget -= cost
	return true
}

// validLabel reports whether label is a valid link label: at most 999
// characters, with no unescaped brackets and at least one character that is
// not whitespace.
func validLabel(label []byte) bool {
	if len(label) > 999 {
		return false
	}
	nonSpace := false
	for i := 0; i < len(label); i++ {
		switch c := label[i]; c {
		case '\\':
			nonSpace = true
			if i+1 < len(label) && isASCIIPunct(label[i+1]) {
				i++
			}
		case '[', ']':
			return false
		case ' ', '\t', '\n', '\r':
		default:
			nonSpace = true
		}
	}
	return nonSpace
}

// parseInlineLinkTail parses the destination and title of an inline link
// after the opening parenthesis.
func (ip *inlineParser) parseInlineLinkTail(i int) (dest, rawDest, title []byte, hasTitle bool, end int, ok bool) {
	src := ip.src
	i = skipSpaceNewline(src, i)
	if i < len(src) && src[i] == ')' {
		return []byte{}, []byte{}, nil, false, i + 1, true
	}
	d, j, ok := scanLinkDest(src, i)
	if !ok {
		return nil, nil, nil, false, 0, false
	}
	k := skipSpaceNewline(src, j)
	if k < len(src) && src[k] == ')' {
		return ip.p.unescapeDest(d), d, nil, false, k + 1, true
	}
	if k == j && (i >= len(src) || src[i] != '<') {
		// The title must be separated from the destination, unless the
		// destination is in angle brackets, which goldmark accepts.
		return nil, nil, nil, false, 0, false
	}
	t, e, ok := scanLinkTitle(src, k)
	if !ok {
		return nil, nil, nil, false, 0, false
	}
	e = skipSpaceNewline(src, e)
	if e >= len(src) || src[e] != ')' {
		return nil, nil, nil, false, 0, false
	}
	title = ip.p.unescapeString(t)
	if title == nil {
		title = []byte{}
	}
	return ip.p.unescapeDest(d), d, title, true, e + 1, true
}

// blankLabelEnd returns the offset after the "]" that follows only spaces,
// tabs and line endings from b[i], or -1.
func blankLabelEnd(b []byte, i int) int {
	for ; i < len(b); i++ {
		switch b[i] {
		case ']':
			return i + 1
		case ' ', '\t', '\n', '\r':
		default:
			return -1
		}
	}
	return -1
}
