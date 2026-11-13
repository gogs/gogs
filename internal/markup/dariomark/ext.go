package dariomark

import (
	"bytes"
	"path"
	"regexp"
	"strings"
)

// wwwPattern matches an extended "www." autolink.
var wwwPattern = regexp.MustCompile(`^www\.[-a-zA-Z0-9@:%._+~#=]{1,256}\.[a-z]+(?:[/#?][-a-zA-Z0-9@:%_+.~#!?&/=();,'"^{}\[\]>` + "`" + `]*)?`)

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
	inlineSpecialLinkify       = withSpecial(inlineSpecial, " (")
	inlineSpecialLinkifyScheme = withSpecial(inlineSpecial, " (:")
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
	case ' ', '(':
		// Linkify triggers. The character itself stays text.
		ip.pos++
		ip.boundary = ip.pos
		return true
	case ':':
		return ip.schemeLinkify()
	}
	return false
}

// tryLinkify detects a link at the current position, which is a boundary.
func (ip *inlineParser) tryLinkify() bool {
	if len(ip.brackets) > 0 {
		return false
	}
	src := ip.src
	rest := src[ip.pos:]
	switch {
	case len(rest) >= 4 && rest[0] == 'w' && rest[1] == 'w' && rest[2] == 'w' && rest[3] == '.':
		if m := wwwPattern.FindIndex(rest); m != nil {
			if n := trimLinkTail(rest[:m[1]]); n > 0 {
				ip.addLinkify(autoLinkWWW, n)
				return true
			}
		}
	case bytes.HasPrefix(rest, []byte("http://")) || bytes.HasPrefix(rest, []byte("https://")) || bytes.HasPrefix(rest, []byte("ftp://")):
		if ip.p.opts.LinkifyURL != nil {
			if m := ip.p.opts.LinkifyURL(rest); m > 0 {
				if n := trimLinkTail(rest[:m]); n > 0 {
					ip.addLinkify(autoLinkURL, n)
					return true
				}
			}
		}
	}
	if n := ip.scanEmail(); n > 0 {
		ip.addLinkify(autoLinkEmail, n)
		return true
	}
	return false
}

func (ip *inlineParser) addLinkify(typ autoLinkType, n int) {
	link := ip.p.arena.new(kindAutoLink)
	link.autoLink = typ
	link.text = ip.src[ip.pos : ip.pos+n]
	link.dest = link.text
	if typ == autoLinkWWW {
		start := len(ip.p.buf)
		ip.p.buf = append(append(ip.p.buf, "http://"...), link.text...)
		link.dest = ip.p.buf[start:len(ip.p.buf):len(ip.p.buf)]
	}
	ip.mergeLone()
	if c := ip.src[max(ip.pos-1, 0)]; ip.pos > 0 && ip.lineStart == ip.pos-1 && ip.textStart == ip.lineStart && (c == ' ' || c == '(') {
		// The character that triggers linkify is not text for
		// lineStartBackslash.
		ip.textStart = ip.pos
		ip.lineStart = ip.pos
		ip.appendText(ip.src[ip.pos-1 : ip.pos])
	}
	ip.appendNode(link, ip.pos+n)
}

// trimLinkTail returns the length of link without trailing punctuation,
// unbalanced closing parentheses and trailing entity references.
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
		case '?', '!', '.', ',', ':', '*', '_', '~':
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
			for i >= 0 && isASCIIAlnum(link[i]) {
				i--
			}
			if i >= 0 && i < n-2 && link[i] == '&' {
				n = i
				continue
			}
		}
		break
	}
	return n
}

var emailLocal = func() (t [256]bool) {
	for c := 0; c < 256; c++ {
		t[c] = isASCIIAlnum(byte(c))
	}
	for _, c := range []byte(".!#$%&'*+/=?^_`{|}~-") {
		t[c] = true
	}
	return t
}()

// scanEmail returns the length of the email address at the current
// position, or 0.
func (ip *inlineParser) scanEmail() int {
	src := ip.src
	pos := ip.pos
	if !isASCIIAlnum(src[pos]) {
		return 0
	}
	if pos < ip.emailFailEnd {
		// An earlier scan from before pos covered this local part and failed.
		return 0
	}
	i := pos
	for i < len(src) && emailLocal[src[i]] {
		i++
	}
	if i >= len(src) || src[i] != '@' {
		ip.emailFailEnd = i
		return 0
	}
	n := scanEmailDomain(src[i+1:])
	if n == 0 {
		ip.emailFailEnd = i
		return 0
	}
	return i + 1 + n - pos
}

// scanEmailDomain returns the length of the domain of an email address at the
// start of b, or 0.
func scanEmailDomain(b []byte) int {
	// A dot that is not followed by a letter or digit ends the domain.
	n := 0
	for n < len(b) && (isASCIIAlnum(b[n]) || b[n] == '-' || b[n] == '.' && n+1 < len(b) && isASCIIAlnum(b[n+1])) {
		n++
	}
	if n < len(b) && b[n] == '_' {
		return 0
	}
	labels := 0
	for start := 0; start < n; {
		e := bytes.IndexByte(b[start:n], '.')
		if e < 0 {
			e = n
		} else {
			e += start
		}
		label := b[start:e]
		if len(label) == 0 || label[0] == '-' || label[len(label)-1] == '-' {
			return 0
		}
		labels++
		start = e + 1
	}
	if labels < 2 {
		return 0
	}
	return n
}

// schemeLinkify detects URLs whose scheme directly follows a character other
// than an ASCII letter or digit, e.g., "see:http://example.com". The current
// position is at the colon.
func (ip *inlineParser) schemeLinkify() bool {
	if len(ip.brackets) > 0 || ip.p.opts.LinkifyURL == nil || !bytes.HasPrefix(ip.src[ip.pos:], []byte("://")) {
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
	m := ip.p.opts.LinkifyURL(ip.src[start:])
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
// of a list item.
func (ip *inlineParser) taskCheckbox() {
	src := ip.src
	if len(src) < 3 || src[0] != '[' || src[2] != ']' || (src[1] != ' ' && src[1] != '\t' && src[1] != 'x' && src[1] != 'X') {
		return
	}
	block := ip.parent
	if block.parent == nil || block.parent.kind != kindListItem || block.parent.first != block {
		return
	}
	cb := ip.p.arena.new(kindTaskCheckbox)
	cb.checked = src[1] == 'x' || src[1] == 'X'
	ip.parent.appendChild(cb)
	next := 3
	for next < len(src) && (src[next] == ' ' || src[next] == '\t' || src[next] == '\n' || src[next] == '\r') {
		next++
	}
	ip.pos, ip.textStart, ip.boundary = next, next, next
}

// postInline applies the Gogs link rules after the inline contents of
// all blocks have been parsed: relative link destinations are joined with
// the link base, and autolinks inside raw HTML anchors become text.
func (p *parser) postInline(n *node) {
	inRawAnchor := false
	p.walkLinks(n, &inRawAnchor)
}

func (p *parser) walkLinks(n *node, inRawAnchor *bool) {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindLink:
			if p.opts.RelativeLinks {
				// Relative destinations are joined before escapes and entities
				// are resolved.
				dest := c.rawDest
				if len(dest) > 0 && dest[0] != '#' && (p.opts.IsLink == nil || !p.opts.IsLink(dest)) {
					c.dest = p.unescapeDest(p.store([]byte(joinLink(p.opts.LinkBase, string(dest)))))
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
