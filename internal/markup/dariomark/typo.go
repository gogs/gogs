package dariomark

import (
	"bytes"
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
	inlineSpecialTypo               = withSpecial(inlineSpecial, `"'-.123456789`)
	inlineSpecialTypoLinkify        = withSpecial(inlineSpecialLinkify, `"'-.123456789`)
	inlineSpecialTypoLinkifyScheme  = withSpecial(inlineSpecialLinkifyScheme, `"'-.123456789`)
	inlineSpecialTypoNoDash         = withSpecial(inlineSpecial, `"'.123456789`)
	inlineSpecialTypoNoDashLinkify  = withSpecial(inlineSpecialLinkify, `"'.123456789`)
	inlineSpecialTypoNoDashLinkifyS = withSpecial(inlineSpecialLinkifyScheme, `"'.123456789`)
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
	case '"', '\'':
		ip.quote(c)
		return true
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
	node.level = int32(numerator)
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
	node.level = int32(t)
	ip.appendNode(node, ip.pos+n)
}

// quote converts a straight quote. Like cmark-gfm with smart punctuation,
// quotes are delimiters that pair up like emphasis, see processEmphasis. A
// quote that can open and is not right-flanking may start a pair, and a
// right-flanking quote may end one. Like cmark-gfm, a single quote after "]"
// or ")" does not start a pair, e.g., in "[link](url)'s". Unpaired single quotes are apostrophes,
// and unpaired double quotes are closing quotes when they could end a pair,
// otherwise opening quotes.
func (ip *inlineParser) quote(c byte) {
	i := ip.pos
	before := ip.sourceRuneBefore(i)
	after := runeAt(ip.src, i+1)
	afterWS, afterP := isUnicodeWhitespace(after), isUnicodePunct(after)
	beforeWS, beforeP := isUnicodeWhitespace(before), isUnicodePunct(before)
	leftFlanking := !afterWS && (!afterP || beforeWS || beforeP)
	rightFlanking := !beforeWS && (!beforeP || afterWS || afterP)
	canOpen := leftFlanking && !rightFlanking && !(c == '\'' && (before == ']' || before == ')'))
	canClose := rightFlanking
	t := typoRightSingle
	if c == '"' {
		t = typoLeftDouble
		if canClose {
			t = typoRightDouble
		}
	}
	node := ip.p.arena.new(kindTypo)
	node.level = int32(t)
	ip.appendNode(node, i+1)
	if canOpen || canClose {
		ip.pushDelim(delimiter{node: node, start: int32(i), char: c, count: 1, origCount: 1, canOpen: canOpen, canClose: canClose})
	}
}

// sourceRuneBefore returns the rune before src[i]. The start of the text
// counts as a line ending.
func (ip *inlineParser) sourceRuneBefore(i int) rune {
	if i <= 0 {
		return '\n'
	}
	return runeBefore(ip.src, i)
}

// countDigitsBack returns the number of ASCII digits at the end of b.
func countDigitsBack(b []byte) int {
	n := 0
	for n < len(b) && isASCIIDigit(b[len(b)-1-n]) {
		n++
	}
	return n
}
