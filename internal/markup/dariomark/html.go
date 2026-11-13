package dariomark

import (
	"bytes"
)

var htmlBlock1Tags = []string{"pre", "script", "style", "textarea"}

var htmlBlock6Tags = map[string]bool{
	"address": true, "article": true, "aside": true, "base": true, "basefont": true, "blockquote": true,
	"body": true, "caption": true, "center": true, "col": true, "colgroup": true, "dd": true, "details": true,
	"dialog": true, "dir": true, "div": true, "dl": true, "dt": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true, "frameset": true, "h1": true, "h2": true,
	"h3": true, "h4": true, "h5": true, "h6": true, "head": true, "header": true, "hr": true, "html": true,
	"iframe": true, "legend": true, "li": true, "link": true, "main": true, "menu": true, "menuitem": true,
	"nav": true, "noframes": true, "ol": true, "optgroup": true, "option": true, "p": true, "param": true,
	"search": true, "section": true, "summary": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "title": true, "tr": true, "track": true, "ul": true,
}

func isASCIILetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isASCIIDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

func isASCIIAlnum(c byte) bool {
	return isASCIILetter(c) || isASCIIDigit(c)
}

func toLowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func hasPrefixFold(b []byte, prefix string) bool {
	if len(b) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if toLowerASCII(b[i]) != prefix[i] {
			return false
		}
	}
	return true
}

func containsFold(b []byte, s string) bool {
	for i := 0; i+len(s) <= len(b); i++ {
		if hasPrefixFold(b[i:], s) {
			return true
		}
	}
	return false
}

// htmlBlockStart returns the type of HTML block that line starts, or 0.
func htmlBlockStart(line []byte, interruptsParagraph bool) int {
	if len(line) < 2 || line[0] != '<' {
		return 0
	}
	rest := line[1:]
	for _, tag := range htmlBlock1Tags {
		if hasPrefixFold(rest, tag) {
			after := rest[len(tag):]
			if len(after) == 0 || after[0] == ' ' || after[0] == '\t' || after[0] == '>' {
				return 1
			}
		}
	}
	if bytes.HasPrefix(rest, []byte("!--")) {
		return 2
	}
	if rest[0] == '?' {
		return 3
	}
	// Like GitHub, declarations start with an uppercase letter, as in
	// CommonMark 0.29.
	if rest[0] == '!' && len(rest) > 1 && 'A' <= rest[1] && rest[1] <= 'Z' {
		return 4
	}
	if bytes.HasPrefix(rest, []byte("![CDATA[")) {
		return 5
	}
	name := rest
	if name[0] == '/' {
		name = name[1:]
	}
	i := 0
	for i < len(name) && isASCIIAlnum(name[i]) {
		i++
	}
	if i > 0 && i <= 10 {
		var lower [10]byte
		for j := 0; j < i; j++ {
			lower[j] = toLowerASCII(name[j])
		}
		if htmlBlock6Tags[string(lower[:i])] {
			after := name[i:]
			if len(after) == 0 || after[0] == ' ' || after[0] == '\t' || after[0] == '>' ||
				(after[0] == '/' && len(after) > 1 && after[1] == '>') {
				return 6
			}
		}
	}
	if interruptsParagraph {
		return 0
	}
	var n int
	tagStart := 1
	if rest[0] == '/' {
		n = scanClosingTag(line)
		tagStart = 2
	} else {
		n = scanOpenTag(line)
	}
	if n > 0 {
		j := tagStart
		for j < len(line) && (isASCIIAlnum(line[j]) || line[j] == '-') {
			j++
		}
		tag := line[tagStart:j]
		for _, t := range htmlBlock1Tags {
			if len(tag) == len(t) && hasPrefixFold(tag, t) {
				return 0
			}
		}
	}
	if n > 0 && isBlankBytes(line[n:]) {
		return 7
	}
	return 0
}

func htmlBlockEnds(typ int, line []byte) bool {
	switch typ {
	case 1:
		return containsFold(line, "</pre>") || containsFold(line, "</script>") ||
			containsFold(line, "</style>") || containsFold(line, "</textarea>")
	case 2:
		return bytes.Contains(line, []byte("-->"))
	case 3:
		return bytes.Contains(line, []byte("?>"))
	case 4:
		return bytes.IndexByte(line, '>') >= 0
	case 5:
		return bytes.Contains(line, []byte("]]>"))
	}
	return false
}

func (p *parser) tryHTMLBlock(container *node) int {
	// Type 7 blocks cannot interrupt a paragraph. Like GitHub, they end a
	// paragraph that the line would otherwise continue lazily.
	typ := htmlBlockStart(p.line[p.nextNonspace:], isParagraph(container))
	if typ == 0 {
		return startNone
	}
	p.closeUnmatchedBlocks()
	b := p.addChild(kindHTMLBlock)
	b.level = int32(typ)
	// The indentation is part of the block's content.
	return startLeaf
}

// skipTagSpace skips spaces, tabs and up to one line ending.
func skipTagSpace(b []byte, i int) int {
	newline := false
	for i < len(b) {
		switch b[i] {
		case ' ', '\t':
			i++
		case '\r':
			if newline || i+1 >= len(b) || b[i+1] != '\n' {
				return i
			}
			i++
		case '\n':
			if newline {
				return i
			}
			newline = true
			i++
		default:
			return i
		}
	}
	return i
}

// scanOpenTag returns the length of the open tag at the start of b, or 0.
func scanOpenTag(b []byte) int {
	var s htmlScanner
	s.reset()
	return s.scanOpenTag(b, 0)
}

func (s *htmlScanner) scanOpenTag(full []byte, pos int) int {
	b := full[pos:]
	if len(b) < 2 || b[0] != '<' || !isASCIILetter(b[1]) {
		return 0
	}
	i := 2
	for i < len(b) && (isASCIIAlnum(b[i]) || b[i] == '-') {
		i++
	}
	for {
		j := skipTagSpace(b, i)
		if j >= len(b) {
			return 0
		}
		if b[j] == '>' {
			return j + 1
		}
		if b[j] == '/' {
			if j+1 < len(b) && b[j+1] == '>' {
				return j + 2
			}
			return 0
		}
		if j == i {
			// Attributes must be separated by whitespace.
			return 0
		}
		c := b[j]
		if !(isASCIILetter(c) || c == '_' || c == ':') {
			return 0
		}
		j++
		for j < len(b) && (isASCIIAlnum(b[j]) || b[j] == '_' || b[j] == '.' || b[j] == ':' || b[j] == '-') {
			j++
		}
		i = j
		k := skipTagSpace(b, j)
		if k < len(b) && b[k] == '=' {
			k = skipTagSpace(b, k+1)
			if k >= len(b) {
				return 0
			}
			switch q := b[k]; q {
			case '"', '\'':
				failed := &s.noDQ
				if q == '\'' {
					failed = &s.noSQ
				}
				e := indexFrom(full, pos+k+1, string(q), failed)
				if e < 0 {
					return 0
				}
				i = e - pos + 1
			default:
				s := k
				for k < len(b) {
					c := b[k]
					if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '"' || c == '\'' || c == '=' || c == '<' || c == '>' || c == '`' {
						break
					}
					k++
				}
				if k == s {
					return 0
				}
				i = k
			}
		}
	}
}

// scanClosingTag returns the length of the closing tag at the start of b, or 0.
func scanClosingTag(b []byte) int {
	if len(b) < 3 || b[0] != '<' || b[1] != '/' || !isASCIILetter(b[2]) {
		return 0
	}
	i := 3
	for i < len(b) && (isASCIIAlnum(b[i]) || b[i] == '-') {
		i++
	}
	i = skipTagSpace(b, i)
	if i < len(b) && b[i] == '>' {
		return i + 1
	}
	return 0
}

// htmlScanner scans inline raw HTML. It remembers where searches for
// terminators failed, so that scanning stays linear on inputs with many
// unterminated constructs.
type htmlScanner struct {
	noCommentEnd, noPIEnd, noDeclEnd, noCDATAEnd, noDQ, noSQ int
}

func (s *htmlScanner) reset() {
	*s = htmlScanner{noCommentEnd: -1, noPIEnd: -1, noDeclEnd: -1, noCDATAEnd: -1, noDQ: -1, noSQ: -1}
}

// indexFrom returns the index of sep in b at or after from, or -1. The
// search is skipped when an earlier search from a smaller offset failed.
func indexFrom(b []byte, from int, sep string, failedFrom *int) int {
	if *failedFrom >= 0 && from >= *failedFrom {
		return -1
	}
	i := bytes.Index(b[from:], []byte(sep))
	if i < 0 {
		if *failedFrom < 0 || from < *failedFrom {
			*failedFrom = from
		}
		return -1
	}
	return from + i
}

// scan returns the length of the raw HTML at b[pos:], or 0.
func (s *htmlScanner) scan(b []byte, pos int) int {
	rest := b[pos:]
	if len(rest) < 2 || rest[0] != '<' {
		return 0
	}
	switch c := rest[1]; {
	case isASCIILetter(c):
		return s.scanOpenTag(b, pos)
	case c == '/':
		return scanClosingTag(rest)
	case c == '?':
		e := indexFrom(b, pos+2, "?>", &s.noPIEnd)
		if e < 0 {
			return 0
		}
		return e + 2 - pos
	case c == '!':
		if bytes.HasPrefix(rest, []byte("<!--")) {
			if bytes.HasPrefix(rest, []byte("<!-->")) {
				return 5
			}
			if bytes.HasPrefix(rest, []byte("<!--->")) {
				return 6
			}
			e := indexFrom(b, pos+4, "-->", &s.noCommentEnd)
			if e < 0 {
				return 0
			}
			return e + 3 - pos
		}
		if bytes.HasPrefix(rest, []byte("<![CDATA[")) {
			e := indexFrom(b, pos+9, "]]>", &s.noCDATAEnd)
			if e < 0 {
				return 0
			}
			return e + 3 - pos
		}
		// Like GitHub, an inline declaration is uppercase letters and
		// whitespace after "<!", as in CommonMark 0.29.
		name := 2
		for name < len(rest) && 'A' <= rest[name] && rest[name] <= 'Z' {
			name++
		}
		if name > 2 && name < len(rest) && isASCIISpace(rest[name]) {
			e := indexFrom(b, pos+3, ">", &s.noDeclEnd)
			if e < 0 {
				return 0
			}
			return e + 1 - pos
		}
	}
	return 0
}
