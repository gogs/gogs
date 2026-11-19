// Copyright 2010 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package dariomark

import (
	"bytes"

	"golang.org/x/net/html"
)

// htmlTokenizer splits HTML into the tokens that html.Tokenizer returns for
// it, with the same raw bytes, see TestHTMLTokenizer. It is a port of
// golang.org/x/net/html/token.go that reads a byte slice instead of an
// io.Reader, never modifies the slice and leaves unescaping to its caller, so
// that tokenizing allocates nothing.
//
// It does not support CDATA sections, which html.Tokenizer reads as comments
// unless AllowCDATA is set.
type htmlTokenizer struct {
	buf []byte
	// eof is whether a read went past the end of buf, which corresponds to
	// html.Tokenizer's io.EOF error.
	eof bool
	tt  html.TokenType
	// raw is the span of the current token in buf. data is the span of the
	// text of a text, comment or doctype token or of the name of a tag.
	raw, data span
	// pendingAttr is the key and value of the attribute being read.
	pendingAttr [2]span
	// attr are the keys and values of the attributes of the current start
	// tag, without the attributes whose keys repeat earlier keys.
	attr [][2]span
	// attrKeys are the lower-cased keys of attr when a tag has many
	// attributes, see seenAttr.
	attrKeys map[string]struct{}
	// rawTag is the lower-cased name of the element whose end tag ends the
	// next token, which is raw text, or "" for other tokens.
	rawTag string
	// textIsRaw is whether the text of the current text token is not
	// escaped.
	textIsRaw bool
	// convertNUL is whether NUL bytes in the text of the current text token
	// become U+FFFD.
	convertNUL bool
}

// span is a range of bytes in htmlTokenizer.buf. The start is inclusive, the
// end is exclusive.
type span struct {
	start, end int
}

func (z *htmlTokenizer) reset(buf []byte) {
	attr := z.attr[:0]
	*z = htmlTokenizer{buf: buf, attr: attr, attrKeys: z.attrKeys}
}

// readByte returns the next byte, or sets eof at the end of buf.
func (z *htmlTokenizer) readByte() byte {
	if z.raw.end >= len(z.buf) {
		z.eof = true
		return 0
	}
	c := z.buf[z.raw.end]
	z.raw.end++
	return c
}

func (z *htmlTokenizer) skipWhiteSpace() {
	if z.eof {
		return
	}
	for {
		c := z.readByte()
		if z.eof {
			return
		}
		switch c {
		case ' ', '\n', '\r', '\t', '\f':
		default:
			z.raw.end--
			return
		}
	}
}

// readRawOrRCDATA reads until the next "</foo>", where "foo" is z.rawTag.
func (z *htmlTokenizer) readRawOrRCDATA() {
	if z.rawTag == "script" {
		z.readScript()
		z.textIsRaw = true
		z.rawTag = ""
		return
	}
	for {
		i := bytes.IndexByte(z.buf[z.raw.end:], '<')
		if i < 0 {
			z.raw.end = len(z.buf)
			z.eof = true
			break
		}
		z.raw.end += i + 1
		c := z.readByte()
		if z.eof {
			break
		}
		if c != '/' {
			z.raw.end--
			continue
		}
		if z.readRawEndTag() || z.eof {
			break
		}
	}
	z.data.end = z.raw.end
	// The RCDATA of textarea and title elements can contain character
	// references.
	z.textIsRaw = z.rawTag != "textarea" && z.rawTag != "title"
	z.rawTag = ""
}

// readRawEndTag attempts to read a tag like "</foo>", where "foo" is z.rawTag.
// If it succeeds, it backs up the input position to reconsume the tag and
// returns true. The opening "</" has already been consumed.
func (z *htmlTokenizer) readRawEndTag() bool {
	for i := 0; i < len(z.rawTag); i++ {
		c := z.readByte()
		if z.eof {
			return false
		}
		if c != z.rawTag[i] && c != z.rawTag[i]-('a'-'A') {
			z.raw.end--
			return false
		}
	}
	c := z.readByte()
	if z.eof {
		return false
	}
	switch c {
	case ' ', '\n', '\r', '\t', '\f', '/', '>':
		// The 3 is 2 for the leading "</" plus 1 for the trailing character c.
		z.raw.end -= 3 + len(z.rawTag)
		return true
	}
	z.raw.end--
	return false
}

// readScript reads until the next </script> tag, following the rules for
// escaping and hiding the end tag.
func (z *htmlTokenizer) readScript() {
	defer func() {
		z.data.end = z.raw.end
	}()
	var c byte

scriptData:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '<' {
		goto scriptDataLessThanSign
	}
	goto scriptData

scriptDataLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '/':
		goto scriptDataEndTagOpen
	case '!':
		goto scriptDataEscapeStart
	}
	z.raw.end--
	goto scriptData

scriptDataEndTagOpen:
	if z.readRawEndTag() || z.eof {
		return
	}
	goto scriptData

scriptDataEscapeStart:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '-' {
		goto scriptDataEscapeStartDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscapeStartDash:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '-' {
		goto scriptDataEscapedDashDash
	}
	z.raw.end--
	goto scriptData

scriptDataEscaped:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDash
	case '<':
		goto scriptDataEscapedLessThanSign
	}
	goto scriptDataEscaped

scriptDataEscapedDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDashDash
	case '<':
		goto scriptDataEscapedLessThanSign
	}
	goto scriptDataEscaped

scriptDataEscapedDashDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataEscapedDashDash
	case '<':
		goto scriptDataEscapedLessThanSign
	case '>':
		goto scriptData
	}
	goto scriptDataEscaped

scriptDataEscapedLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		goto scriptDataEscapedEndTagOpen
	}
	if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
		goto scriptDataDoubleEscapeStart
	}
	z.raw.end--
	goto scriptData

scriptDataEscapedEndTagOpen:
	if z.readRawEndTag() || z.eof {
		return
	}
	goto scriptDataEscaped

scriptDataDoubleEscapeStart:
	z.raw.end--
	for i := 0; i < len("script"); i++ {
		c = z.readByte()
		if z.eof {
			return
		}
		if c != "script"[i] && c != "SCRIPT"[i] {
			z.raw.end--
			goto scriptDataEscaped
		}
	}
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case ' ', '\n', '\r', '\t', '\f', '/', '>':
		goto scriptDataDoubleEscaped
	}
	z.raw.end--
	goto scriptDataEscaped

scriptDataDoubleEscaped:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDashDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedDashDash:
	c = z.readByte()
	if z.eof {
		return
	}
	switch c {
	case '-':
		goto scriptDataDoubleEscapedDashDash
	case '<':
		goto scriptDataDoubleEscapedLessThanSign
	case '>':
		goto scriptData
	}
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapedLessThanSign:
	c = z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		goto scriptDataDoubleEscapeEnd
	}
	z.raw.end--
	goto scriptDataDoubleEscaped

scriptDataDoubleEscapeEnd:
	if z.readRawEndTag() {
		z.raw.end += len("</script>")
		goto scriptDataEscaped
	}
	if z.eof {
		return
	}
	goto scriptDataDoubleEscaped
}

// readComment reads the next comment token starting with "<!--". The opening
// "<!--" has already been consumed.
func (z *htmlTokenizer) readComment() {
	z.data.start = z.raw.end
	defer func() {
		if z.data.end < z.data.start {
			// It's a comment with no data, like <!-->.
			z.data.end = z.data.start
		}
	}()

	var dashCount int
	beginning := true
	for {
		c := z.readByte()
		if z.eof {
			z.data.end = z.calculateAbruptCommentDataEnd()
			return
		}
		switch c {
		case '-':
			dashCount++
			continue
		case '>':
			if dashCount >= 2 || beginning {
				z.data.end = z.raw.end - len("-->")
				return
			}
		case '!':
			if dashCount >= 2 {
				c = z.readByte()
				if z.eof {
					z.data.end = z.calculateAbruptCommentDataEnd()
					return
				} else if c == '>' {
					z.data.end = z.raw.end - len("--!>")
					return
				} else if c == '-' {
					dashCount = 1
					beginning = false
					continue
				}
			}
		}
		dashCount = 0
		beginning = false
	}
}

func (z *htmlTokenizer) calculateAbruptCommentDataEnd() int {
	raw := z.buf[z.raw.start:z.raw.end]
	const prefixLen = len("<!--")
	if len(raw) >= prefixLen {
		raw = raw[prefixLen:]
		if bytes.HasSuffix(raw, []byte("--!")) {
			return z.raw.end - 3
		} else if bytes.HasSuffix(raw, []byte("--")) {
			return z.raw.end - 2
		} else if bytes.HasSuffix(raw, []byte("-")) {
			return z.raw.end - 1
		}
	}
	return z.raw.end
}

// readUntilCloseAngle reads until the next ">".
func (z *htmlTokenizer) readUntilCloseAngle() {
	z.data.start = z.raw.end
	i := bytes.IndexByte(z.buf[z.raw.end:], '>')
	if i < 0 {
		z.raw.end = len(z.buf)
		z.eof = true
		z.data.end = z.raw.end
		return
	}
	z.raw.end += i + 1
	z.data.end = z.raw.end - len(">")
}

// readMarkupDeclaration reads the next token starting with "<!". It might be
// a "<!--comment-->", a "<!DOCTYPE foo>" or "<!a bogus comment". The opening
// "<!" has already been consumed.
func (z *htmlTokenizer) readMarkupDeclaration() html.TokenType {
	z.data.start = z.raw.end
	var c [2]byte
	for i := 0; i < 2; i++ {
		c[i] = z.readByte()
		if z.eof {
			// A bogus comment.
			z.data.end = z.raw.end
			if i == 1 && c[0] == '>' {
				z.data.end--
			}
			return html.CommentToken
		}
	}
	if c[0] == '-' && c[1] == '-' {
		z.readComment()
		return html.CommentToken
	}
	z.raw.end -= 2
	if z.readDoctype() {
		return html.DoctypeToken
	}
	// It's a bogus comment.
	z.readUntilCloseAngle()
	return html.CommentToken
}

// readDoctype attempts to read a doctype declaration and returns true if
// successful. The opening "<!" has already been consumed.
func (z *htmlTokenizer) readDoctype() bool {
	const s = "DOCTYPE"
	for i := 0; i < len(s); i++ {
		c := z.readByte()
		if z.eof {
			// Back up to read the fragment of "DOCTYPE" again as a bogus
			// comment, which ends at the end of buf.
			z.raw.end = z.data.start
			z.eof = false
			return false
		}
		if c != s[i] && c != s[i]+('a'-'A') {
			// Back up to read the fragment of "DOCTYPE" again.
			z.raw.end = z.data.start
			return false
		}
	}
	if z.skipWhiteSpace(); z.eof {
		z.data.start = z.raw.end
		z.data.end = z.raw.end
		return true
	}
	z.readUntilCloseAngle()
	return true
}

// setRawTag sets z.rawTag to the name in names that the start tag has, if
// any.
func (z *htmlTokenizer) setRawTag(names ...string) {
	for _, s := range names {
		if z.startTagIs(s) {
			z.rawTag = s
			return
		}
	}
}

// startTagIs returns whether the name of the start tag case-insensitively
// matches s.
func (z *htmlTokenizer) startTagIs(s string) bool {
	if z.data.end-z.data.start != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := z.buf[z.data.start+i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != s[i] {
			return false
		}
	}
	return true
}

// readStartTag reads the next start tag token. The opening "<a" has already
// been consumed, where 'a' means anything in [A-Za-z].
func (z *htmlTokenizer) readStartTag() html.TokenType {
	z.readTag(true)
	if z.eof {
		return html.ErrorToken
	}
	// Several tags flag the tokenizer's next token as raw.
	c := z.buf[z.data.start]
	if 'A' <= c && c <= 'Z' {
		c += 'a' - 'A'
	}
	switch c {
	case 'i':
		z.setRawTag("iframe")
	case 'n':
		z.setRawTag("noembed", "noframes", "noscript")
	case 'p':
		z.setRawTag("plaintext")
	case 's':
		z.setRawTag("script", "style")
	case 't':
		z.setRawTag("textarea", "title")
	case 'x':
		z.setRawTag("xmp")
	}
	// The tag is self-closing if its last character before the ">" is a "/"
	// that does not end the unquoted value of its last attribute, e.g., not
	// in <p a=/>.
	n := len(z.attr)
	if z.buf[z.raw.end-2] == '/' && (n == 0 || z.raw.end-2 != z.attr[n-1][1].end-1) {
		return html.SelfClosingTagToken
	}
	return html.StartTagToken
}

// readTag reads the next tag token and its attributes. If saveAttr, those
// attributes are saved in z.attr, otherwise z.attr is set to an empty slice.
// The opening "<a" or "</a" has already been consumed, where 'a' means anything
// in [A-Za-z].
func (z *htmlTokenizer) readTag(saveAttr bool) {
	z.attr = z.attr[:0]
	z.readTagName()
	if z.skipWhiteSpace(); z.eof {
		return
	}
	for {
		c := z.readByte()
		if z.eof || c == '>' {
			break
		}
		z.raw.end--
		z.readTagAttrKey()
		z.readTagAttrVal()
		if saveAttr && z.pendingAttr[0].start != z.pendingAttr[0].end && !z.seenAttr(z.pendingAttr[0]) {
			z.attr = append(z.attr, z.pendingAttr)
		}
		if z.skipWhiteSpace(); z.eof {
			break
		}
	}
}

// manyAttrs is the number of attributes from which seenAttr looks keys up in
// a map, so that tags with many attributes take linear time.
const manyAttrs = 16

// seenAttr reports whether a saved attribute of the current tag has the key
// in buf at s, compared case-insensitively in ASCII.
func (z *htmlTokenizer) seenAttr(s span) bool {
	key := z.buf[s.start:s.end]
	if len(z.attr) < manyAttrs {
		for _, a := range z.attr {
			if equalFoldASCII(z.buf[a[0].start:a[0].end], key) {
				return true
			}
		}
		return false
	}
	if len(z.attr) == manyAttrs {
		if z.attrKeys == nil {
			z.attrKeys = make(map[string]struct{})
		}
		clear(z.attrKeys)
		for _, a := range z.attr {
			z.attrKeys[string(lowerASCII(z.buf[a[0].start:a[0].end]))] = struct{}{}
		}
	}
	lower := string(lowerASCII(key))
	if _, ok := z.attrKeys[lower]; ok {
		return true
	}
	z.attrKeys[lower] = struct{}{}
	return false
}

// lowerASCII returns a copy of b with ASCII letters lower-cased.
func lowerASCII(b []byte) []byte {
	l := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		l[i] = c
	}
	return l
}

// equalFoldASCII reports whether a and b are equal, ignoring the case of
// ASCII letters.
func equalFoldASCII(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if 'A' <= x && x <= 'Z' {
			x += 'a' - 'A'
		}
		if 'A' <= y && y <= 'Z' {
			y += 'a' - 'A'
		}
		if x != y {
			return false
		}
	}
	return true
}

// readTagName sets z.data to the "div" in "<div k=v>". The first byte of the
// tag name has already been consumed.
func (z *htmlTokenizer) readTagName() {
	z.data.start = z.raw.end - 1
	for {
		c := z.readByte()
		if z.eof {
			z.data.end = z.raw.end
			return
		}
		switch c {
		case ' ', '\n', '\r', '\t', '\f':
			z.data.end = z.raw.end - 1
			return
		case '/', '>':
			z.raw.end--
			z.data.end = z.raw.end
			return
		}
	}
}

// readTagAttrKey sets z.pendingAttr[0] to the "k" in "<div k=v>".
func (z *htmlTokenizer) readTagAttrKey() {
	z.pendingAttr[0].start = z.raw.end
	for {
		c := z.readByte()
		if z.eof {
			z.pendingAttr[0].end = z.raw.end
			return
		}
		switch c {
		case '=':
			if z.pendingAttr[0].start+1 == z.raw.end {
				// An equals sign before the attribute name begins is a
				// character of the name.
				continue
			}
			fallthrough
		case ' ', '\n', '\r', '\t', '\f', '/', '>':
			z.raw.end--
			z.pendingAttr[0].end = z.raw.end
			return
		}
	}
}

// readTagAttrVal sets z.pendingAttr[1] to the "v" in "<div k=v>".
func (z *htmlTokenizer) readTagAttrVal() {
	z.pendingAttr[1].start = z.raw.end
	z.pendingAttr[1].end = z.raw.end
	if z.skipWhiteSpace(); z.eof {
		return
	}
	c := z.readByte()
	if z.eof {
		return
	}
	if c == '/' {
		// A solidus after the name starts the self-closing start tag state.
		return
	}
	if c != '=' {
		z.raw.end--
		return
	}
	if z.skipWhiteSpace(); z.eof {
		return
	}
	quote := z.readByte()
	if z.eof {
		return
	}
	switch quote {
	case '>':
		z.raw.end--
		return

	case '\'', '"':
		z.pendingAttr[1].start = z.raw.end
		i := bytes.IndexByte(z.buf[z.raw.end:], quote)
		if i < 0 {
			z.raw.end = len(z.buf)
			z.eof = true
			z.pendingAttr[1].end = z.raw.end
			return
		}
		z.raw.end += i + 1
		z.pendingAttr[1].end = z.raw.end - 1
		return

	default:
		z.pendingAttr[1].start = z.raw.end - 1
		for {
			c := z.readByte()
			if z.eof {
				z.pendingAttr[1].end = z.raw.end
				return
			}
			switch c {
			case ' ', '\n', '\r', '\t', '\f':
				z.pendingAttr[1].end = z.raw.end - 1
				return
			case '>':
				z.raw.end--
				z.pendingAttr[1].end = z.raw.end
				return
			}
		}
	}
}

// next scans the next token and returns its type.
func (z *htmlTokenizer) next() html.TokenType {
	z.raw.start = z.raw.end
	z.data.start = z.raw.end
	z.data.end = z.raw.end
	if z.eof {
		z.tt = html.ErrorToken
		return z.tt
	}
	if z.rawTag != "" {
		if z.rawTag == "plaintext" {
			// Read everything up to the end.
			z.raw.end = len(z.buf)
			z.eof = true
			z.data.end = z.raw.end
			z.textIsRaw = true
		} else {
			z.readRawOrRCDATA()
		}
		if z.data.end > z.data.start {
			z.tt = html.TextToken
			z.convertNUL = true
			return z.tt
		}
	}
	z.textIsRaw = false
	z.convertNUL = false

	for {
		i := bytes.IndexByte(z.buf[z.raw.end:], '<')
		if i < 0 {
			z.raw.end = len(z.buf)
			z.eof = true
			break
		}
		z.raw.end += i + 1

		// Check if the '<' we have just read is part of a tag, comment or
		// doctype. If not, it's part of the accumulated text token.
		c := z.readByte()
		if z.eof {
			break
		}
		var tokenType html.TokenType
		switch {
		case 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z':
			tokenType = html.StartTagToken
		case c == '/':
			tokenType = html.EndTagToken
		case c == '!' || c == '?':
			// Comments, doctypes and processing instructions.
			tokenType = html.CommentToken
		default:
			// Reconsume the current character.
			z.raw.end--
			continue
		}

		// Text before the non-text token is returned first, and the non-text
		// token on the next call.
		if x := z.raw.end - len("<a"); z.raw.start < x {
			z.raw.end = x
			z.data.end = x
			z.tt = html.TextToken
			return z.tt
		}
		switch tokenType {
		case html.StartTagToken:
			z.tt = z.readStartTag()
			return z.tt
		case html.EndTagToken:
			c = z.readByte()
			if z.eof {
				break
			}
			if c == '>' {
				// "</>" does not generate a token at all. Like html.Tokenizer,
				// generate an empty comment.
				z.tt = html.CommentToken
				return z.tt
			}
			if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' {
				z.readTag(false)
				if z.eof {
					z.tt = html.ErrorToken
				} else {
					z.tt = html.EndTagToken
				}
				return z.tt
			}
			z.raw.end--
			z.readUntilCloseAngle()
			z.tt = html.CommentToken
			return z.tt
		case html.CommentToken:
			if c == '!' {
				z.tt = z.readMarkupDeclaration()
				return z.tt
			}
			z.raw.end--
			z.readUntilCloseAngle()
			z.tt = html.CommentToken
			return z.tt
		}
		break
	}
	if z.raw.start < z.raw.end {
		z.data.end = z.raw.end
		z.tt = html.TextToken
		return z.tt
	}
	z.tt = html.ErrorToken
	return z.tt
}
