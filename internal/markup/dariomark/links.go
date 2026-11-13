package dariomark

// openBracket pushes a bracket that may start a link or an image. The bracket
// stays in the pending text, so that brackets that do not form links cost no
// nodes. See flushText and closeBracket.
func (ip *inlineParser) openBracket(image bool) {
	start := ip.pos
	end := start + 1
	if image {
		end++
	}
	if ip.p.opts.Footnotes && ip.footnoteRef() {
		return
	}
	ip.keepPending()
	ip.brackets.push(bracket{start: int32(start), delimBottom: ip.delimTop})
	ip.pos, ip.textStart, ip.boundary = end, end, end
}

// image reports whether b opens an image.
func (b bracket) image(src []byte) bool {
	return src[b.start] == '!'
}

// end returns the offset after the opening bracket.
func (b bracket) end(src []byte) int {
	if b.image(src) {
		return int(b.start) + 2
	}
	return int(b.start) + 1
}

// popBracket removes the top bracket.
func (ip *inlineParser) popBracket() {
	n := ip.brackets.len() - 1
	ip.brackets.truncate(n)
	ip.unflushed = min(ip.unflushed, n)
	ip.inactiveBelow = min(ip.inactiveBelow, n)
	for len(ip.bracketText) > 0 && int(ip.bracketText[len(ip.bracketText)-1].first) >= n {
		ip.bracketText = ip.bracketText[:len(ip.bracketText)-1]
	}
}

func (ip *inlineParser) closeBracket() bool {
	if ip.brackets.len() == 0 {
		return false
	}
	src := ip.src
	closePos := ip.pos
	b := *ip.brackets.at(ip.brackets.len() - 1)
	image := b.image(src)
	if !image && ip.brackets.len()-1 < ip.inactiveBelow {
		// Links cannot contain other links.
		ip.popBracket()
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
		label := src[b.end(src):closePos]
		labelEnd := end
		if end < len(src) && src[end] == '[' {
			if l, e := scanLinkLabel(src, end); e >= 0 {
				label, labelEnd = l, e
			} else if end+1 < len(src) && src[end+1] == ']' {
				// A collapsed reference.
				labelEnd = end + 2
			}
		}
		if label != nil && validLabel(label) {
			if ref, ok := ip.p.refs[normalizeLabel(label)]; ok && ip.p.spendRefBudget(ref, image) {
				dest, rawDest, title, hasTitle, matched, end = ref.dest, ref.rawDest, ref.title, ref.title != nil, true, labelEnd
			}
		}
	}
	if !matched {
		ip.popBracket()
		return false
	}

	ip.flushText(closePos)
	k := kindLink
	if image {
		k = kindImage
	}
	link := ip.p.arena.new(k)
	e := ip.p.arena.newExt(link)
	e.dest = dest
	e.rawDest = rawDest
	e.title = title
	e.hasTitle = hasTitle
	for n := ip.splitBracket(b); n != nil; {
		next := n.next
		n.unlink()
		link.appendChild(n)
		n = next
	}
	ip.parent.appendChild(link)
	ip.processEmphasis(b.delimBottom)
	ip.popBracket()
	if !image {
		// Links cannot contain other links, so the brackets below become
		// inactive, except for images.
		ip.inactiveBelow = ip.brackets.len()
	}
	ip.pos, ip.textStart, ip.boundary = end, end, end
	return true
}

// splitBracket removes the opening bracket b, which is at the top of the
// stack, from the text node that holds it, and returns the first node after
// it, which starts the content of the link.
func (ip *inlineParser) splitBracket(b bracket) *node {
	g := ip.bracketText[len(ip.bracketText)-1]
	t := g.node
	off := int(b.start - g.start)
	var first *node
	if after := t.text[off+b.end(ip.src)-int(b.start):]; len(after) > 0 {
		a := ip.p.arena.new(kindText)
		a.text = after
		t.insertAfter(a)
		first = a
	} else {
		first = t.next
	}
	if off > 0 {
		t.text = t.text[:off]
	} else {
		// Only brackets from b on are in t, so its group is popped with b.
		t.unlink()
	}
	return first
}

// spendRefBudget reports whether the document may expand another use of ref,
// and accounts for it. Each use copies the destination and title of the
// definition into the output, which would make the output quadratic in the
// input, e.g., for a long destination used by many short references. Past
// the budget, references render as text, like undefined ones. Images count
// twice, since post-processing wraps them in links to their source.
func (p *parser) spendRefBudget(ref linkRef, image bool) bool {
	cost := len(ref.dest) + len(ref.title)
	if image {
		cost += len(ref.dest)
	}
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
	if k == j {
		// The title must be separated from the destination.
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
