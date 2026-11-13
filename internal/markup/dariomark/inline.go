package dariomark

import (
	"bytes"
	"unicode/utf8"
)

type delimiter struct {
	node              *node
	char              byte
	count, origCount  int
	canOpen, canClose bool
	prev, next        int
}

type bracket struct {
	node        *node
	pos         int // offset after the opening bracket
	image       bool
	active      bool
	delimBottom int // the top of the delimiter stack when the bracket was pushed
}

type backtickRuns struct {
	built bool
	// positions of maximal backtick runs by length, and the next unused index.
	byLen map[int][]int
	next  map[int]int
}

type inlineParser struct {
	p      *parser
	src    []byte
	pos    int
	parent *node
	// textStart is the start of the text that has not been added as a node.
	textStart int

	delims    []delimiter
	delimTop  int
	brackets  []bracket
	html      htmlScanner
	backticks backtickRuns
	inCell    bool
	special   *[256]bool
	linkify   bool
	// boundary is the position where a linkified link may start, e.g., after
	// an inline node.
	boundary     int
	emailFailEnd int
	// line is the number of line endings in src before linePos.
	line, linePos int
	// doubleOpen and singleOpen count the opening quotes converted by the
	// typographer in the document so far.
	doubleOpen, singleOpen int

	// lineStart is the start of the current line after a line ending.
	// loneEOL reports whether the previous line ended with an unpaired
	// backslash, which pairs with a backslash that starts the line.
	lineStart int
	loneEOL   bool
	// lineStartPaired is the position of the last backslash at a line start
	// that was paired with an unpaired backslash at the end of the line
	// before.
	lineStartPaired int
	lone            []loneBackslash

	cells [][]byte
}

// loneBackslash is a text node holding a backslash that stays literal because
// it is paired across a line ending, see inlineParser.lineStartBackslash.
type loneBackslash struct {
	node *node
	// char is the character after the backslash.
	char byte
	// failedBracket reports whether a bracket after the backslash was closed
	// without forming a link.
	failedBracket bool
	// pending is the start of the text after the backslash, or -1. merged
	// reports whether that text was ended by inline syntax rather than by a
	// line ending.
	pending int
	merged  bool
	// triggerAfter reports whether inline syntax may start later on the line.
	triggerAfter bool
}

func (ip *inlineParser) reset() {
	ip.p = nil
	ip.src = nil
	ip.parent = nil
	ip.delims = ip.delims[:0]
	ip.brackets = ip.brackets[:0]
	ip.doubleOpen, ip.singleOpen = 0, 0
	clear(ip.cells)
	ip.cells = ip.cells[:0]
}

var inlineSpecial = func() (t [256]bool) {
	for _, c := range []byte("\n\\`*_[]!<&~") {
		t[c] = true
	}
	return t
}()

// processInlines parses the inline contents of all leaf blocks under n.
func (p *parser) processInlines(n *node) {
	for c := n.first; c != nil; c = c.next {
		switch c.kind {
		case kindParagraph:
			p.parseInlines(c, c.text, false)
		case kindHeading:
			text := c.text
			if !c.setext && c.segEnd > c.segStart {
				s := p.segs[c.segStart]
				text = p.src[s.start:s.end]
			}
			p.parseInlines(c, text, false)
		case kindTableCell:
			p.parseInlines(c, c.text, true)
		case kindFootnoteDef:
			// All definitions are parsed together, in the order they closed,
			// where the first one appears. This decides the footnote numbering.
			// When the first to close is nested in another definition, none of
			// them are parsed, which matches goldmark.
			if !p.footnotes.inlined {
				p.footnotes.inlined = true
				if !p.footnotes.nestedFirst() {
					for _, d := range p.footnotes.defs {
						p.processInlines(d)
					}
				}
			}
		default:
			if c.first != nil {
				p.processInlines(c)
			}
		}
	}
}

func (p *parser) parseInlines(parent *node, src []byte, inCell bool) {
	ip := &p.inl
	ip.p = p
	ip.src = src
	ip.pos = 0
	ip.textStart = 0
	ip.parent = parent
	ip.inCell = inCell
	ip.delims = ip.delims[:0]
	ip.delimTop = -1
	ip.brackets = ip.brackets[:0]
	ip.html.reset()
	ip.backticks.built = false
	ip.special = p.specialChars()
	ip.linkify = p.opts.Linkify
	ip.boundary = 0
	ip.emailFailEnd = 0
	ip.line, ip.linePos = 0, 0
	ip.lineStart, ip.loneEOL, ip.lineStartPaired = -1, false, -1
	ip.lone = ip.lone[:0]
	if p.opts.TaskLists {
		ip.taskCheckbox()
	}
	ip.parse()
}

func (ip *inlineParser) parse() {
	src := ip.src
	opts := ip.p.opts
	special := ip.special
	for ip.pos < len(src) {
		c := src[ip.pos]
		if ip.pos == ip.boundary && ip.linkify && isASCIIAlnum(c) && ip.tryLinkify() {
			continue
		}
		if !special[c] {
			ip.pos++
			continue
		}
		start := ip.pos
		handled := false
		switch c {
		case '\n':
			ip.lineEnding()
			handled = true
		case '\\':
			if ip.loneEOL && ip.pos == ip.lineStart {
				handled = ip.lineStartBackslash()
			} else {
				handled = ip.backslash()
			}
		case '`':
			ip.codeSpan()
			handled = true
		case '*', '_':
			handled = ip.delimRun(c)
		case '~':
			if opts.Strikethrough {
				handled = ip.delimRun(c)
			}
		case '[':
			ip.openBracket(false)
			handled = true
		case '!':
			if ip.pos+1 < len(src) && src[ip.pos+1] == '[' {
				ip.openBracket(true)
				handled = true
			}
		case ']':
			handled = ip.closeBracket()
		case '<':
			handled = ip.angle()
		case '&':
			handled = ip.entity()
		}
		if !handled && opts.Typographer.Enabled {
			handled = ip.typographer(c)
		}
		if !handled {
			handled = ip.extension(c)
		}
		if !handled && ip.pos == start {
			ip.pos++
		}
	}
	ip.flushText(len(src))
	ip.processEmphasis(-1)
	for _, l := range ip.lone {
		next := l.node.next
		leftover := l.pending < 0 && (l.char == '*' || l.char == '_' || l.char == '~') && next != nil && next.kind == kindText && len(next.text) > 0
		staysText := l.pending >= 0 && next != nil && next.kind == kindText && len(next.text) > 0 && &next.text[0] == &src[l.pending]
		if leftover || l.failedBracket || l.merged || l.triggerAfter && staysText {
			// The backslash escapes the delimiter or bracket that stayed text.
			l.node.unlink()
		}
	}
}

// flushText adds the pending text up to end as a text node.
func (ip *inlineParser) flushText(end int) {
	if end > ip.textStart {
		ip.appendText(ip.src[ip.textStart:end])
	}
	ip.textStart = end
}

func (ip *inlineParser) appendText(b []byte) *node {
	n := ip.p.arena.new(kindText)
	n.text = b
	ip.parent.appendChild(n)
	return n
}

// appendNode flushes the pending text up to the current position, appends n
// and continues the pending text at next.
func (ip *inlineParser) appendNode(n *node, next int) {
	if ip.pos == ip.lineStart && ip.textStart == ip.pos {
		// Syntax without text in between keeps the line start for
		// lineStartBackslash.
		ip.lineStart = next
	}
	ip.flushText(ip.pos)
	ip.parent.appendChild(n)
	ip.pos = next
	ip.textStart = next
	ip.boundary = next
}

func (ip *inlineParser) lineEnding() {
	src := ip.src
	end := ip.pos
	if end > ip.textStart && src[end-1] == '\r' {
		end--
	}
	spaces := 0
	for end-spaces > ip.textStart && src[end-spaces-1] == ' ' {
		spaces++
	}
	k := kindSoftBreak
	if spaces >= 2 {
		k = kindHardBreak
	}
	if ip.linkify {
		// With linkify, at most three whitespace characters are removed before
		// a hard line break, and one before a soft line break.
		limit := 1
		if k == kindHardBreak {
			limit = 3
		}
		spaces = 0
		for spaces < limit && end-spaces > ip.textStart && (src[end-spaces-1] == ' ' || src[end-spaces-1] == '\t') {
			spaces++
		}
	}
	ip.flushText(end - spaces)
	n := ip.p.arena.new(k)
	ip.parent.appendChild(n)

	runEnd := end
	for runEnd > 0 && src[runEnd-1] == ' ' {
		runEnd--
	}
	runStart := runEnd
	for runStart > 0 && src[runStart-1] == '\\' {
		runStart--
	}
	trailing := end - runEnd
	run := runEnd - runStart
	// A line of only inline syntax keeps an unpaired backslash before it.
	if runStart == ip.lineStartPaired {
		run--
	}
	ip.loneEOL = (trailing == 0 || trailing == 2) && (ip.loneEOL && ip.lineStart >= runEnd || run%2 == 1)

	next := ip.pos + 1
	for next < len(src) && (src[next] == ' ' || src[next] == '\t') {
		next++
	}
	ip.pos, ip.textStart, ip.boundary = next, next, next
	ip.lineStart = next
}

// lineStartBackslash handles backslashes at the start of a line after a line
// that ended with an unpaired backslash. The first backslash pairs with that
// one, so it stays literal and the parity of the escapes that follow flips
// for the characters that start inline syntax.
func (ip *inlineParser) lineStartBackslash() bool {
	src := ip.src
	pos := ip.pos
	n := 0
	for pos+n < len(src) && src[pos+n] == '\\' {
		n++
	}
	i := pos + n
	ip.lineStartPaired = pos
	if i >= len(src) || !ip.escapeFlipped(src[i]) {
		return ip.backslash()
	}
	ip.flushText(pos)
	if n%2 == 0 {
		ip.appendText(src[pos+n/2 : i+1])
		ip.pos, ip.textStart = i+1, i+1
		return true
	}
	if n > 1 {
		ip.appendText(src[pos : pos+(n-1)/2])
	}
	l := loneBackslash{node: ip.appendText(src[i-1 : i]), char: src[i], pending: i}
	switch {
	case l.char == '*', l.char == '_', l.char == '~', l.char == '[', l.char == '`',
		l.char == '!' && i+1 < len(src) && src[i+1] == '[':
		l.pending = -1
	default:
		for j := i + 1; j < len(src) && src[j] != '\n'; j++ {
			c := src[j]
			if c == '\\' && j+1 < len(src) && isASCIIPunct(src[j+1]) {
				j++
				continue
			}
			if c == ' ' && ip.linkify && j+2 < len(src) && src[j+1] == ' ' && src[j+2] == '\n' {
				// Two spaces before a hard line break.
				break
			}
			if ip.escapeFlipped(c) || (c == ' ' || c == '\t') && ip.linkify {
				l.triggerAfter = true
				break
			}
		}
	}
	ip.lone = append(ip.lone, l)
	ip.pos, ip.textStart = i, i
	return true
}

// mergeLone records that the pending text, if it directly follows a literal
// backslash from lineStartBackslash, is ended by inline syntax. The backslash
// then escapes the first character of that text.
func (ip *inlineParser) mergeLone() {
	if n := len(ip.lone); n > 0 && ip.lone[n-1].pending == ip.textStart && ip.pos > ip.textStart {
		ip.lone[n-1].merged = true
	}
}

// escapeFlipped reports whether a backslash escape of c at the start of a line
// is affected by an unpaired backslash at the end of the line before.
func (ip *inlineParser) escapeFlipped(c byte) bool {
	opts := ip.p.opts
	switch c {
	case '!', '<', '[', ']', '`', '*', '_':
		return true
	case '~':
		return opts.Strikethrough
	case '(':
		return opts.Linkify
	case ':':
		return opts.Linkify && opts.LinkifyAfterPunct
	case '"', '\'', ',', '-', '.', '>':
		return opts.Typographer.Enabled
	}
	return false
}

func isASCIIPunct(c byte) bool {
	return c >= '!' && c <= '/' || c >= ':' && c <= '@' || c >= '[' && c <= '`' || c >= '{' && c <= '~'
}

func (ip *inlineParser) backslash() bool {
	src := ip.src
	i := ip.pos + 1
	// A backslash after an escaped backslash does not escape a line ending.
	afterBackslash := ip.pos > 0 && src[ip.pos-1] == '\\'
	if !afterBackslash && (i < len(src) && src[i] == '\n' || i+1 < len(src) && src[i] == '\r' && src[i+1] == '\n') {
		ip.loneEOL = ip.pos == ip.lineStartPaired
		ip.flushText(ip.pos)
		ip.parent.appendChild(ip.p.arena.new(kindHardBreak))
		if src[i] == '\r' {
			i++
		}
		next := i + 1
		for next < len(src) && (src[next] == ' ' || src[next] == '\t') {
			next++
		}
		ip.pos, ip.textStart, ip.boundary = next, next, next
		ip.lineStart = next
		return true
	}
	if i < len(src) && isASCIIPunct(src[i]) {
		ip.flushText(ip.pos)
		// The escaped character starts the next text node.
		ip.textStart = i
		ip.pos = i + 1
		return true
	}
	if ip.linkify && i < len(src) && src[i] == ' ' && !isBlankLineTail(src[i:]) {
		// An unpaired backslash before a space pairs like one before a line
		// ending, see lineStartBackslash.
		ip.flushText(i)
		ip.pos = i
		ip.loneEOL = true
		ip.lineStart = i
		ip.lineStartPaired = -1
		return true
	}
	return false
}

// isBlankLineTail reports whether b has only spaces and tabs before the end of
// its first line.
func isBlankLineTail(b []byte) bool {
	for _, c := range b {
		switch c {
		case ' ', '\t', '\r':
		case '\n':
			return true
		default:
			return false
		}
	}
	return true
}

func (ip *inlineParser) buildBackticks() {
	b := &ip.backticks
	b.built = true
	if b.byLen == nil {
		b.byLen = make(map[int][]int)
		b.next = make(map[int]int)
	}
	for k, v := range b.byLen {
		b.byLen[k] = v[:0]
	}
	clear(b.next)
	src := ip.src
	for i := 0; i < len(src); {
		if src[i] != '`' {
			j := bytes.IndexByte(src[i:], '`')
			if j < 0 {
				break
			}
			i += j
			continue
		}
		j := i
		for j < len(src) && src[j] == '`' {
			j++
		}
		b.byLen[j-i] = append(b.byLen[j-i], i)
		i = j
	}
}

// findBackticks returns the start of the first maximal run of n backticks at
// or after pos, or -1.
func (ip *inlineParser) findBackticks(n, pos int) int {
	b := &ip.backticks
	if !b.built {
		ip.buildBackticks()
	}
	list := b.byLen[n]
	k := b.next[n]
	for k < len(list) && list[k] < pos {
		k++
	}
	b.next[n] = k
	if k < len(list) {
		return list[k]
	}
	return -1
}

func (ip *inlineParser) codeSpan() {
	src := ip.src
	start := ip.pos
	end := start
	for end < len(src) && src[end] == '`' {
		end++
	}
	n := end - start
	closer := ip.findBackticks(n, end)
	if closer < 0 {
		// The whole run is literal text.
		if ip.pos == ip.lineStart && ip.textStart == ip.pos {
			t := ip.p.arena.new(kindText)
			t.text = src[start:end]
			ip.appendNode(t, end)
			return
		}
		ip.pos = end
		ip.unmatchedBackticks(end)
		return
	}
	code := ip.p.arena.new(kindCodeSpan)
	code.text, code.title = ip.codeContent(src[end:closer])
	ip.appendNode(code, closer+n)
}

// codeContent returns the content of a code span, and the same content with
// its line endings kept, which goldmark uses in image descriptions.
func (ip *inlineParser) codeContent(b []byte) (content, plain []byte) {
	plain = b
	if bytes.IndexByte(b, '\n') >= 0 || (ip.inCell && bytes.Contains(b, []byte(`\|`))) {
		out := ip.p.buf[len(ip.p.buf):]
		for i := 0; i < len(b); i++ {
			if ip.inCell && b[i] == '\\' && i+1 < len(b) && b[i+1] == '|' {
				continue
			}
			out = append(out, b[i])
		}
		ip.p.buf = append(ip.p.buf, out...)
		plain = ip.p.buf[len(ip.p.buf)-len(out):]
		b = bytes.ReplaceAll(plain, []byte("\n"), []byte(" "))
	}
	if len(b) >= 2 && b[0] == ' ' && b[len(b)-1] == ' ' {
		// Unlike CommonMark, goldmark also keeps the spaces around content
		// of only tabs and line endings.
		for _, c := range b {
			if c != ' ' && c != '\t' && c != '\r' && c != '\n' {
				return b[1 : len(b)-1], plain[1 : len(plain)-1]
			}
		}
	}
	return b, plain
}

func (ip *inlineParser) angle() bool {
	src := ip.src
	if n, ok := scanAutolinkURI(src[ip.pos:]); ok {
		link := ip.p.arena.new(kindAutoLink)
		link.text = src[ip.pos+1 : ip.pos+n-1]
		link.dest = link.text
		link.autoLink = autoLinkURL
		link.angle = true
		ip.appendNode(link, ip.pos+n)
		return true
	}
	if n, ok := scanAutolinkEmail(src[ip.pos:]); ok {
		link := ip.p.arena.new(kindAutoLink)
		link.text = src[ip.pos+1 : ip.pos+n-1]
		link.dest = link.text
		link.autoLink = autoLinkEmail
		link.angle = true
		ip.appendNode(link, ip.pos+n)
		return true
	}
	if n := ip.html.scan(src, ip.pos); n > 0 {
		h := ip.p.arena.new(kindRawHTML)
		h.text = src[ip.pos : ip.pos+n]
		ip.p.hasRawHTML = true
		ip.appendNode(h, ip.pos+n)
		return true
	}
	return false
}

func scanAutolinkURI(b []byte) (int, bool) {
	if len(b) < 4 || b[0] != '<' || !isASCIILetter(b[1]) {
		return 0, false
	}
	i := 2
	for i < len(b) && i-1 <= 32 && (isASCIIAlnum(b[i]) || b[i] == '+' || b[i] == '.' || b[i] == '-') {
		i++
	}
	if i-1 < 2 || i-1 > 32 || i >= len(b) || b[i] != ':' {
		return 0, false
	}
	for i++; i < len(b); i++ {
		c := b[i]
		if c == '>' {
			return i + 1, true
		}
		if c <= ' ' || c == '<' || c == 0x7f {
			return 0, false
		}
	}
	return 0, false
}

func isEmailLocal(c byte) bool {
	return isASCIIAlnum(c) || bytes.IndexByte([]byte(".!#$%&'*+/=?^_`{|}~-"), c) >= 0
}

func scanAutolinkEmail(b []byte) (int, bool) {
	if len(b) < 2 || b[0] != '<' {
		return 0, false
	}
	i := 1
	for i < len(b) && isEmailLocal(b[i]) {
		i++
	}
	if i == 1 || i >= len(b) || b[i] != '@' {
		return 0, false
	}
	i++
	for {
		// A label of letters, digits and hyphens that neither starts nor ends
		// with a hyphen.
		s := i
		for i < len(b) && i-s < 63 && (isASCIIAlnum(b[i]) || b[i] == '-') {
			i++
		}
		if i == s || b[s] == '-' || b[i-1] == '-' {
			return 0, false
		}
		if i >= len(b) {
			return 0, false
		}
		switch b[i] {
		case '>':
			return i + 1, true
		case '.':
			i++
		default:
			return 0, false
		}
	}
}

func (ip *inlineParser) entity() bool {
	r, n := decodeEntity(ip.src[ip.pos:])
	if n == 0 {
		return false
	}
	t := ip.p.arena.new(kindText)
	t.text = r
	lineStart := ip.lineStart
	ip.appendNode(t, ip.pos+n)
	// Entities are text for lineStartBackslash.
	ip.lineStart = lineStart
	// Entities do not end a text run for linkify.
	ip.boundary = -1
	return true
}

var replacementChar = []byte("\uFFFD")

// decodeEntity decodes the entity or numeric character reference at the
// start of b and returns its replacement and length.
func decodeEntity(b []byte) ([]byte, int) {
	return decodeEntityOctal(b, false)
}

// decodeEntityOctal decodes like decodeEntity. If octal is true, decimal
// numeric character references with a leading zero are octal, and invalid
// octal numbers are replaced with U+FFFD.
func decodeEntityOctal(b []byte, octal bool) ([]byte, int) {
	if len(b) < 3 || b[0] != '&' {
		return nil, 0
	}
	if b[1] == '#' {
		i := 2
		hex := false
		if i < len(b) && (b[i] == 'x' || b[i] == 'X') {
			hex = true
			i++
		}
		s := i
		r := 0
		for i < len(b) {
			c := b[i]
			var d int
			switch {
			case isASCIIDigit(c):
				d = int(c - '0')
			case hex && 'a' <= c && c <= 'f':
				d = int(c-'a') + 10
			case hex && 'A' <= c && c <= 'F':
				d = int(c-'A') + 10
			default:
				d = -1
			}
			if d < 0 {
				break
			}
			if hex {
				r = r*16 + d
			} else {
				r = r*10 + d
			}
			i++
			if hex && i-s > 6 || !hex && i-s > 7 {
				return nil, 0
			}
		}
		if i == s || i >= len(b) || b[i] != ';' {
			return nil, 0
		}
		if octal && !hex && b[s] == '0' {
			r = 0
			for _, c := range b[s:i] {
				if c > '7' {
					r = 0
					break
				}
				r = r*8 + int(c-'0')
			}
		}
		if r == 0 || r > utf8.MaxRune || (r >= 0xD800 && r <= 0xDFFF) {
			return replacementChar, i + 1
		}
		return utf8.AppendRune(nil, rune(r)), i + 1
	}
	i := 1
	for i < len(b) && i <= 32 && isASCIIAlnum(b[i]) {
		i++
	}
	if i == 1 || i >= len(b) || b[i] != ';' {
		return nil, 0
	}
	if s, ok := entities[string(b[1:i])]; ok {
		return []byte(s), i + 1
	}
	return nil, 0
}

// unescapeString resolves backslash escapes and entities in b.
// unescapeDest resolves backslash escapes and then entities in a link
// destination, see decodeEntityOctal.
func (p *parser) unescapeDest(b []byte) []byte {
	if bytes.IndexByte(b, '\\') >= 0 {
		start := len(p.buf)
		for i := 0; i < len(b); i++ {
			if b[i] == '\\' && i+1 < len(b) && isASCIIPunct(b[i+1]) {
				i++
			}
			p.buf = append(p.buf, b[i])
		}
		b = p.buf[start:len(p.buf):len(p.buf)]
	}
	if bytes.IndexByte(b, '&') < 0 {
		return b
	}
	start := len(p.buf)
	for i := 0; i < len(b); {
		if b[i] == '&' {
			if r, n := decodeEntityOctal(b[i:], true); n > 0 {
				p.buf = append(p.buf, r...)
				i += n
				continue
			}
		}
		p.buf = append(p.buf, b[i])
		i++
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}

func (p *parser) unescapeString(b []byte) []byte {
	if bytes.IndexByte(b, '\\') < 0 && bytes.IndexByte(b, '&') < 0 {
		return b
	}
	start := len(p.buf)
	for i := 0; i < len(b); {
		c := b[i]
		if c == '\\' && i+1 < len(b) && isASCIIPunct(b[i+1]) {
			p.buf = append(p.buf, b[i+1])
			i += 2
			continue
		}
		if c == '&' {
			if r, n := decodeEntity(b[i:]); n > 0 {
				p.buf = append(p.buf, r...)
				i += n
				continue
			}
		}
		p.buf = append(p.buf, c)
		i++
	}
	return p.buf[start:len(p.buf):len(p.buf)]
}
