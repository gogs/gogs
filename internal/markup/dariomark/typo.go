package dariomark

import (
	"bytes"
	"unicode"
	"unicode/utf8"
)

type typo uint8

const (
	typoLeftSingle typo = iota
	typoRightSingle
	typoLeftDouble
	typoRightDouble
	typoEnDash
	typoEmDash
	typoEllipsis
	typoLeftAngle
	typoRightAngle
)

var typoEntities = [...]struct{ html, text string }{
	typoLeftSingle:  {"&lsquo;", "\u2018"},
	typoRightSingle: {"&rsquo;", "\u2019"},
	typoLeftDouble:  {"&ldquo;", "\u201c"},
	typoRightDouble: {"&rdquo;", "\u201d"},
	typoEnDash:      {"&ndash;", "\u2013"},
	typoEmDash:      {"&mdash;", "\u2014"},
	typoEllipsis:    {"&hellip;", "\u2026"},
	typoLeftAngle:   {"&laquo;", "\u00ab"},
	typoRightAngle:  {"&raquo;", "\u00bb"},
}

// typoEntity returns the replacement of t under the options.
func (o *Typographer) entity(t typo) (html, text string) {
	switch t {
	case typoEnDash:
		if !o.LatexDashes {
			t = typoEmDash
		}
	case typoLeftDouble:
		if o.AngledQuotes {
			t = typoLeftAngle
		}
	case typoRightDouble:
		if o.AngledQuotes {
			t = typoRightAngle
		}
	}
	e := typoEntities[t]
	return e.html, e.text
}

var (
	inlineSpecialTypo               = withSpecial(inlineSpecial, `"'-.>123456789`)
	inlineSpecialTypoLinkify        = withSpecial(inlineSpecialLinkify, `"'-.>123456789`)
	inlineSpecialTypoLinkifyScheme  = withSpecial(inlineSpecialLinkifyScheme, `"'-.>123456789`)
	inlineSpecialTypoNoDash         = withSpecial(inlineSpecial, `"'.>123456789`)
	inlineSpecialTypoNoDashLinkify  = withSpecial(inlineSpecialLinkify, `"'.>123456789`)
	inlineSpecialTypoNoDashLinkifyS = withSpecial(inlineSpecialLinkifyScheme, `"'.>123456789`)
)

// typographer replaces the punctuation at the current position, which is not
// escaped.
func (ip *inlineParser) typographer(c byte) bool {
	src := ip.src
	i := ip.pos
	switch c {
	case '-':
		if !ip.p.opts.Typographer.Dashes || i+1 >= len(src) || src[i+1] != '-' {
			return false
		}
		if i+2 < len(src) && src[i+2] == '-' {
			ip.addTypo(typoEmDash, 3)
		} else {
			ip.addTypo(typoEnDash, 2)
		}
		return true
	case '.':
		if i+2 < len(src) && src[i+1] == '.' && src[i+2] == '.' {
			ip.addTypo(typoEllipsis, 3)
			return true
		}
	case '<':
		if i+1 < len(src) && src[i+1] == '<' {
			ip.addTypo(typoLeftAngle, 2)
			return true
		}
	case '>':
		if i+1 < len(src) && src[i+1] == '>' {
			ip.addTypo(typoRightAngle, 2)
			return true
		}
	case '"':
		return ip.doubleQuote()
	case '\'':
		return ip.singleQuote()
	default:
		if '1' <= c && c <= '9' {
			return ip.fraction()
		}
	}
	return false
}

// fraction replaces the fraction at the current position. A fraction is
// surrounded by fractionBoundary bytes, and its numerator does not start
// with "0".
func (ip *inlineParser) fraction() bool {
	src := ip.src
	i := ip.pos
	if i > 0 && !fractionBoundary(src[i-1]) {
		return false
	}
	if !ip.p.opts.Typographer.Fractions {
		// Only "1/2", "1/4" and "3/4" are replaced. "1/4th" and "3/4ths" are
		// ordinals.
		rest := src[i:]
		var ok bool
		switch {
		case bytes.HasPrefix(rest, []byte("1/2")):
			ok = len(rest) == 3 || fractionBoundary(rest[3])
		case bytes.HasPrefix(rest, []byte("1/4")):
			ok = len(rest) == 3 || fractionBoundary(rest[3]) || bytes.HasPrefix(rest[3:], []byte("th"))
		case bytes.HasPrefix(rest, []byte("3/4")):
			ok = len(rest) == 3 || fractionBoundary(rest[3]) || bytes.HasPrefix(rest[3:], []byte("ths"))
		}
		if !ok {
			return false
		}
		ip.addFraction(3, 0)
		return true
	}
	j := i
	for j < len(src) && isASCIIDigit(src[j]) {
		j++
	}
	numerator := j - i
	switch {
	case j < len(src) && src[j] == '/':
		j++
	case bytes.HasPrefix(src[j:], fractionSlash):
		j += len(fractionSlash)
	default:
		return false
	}
	k := j
	for k < len(src) && isASCIIDigit(src[k]) {
		k++
	}
	if k == j || k < len(src) && !fractionBoundary(src[k]) {
		return false
	}
	ip.addFraction(k-i, numerator)
	return true
}

// fractionSlash is U+2044 FRACTION SLASH.
var fractionSlash = []byte("\u2044")

// fractionBoundary reports whether c may precede or follow a fraction, i.e.,
// it is ASCII but neither a letter, a digit nor "/".
func fractionBoundary(c byte) bool {
	return c < utf8.RuneSelf && c != '/' && !isASCIIAlnum(c)
}

func (ip *inlineParser) addFraction(n, numerator int) {
	node := ip.p.arena.new(kindFraction)
	node.text = ip.src[ip.pos : ip.pos+n]
	node.level = numerator
	ip.appendNode(node, ip.pos+n)
}

// vulgarFraction returns the HTML and the text of the vulgar fraction of n.
func vulgarFraction(n *node) (html, text string) {
	switch {
	case n.text[0] == '3':
		return "&frac34;", "\u00be"
	case n.text[2] == '4':
		return "&frac14;", "\u00bc"
	}
	return "&frac12;", "\u00bd"
}

func (ip *inlineParser) addTypo(t typo, n int) {
	node := ip.p.arena.new(kindTypo)
	node.level = int(t)
	ip.appendNode(node, ip.pos+n)
}

type charClass uint8

const (
	classSpace charClass = iota
	classPunct
	classSymbol
	// classGap is a character before a quote that is neither a letter, a
	// digit, whitespace nor punctuation, e.g., a combining mark. After a quote,
	// it is classOther.
	classGap
	classDigit
	classOther
)

func runeClass(r rune) charClass {
	switch {
	// Quotes treat all Unicode spaces as spaces, e.g., U+2028.
	case isUnicodeWhitespace(r) || unicode.IsSpace(r):
		return classSpace
	case r < 0x80 && isASCIIPunct(byte(r)):
		if unicode.IsPunct(r) {
			return classPunct
		}
		return classSymbol
	case r >= '0' && r <= '9':
		return classDigit
	case r < 0x80 && (r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'):
		return classOther
	case unicode.IsPunct(r):
		return classPunct
	case unicode.IsSymbol(r):
		return classSymbol
	case unicode.IsDigit(r):
		return classDigit
	case unicode.IsLetter(r):
		return classOther
	}
	return classGap
}

// byteClass classifies the byte at src[i] on its own, which treats every
// byte of a multibyte character as other, and ASCII symbols as punctuation.
func byteClass(src []byte, i int) charClass {
	if i >= len(src) {
		return classSpace
	}
	c := src[i]
	switch {
	case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v':
		return classSpace
	case isASCIIPunct(c):
		return classPunct
	case c >= '0' && c <= '9':
		return classDigit
	}
	return classOther
}

func classAt(src []byte, i int) charClass {
	if i >= len(src) {
		return classSpace
	}
	if c := runeClass(runeAt(src, i)); c != classGap {
		return c
	}
	return classOther
}

// quoteBefore classifies the character before the quote at src[i].
func (ip *inlineParser) quoteBefore(i int) charClass {
	return runeClass(ip.sourceRuneBefore(i))
}

// sourceRuneBefore returns the rune before src[i]. On a line other than the
// first of a paragraph, it is the rune before the line in the source, e.g.,
// ">" of a blockquote marker without a space, which goldmark uses for quotes
// and delimiter runs.
func (ip *inlineParser) sourceRuneBefore(i int) rune {
	src := ip.src
	if i <= 0 {
		return '\n'
	}
	if src[i-1] != '\n' || (ip.parent.kind != kindParagraph && ip.parent.kind != kindHeading) {
		return runeBefore(src, i)
	}
	ip.line += bytes.Count(src[ip.linePos:i], newline)
	ip.linePos = i
	k := ip.parent.segStart + ip.line
	if k >= ip.parent.segEnd {
		return '\n'
	}
	return runeBefore(ip.p.src, int(ip.p.segs[k].start))
}

func isWordClass(c charClass) bool {
	return c >= classDigit
}

func isPunctClass(c charClass) bool {
	return c == classPunct || c == classSymbol
}

// canCloseQuote reports whether a quote between the characters of the
// classes before and after can close a quotation. next is the position of the
// second byte after the quote.
func canCloseQuote(src []byte, before, after charClass, next int) bool {
	switch {
	case before == classSpace:
		return false
	case isPunctClass(before):
		return after == classSpace || after == classPunct && byteClass(src, next) <= classPunct
	}
	return !isWordClass(after)
}

// doubleQuote converts a straight double quote. An opening quote is always
// converted, and a closing quote only after an opening quote earlier in the
// document. Opening looks past a run of quotes.
func (ip *inlineParser) doubleQuote() bool {
	src := ip.src
	i := ip.pos
	j := i + 1
	for j < len(src) && src[j] == '"' {
		j++
	}
	before := ip.quoteBefore(i)
	afterRun := classAt(src, j)
	canOpen := afterRun != classSpace && (before == classSpace || isPunctClass(before) && isWordClass(afterRun))
	var canClose bool
	switch {
	case before == classSpace:
	case isPunctClass(before):
		canClose = afterRun == classSpace || canCloseQuote(src, before, classAt(src, i+1), i+2)
	default:
		// Closing looks past at most one more quote.
		after := afterRun
		if j > i+2 {
			after = classAt(src, i+2)
		}
		canClose = !isWordClass(after) && (before != classDigit || j == i+1)
	}
	switch {
	case canOpen && canClose:
		ip.doubleOpen++
		ip.addTypo(typoLeftDouble, 1)
	case ip.doubleOpen > 0 && canClose:
		ip.doubleOpen--
		ip.addTypo(typoRightDouble, 1)
	case canOpen:
		ip.doubleOpen++
		ip.addTypo(typoLeftDouble, 1)
	default:
		return false
	}
	return true
}

// singleQuote converts a straight single quote, which is also an apostrophe.
func (ip *inlineParser) singleQuote() bool {
	src := ip.src
	i := ip.pos
	before := ip.quoteBefore(i)
	after := classAt(src, i+1)
	if isWordClass(before) || before == classGap {
		switch {
		case after == classDigit:
			return false
		case after == classOther && before != classGap:
			ip.addTypo(typoRightSingle, 1)
			return true
		}
	} else {
		next := byte(0)
		if i+1 < len(src) {
			next = src[i+1]
		}
		switch {
		case before != classSymbol && (next == 'e' || next == 'l' || next == 'n' || next == 't'),
			(next == 'd' || next == 'm' || next == 's' || next == 't') && byteClass(src, i+2) <= classPunct,
			i+2 < len(src) && isContraction2(next, src[i+2]) && byteClass(src, i+3) <= classPunct,
			after == classDigit && classAt(src, i+2) == classDigit && i+3 < len(src) && src[i+3] == 's' && !isWordRune(runeAt(src, i+4), i+4 < len(src)):
			// Contractions and abbreviated years, e.g., "'tis" and "'90s".
			ip.addTypo(typoRightSingle, 1)
			return true
		}
		j := i + 1
		for j < len(src) && src[j] == '\'' {
			j++
		}
		runEnd := classAt(src, j)
		if isWordClass(runEnd) || before == classSpace && isPunctClass(after) && runEnd != classSpace {
			ip.singleOpen++
			ip.addTypo(typoLeftSingle, 1)
			return true
		}
	}
	// The rest of the line, including the line ending, has at least two bytes.
	rest := 0
	k := i + 1
	for ; k < len(src) && rest < 2; k++ {
		rest++
		if src[k] == '\n' {
			break
		}
	}
	if k == len(src) && ip.parent.textEOL {
		rest++
	}
	switch {
	case rest >= 2:
		ip.addTypo(typoRightSingle, 1)
	case ip.singleOpen > 0 && canCloseQuote(src, before, after, i+2):
		ip.singleOpen--
		ip.addTypo(typoRightSingle, 1)
	default:
		return false
	}
	return true
}

// isContraction2 reports whether a and b end a two-letter contraction, e.g.,
// "'ll".
func isContraction2(a, b byte) bool {
	return a == 'l' && b == 'l' || a == 'r' && b == 'e' || a == 'v' && b == 'e'
}

// isWordRune reports whether r is a letter, a digit or a combining mark.
func isWordRune(r rune, ok bool) bool {
	return ok && (unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r))
}

// countDigitsBack returns the number of ASCII digits at the end of b.
func countDigitsBack(b []byte) int {
	n := 0
	for n < len(b) && isASCIIDigit(b[len(b)-1-n]) {
		n++
	}
	return n
}
