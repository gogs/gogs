package dariomark

import (
	"bytes"
	"path"
	"regexp"
	"strings"
)

var (
	rawAnchorOpen  = regexp.MustCompile(`(?i)^<a[\s>]`)
	rawAnchorClose = regexp.MustCompile(`(?i)^</a\s*>`)
)

// special returns the table of characters that start inline constructs.
func (p *parser) specialChars() *[256]bool {
	if t := &p.opts.Typographer; t.Enabled {
		switch {
		case !p.opts.Linkify && t.Dashes:
			return &inlineSpecialTypo
		case !p.opts.Linkify:
			return &inlineSpecialTypoNoDash
		case p.opts.LinkifyAfterPunct && t.Dashes:
			return &inlineSpecialTypoLinkifyScheme
		case p.opts.LinkifyAfterPunct:
			return &inlineSpecialTypoNoDashLinkifyS
		case t.Dashes:
			return &inlineSpecialTypoLinkify
		}
		return &inlineSpecialTypoNoDashLinkify
	}
	if p.opts.Linkify {
		if p.opts.LinkifyAfterPunct {
			return &inlineSpecialLinkifyScheme
		}
		return &inlineSpecialLinkify
	}
	return &inlineSpecial
}

var (
	inlineSpecialLinkify       = withSpecial(inlineSpecial, " \t(")
	inlineSpecialLinkifyScheme = withSpecial(inlineSpecial, " \t(:")
)

func withSpecial(t [256]bool, chars string) [256]bool {
	for i := 0; i < len(chars); i++ {
		t[chars[i]] = true
	}
	return t
}

// extension handles the characters that only extensions treat specially.
func (ip *inlineParser) extension(c byte) bool {
	switch c {
	case ' ', '\t', '(':
		// Linkify triggers. The character itself stays text.
		ip.pos++
		ip.boundary = ip.pos
		return true
	case ':':
		return ip.schemeLinkify()
	}
	return false
}

// tryLinkify detects a www or URL autolink at the current position, which
// is a boundary. Such autolinks start a line, or follow whitespace, "*", "_",
// "~" or "(" (GFM 0.29, section 6.9).
func (ip *inlineParser) tryLinkify() bool {
	if ip.brackets.len() > 0 {
		return false
	}
	if ip.pos > 0 {
		switch ip.src[ip.pos-1] {
		case ' ', '\t', '\n', '*', '_', '~', '(':
		default:
			return false
		}
	}
	src := ip.src
	rest := src[ip.pos:]
	switch {
	case len(rest) >= 4 && rest[0] == 'w' && rest[1] == 'w' && rest[2] == 'w' && rest[3] == '.':
		if n := scanWWW(rest); n > 0 {
			ip.addLinkify(autoLinkWWW, n)
			return true
		}
	case bytes.HasPrefix(rest, []byte("http://")) || bytes.HasPrefix(rest, []byte("https://")) || bytes.HasPrefix(rest, []byte("ftp://")):
		if m := ip.linkifyURL(rest); m > 0 {
			if n := trimLinkTail(rest[:m]); n > 0 {
				ip.addLinkify(autoLinkURL, n)
				return true
			}
		}
	}
	return false
}

func (ip *inlineParser) addLinkify(typ autoLinkType, n int) {
	link := ip.p.arena.new(kindAutoLink)
	link.autoLink = typ
	link.text = ip.src[ip.pos : ip.pos+n]
	if typ == autoLinkWWW {
		start := len(ip.p.buf)
		ip.p.buf = append(append(ip.p.buf, "http://"...), link.text...)
		ip.p.arena.newExt(link).dest = ip.p.buf[start:len(ip.p.buf):len(ip.p.buf)]
	}
	ip.appendNode(link, ip.pos+n)
}

// linkifyURL returns the length of the URL at the start of b, see
// Options.LinkifyURL.
func (ip *inlineParser) linkifyURL(b []byte) int {
	if f := ip.p.opts.LinkifyURL; f != nil {
		return f(b)
	}
	n := bytes.Index(b[:min(len(b), 8)], []byte("://")) + 3
	// Like GitHub, the domain of a URL needs no period, e.g., "http://localhost".
	d := scanDomain(b[n:], false)
	if d == 0 {
		return 0
	}
	end := n + d
	for end < len(b) && b[end] != '<' && !isASCIISpace(b[end]) {
		end++
	}
	return end
}

// scanWWW returns the length of the extended www autolink at the start of b,
// which starts with "www.", or 0. The autolink is "www.", a valid domain and
// everything up to whitespace or "<", without its trailing punctuation (GFM
// 0.29, section 6.9).
func scanWWW(b []byte) int {
	// Like GitHub, "www." itself counts as a domain segment, e.g., "www.b"
	// is a link.
	end := scanDomain(b, true)
	if end < 4 {
		return 0
	}
	for end < len(b) && b[end] != '<' && !isASCIISpace(b[end]) {
		end++
	}
	return trimLinkTail(b[:end])
}

// scanDomain returns the length of the valid domain at the start of b, or 0.
// A valid domain is segments of letters, digits, "_" and "-", separated by
// periods. It has at least one period if needPeriod is true, and its last
// two segments have no "_". b runs to the end of the inline text.
func scanDomain(b []byte, needPeriod bool) int {
	n := 0
	periods := 0
	// lastUnderscore and prevUnderscore report whether the current and the
	// previous segment have an underscore.
	lastUnderscore, prevUnderscore := false, false
	for n < len(b) {
		c := b[n]
		switch {
		case isASCIIAlnum(c) || c == '-':
		case c == '_':
			// Like GitHub, an underscore that ends the text does not count.
			lastUnderscore = lastUnderscore || n < len(b)-1
		case c == '.' && n+1 < len(b):
			// Like GitHub, a period ends a segment even when the next one
			// is empty, e.g., "www..".
			periods++
			prevUnderscore, lastUnderscore = lastUnderscore, false
		default:
			goto done
		}
		n++
	}
done:
	if n == 0 || needPeriod && periods == 0 || lastUnderscore || prevUnderscore {
		return 0
	}
	return n
}

func isDomainChar(c byte) bool {
	return isASCIIAlnum(c) || c == '-' || c == '_'
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' || c == '\v'
}

// trimLinkTail returns the length of link without trailing punctuation,
// unbalanced closing parentheses and trailing entity references. Like GitHub,
// the punctuation includes quotes and ";", and an entity reference is "&",
// letters and ";".
func trimLinkTail(link []byte) int {
	n := len(link)
	opens, closes := 0, 0
	for _, c := range link {
		switch c {
		case '(':
			opens++
		case ')':
			closes++
		}
	}
	for n > 0 {
		switch c := link[n-1]; c {
		case '?', '!', '.', ',', ':', '*', '_', '~', '"', '\'':
			n--
			continue
		case ')':
			if closes > opens {
				closes--
				n--
				continue
			}
		case ';':
			i := n - 2
			for i >= 0 && isASCIILetter(link[i]) {
				i--
			}
			if i >= 0 && i < n-2 && link[i] == '&' {
				n = i
			} else {
				n--
			}
			continue
		}
		break
	}
	return n
}

var emailLocal = func() (t [256]bool) {
	for c := 0; c < 256; c++ {
		t[c] = isASCIIAlnum(byte(c))
	}
	for _, c := range []byte(".-_+") {
		t[c] = true
	}
	return t
}()

// linkifyEmails turns the email addresses in the text of n into autolinks.
// Like cmark-gfm, this happens after emphasis is resolved, on the text of
// adjacent text nodes, so an address may span delimiter runs that stayed
// text and escaped characters, e.g., "a_b@example.com". The local part is the
// longest run of letters, digits, ".", "-", "_" and "+" before the "@" (GFM
// 0.29, section 6.9). Like cmark-gfm, a domain followed by another "@" is not
// part of an address, e.g., in "a@b.c@d.e", where "b.c@d.e" is.
func (ip *inlineParser) linkifyEmails(n *node) {
	for c := n.first; c != nil; {
		switch c.kind {
		case kindText:
			last := c
			for last.next != nil && last.next.kind == kindText {
				last = last.next
			}
			next := last.next
			ip.linkifyEmailRun(c, last)
			c = next
			continue
		case kindLink, kindImage, kindAutoLink:
		default:
			if c.first != nil {
				ip.linkifyEmails(c)
			}
		}
		c = c.next
	}
}

// linkifyEmailRun linkifies the email addresses in the text of the adjacent
// text nodes from first to last.
func (ip *inlineParser) linkifyEmailRun(first, last *node) {
	text := first.text
	if first != last {
		text = ip.scratch[:0]
		for c := first; ; c = c.next {
			text = append(text, c.text...)
			if c == last {
				break
			}
		}
		ip.scratch = text[:0]
	}
	if bytes.IndexByte(text, '@') < 0 {
		return
	}
	textStart := 0
	e := len(text)
	var added bool
	for at := 0; at < e; at++ {
		if text[at] != '@' {
			continue
		}
		start := at
		for start > textStart && emailLocal[text[start-1]] {
			start--
		}
		if start == at {
			continue
		}
		n := scanEmailDomain(text[at+1:])
		if n == 0 {
			continue
		}
		end := at + 1 + n
		if end < e && text[end] == '@' {
			continue
		}
		if start > textStart {
			t := ip.p.arena.new(kindText)
			t.text = ip.p.store(text[textStart:start])
			first.insertBefore(t)
		}
		link := ip.p.arena.new(kindAutoLink)
		link.autoLink = autoLinkEmail
		link.text = ip.p.store(text[start:end])
		first.insertBefore(link)
		textStart, at, added = end, end-1, true
	}
	if !added {
		return
	}
	if textStart < e {
		t := ip.p.arena.new(kindText)
		t.text = ip.p.store(text[textStart:])
		first.insertBefore(t)
	}
	for c := first; ; {
		next := c.next
		c.unlink()
		if c == last {
			break
		}
		c = next
	}
}

// scanEmailDomain returns the length of the domain of an email address at the
// start of b, or 0. Like GitHub, the domain is letters, digits, "-" and "_",
// and periods followed by a letter or digit. It has at least one period, and
// does not end with "-", "_" or a digit, e.g., "pkg@1.2.3" is not an address.
func scanEmailDomain(b []byte) int {
	n, periods := 0, 0
	for n < len(b) {
		c := b[n]
		if isDomainChar(c) {
			n++
			continue
		}
		if c == '.' && n+1 < len(b) && isASCIIAlnum(b[n+1]) {
			periods++
			n++
			continue
		}
		break
	}
	if periods == 0 {
		return 0
	}
	if c := b[n-1]; c == '-' || c == '_' || isASCIIDigit(c) {
		return 0
	}
	return n
}

// schemeLinkify detects URLs whose scheme directly follows a character other
// than an ASCII letter or digit, e.g., "see:http://example.com". The current
// position is at the colon.
func (ip *inlineParser) schemeLinkify() bool {
	if ip.brackets.len() > 0 || !bytes.HasPrefix(ip.src[ip.pos:], []byte("://")) {
		return false
	}
	pending := ip.src[ip.textStart:ip.pos]
	var scheme int
	for _, s := range [...]string{"https", "http", "ftp"} {
		if bytes.HasSuffix(pending, []byte(s)) {
			scheme = len(s)
			break
		}
	}
	if scheme == 0 {
		return false
	}
	if i := len(pending) - scheme - 1; i >= 0 && isASCIIAlnum(pending[i]) {
		return false
	}
	start := ip.pos - scheme
	m := ip.linkifyURL(ip.src[start:])
	if m == 0 {
		return false
	}
	n := trimLinkTail(ip.src[start : start+m])
	if n == 0 {
		return false
	}
	ip.pos = start
	ip.addLinkify(autoLinkURL, n)
	return true
}

// taskCheckbox parses a task list item marker at the start of the first block
// of a list item, see parser.taskItem.
func (ip *inlineParser) taskCheckbox() {
	block := ip.parent
	if block.parent == nil || block.parent.kind != kindListItem || block.parent.first != block {
		return
	}
	task, checked := ip.p.taskItem(block.parent)
	if !task {
		return
	}
	cb := ip.p.arena.new(kindTaskCheckbox)
	cb.checked = checked
	ip.parent.appendChild(cb)
	src := ip.src
	next := 3
	for next < len(src) && (src[next] == ' ' || src[next] == '\t' || src[next] == '\n' || src[next] == '\r') {
		next++
	}
	ip.pos, ip.textStart, ip.boundary = next, next, next
}

// walkLinks applies Options.RelativeLinks and TextAutoLinksInRawAnchors to the
// inline contents of n: relative link destinations are joined with the link base, and autolinks
// inside raw HTML anchors become text.
func (p *parser) walkLinks(n *node, inRawAnchor *bool) {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindLink:
			if p.opts.RelativeLinks {
				// Relative destinations are joined before escapes and entities
				// are resolved.
				dest := c.ext.rawDest
				if len(dest) > 0 && len(dest) == len(p.joinedRaw) && &dest[0] == &p.joinedRaw[0] {
					// Uses of a link reference share its destination, which is
					// joined once.
					c.ext.dest = p.joined
				} else if len(dest) > 0 && dest[0] != '#' && (p.opts.IsLink == nil || !p.opts.IsLink(dest)) {
					c.ext.dest = p.unescapeDest(p.store([]byte(joinLink(p.opts.LinkBase, string(dest)))))
					p.joinedRaw, p.joined = dest, c.ext.dest
				}
			}
		case kindRawHTML:
			if !p.opts.TextAutoLinksInRawAnchors {
				break
			}
			if rawAnchorOpen.Match(c.text) {
				*inRawAnchor = true
			} else if rawAnchorClose.Match(c.text) {
				*inRawAnchor = false
			}
		case kindAutoLink:
			if *inRawAnchor {
				c.plain = true
			}
		default:
			if c.kind < kindText {
				*inRawAnchor = false
			}
		}
		if c.first != nil {
			p.walkLinks(c, inRawAnchor)
		}
	}
}

func (ip *inlineParser) unmatchedBackticks(end int) {
	ip.boundary = end
}

// joinLink joins the relative link dest to base, which is either a path or an
// absolute URL such as "https://example.com/user/repo". The scheme and host
// of an absolute URL are kept as they are.
func joinLink(base, dest string) string {
	i := strings.Index(base, "://")
	if i < 0 {
		return path.Join(base, dest)
	}
	pathStart := i + 3 + strings.IndexByte(base[i+3:]+"/", '/')
	return base[:pathStart] + path.Join("/", base[pathStart:], dest)
}
