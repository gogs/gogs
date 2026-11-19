package markup

import (
	"bytes"
	"unicode/utf8"

	"gogs.io/gogs/internal/markup/dariomark"
)

// appendDecoded appends the text b of a token to dst as html.Tokenizer
// returns it: "\r\n" and "\r" become "\n", NUL bytes become U+FFFD if nul,
// and character references are decoded if refs, with the rules for attribute
// values if attr.
func appendDecoded(dst, b []byte, nul, refs, attr bool) []byte {
	start := 0
	for i := 0; i < len(b); {
		c := b[i]
		switch {
		case c == '\r':
			dst = append(dst, b[start:i]...)
			dst = append(dst, '\n')
			i++
			if i < len(b) && b[i] == '\n' {
				i++
			}
			start = i
		case c == 0 && nul:
			dst = append(dst, b[start:i]...)
			dst = append(dst, "\ufffd"...)
			i++
			start = i
		case c == '&' && refs:
			dst = append(dst, b[start:i]...)
			n := 0
			dst, n = appendEntity(dst, b[i:], attr)
			i += n
			start = i
		default:
			i++
		}
	}
	return append(dst, b[start:]...)
}

// appendEntity decodes the character reference at the start of s, which
// starts with "&", as html.Tokenizer does, appends the result to dst, and
// returns how many bytes of s it took. A "&" that does not start a reference
// is appended as is.
//
// In attribute values, references without a trailing ";" are decoded only
// if they are complete names, and not if followed by "=".
func appendEntity(dst, s []byte, attr bool) ([]byte, int) {
	if len(s) <= 1 {
		return append(dst, '&'), 1
	}
	i := 1
	if s[i] == '#' {
		if len(s) <= 2 {
			return append(dst, '&'), 1
		}
		i++
		hex := false
		if s[i] == 'x' || s[i] == 'X' {
			hex = true
			i++
		}
		i0 := i
		x := rune(0)
		for ; i < len(s); i++ {
			c := s[i]
			var d, mult rune
			switch {
			case '0' <= c && c <= '9':
				d = rune(c - '0')
			case hex && 'a' <= c && c <= 'f':
				d = rune(c-'a') + 10
			case hex && 'A' <= c && c <= 'F':
				d = rune(c-'A') + 10
			default:
				goto done
			}
			mult = 10
			if hex {
				mult = 16
			}
			if x <= utf8.MaxRune {
				x = mult*x + d
			}
		}
	done:
		if i == i0 {
			return append(dst, '&'), 1
		}
		if i < len(s) && s[i] == ';' {
			i++
		}
		switch {
		case 0x80 <= x && x <= 0x9f:
			x = windows1252[x-0x80]
		case x == 0 || 0xd800 <= x && x <= 0xdfff || x > utf8.MaxRune:
			x = utf8.RuneError
		}
		return utf8.AppendRune(dst, x), i
	}

	for i < len(s) {
		c := s[i]
		i++
		if 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' {
			continue
		}
		if c != ';' {
			i--
		}
		break
	}
	name := s[1:i]
	switch {
	case len(name) == 0:
	case name[len(name)-1] == ';':
		if v, ok := dariomark.Entity(name[:len(name)-1]); ok {
			return append(dst, v...), i
		}
	case attr && i < len(s) && s[i] == '=':
		return append(dst, '&'), 1
	case legacyEntities[string(name)]:
		v, _ := dariomark.Entity(name)
		return append(dst, v...), i
	}
	if !attr {
		// Outside attribute values, the longest legacy reference that
		// starts the name is decoded, e.g., "&notit;" is "¬it;".
		for j := min(len(name)-1, longestLegacyEntity); j > 1; j-- {
			if legacyEntities[string(name[:j])] {
				v, _ := dariomark.Entity(name[:j])
				return append(dst, v...), j + 1
			}
		}
	}
	return append(dst, '&'), 1
}

// longestLegacyEntity is the length of the longest name in legacyEntities.
const longestLegacyEntity = 6

// legacyEntities are the names of the character references that are decoded
// without a trailing ";".
var legacyEntities = func() map[string]bool {
	m := map[string]bool{}
	for _, name := range []string{
		"AElig", "AMP", "Aacute", "Acirc", "Agrave", "Aring", "Atilde", "Auml", "COPY", "Ccedil", "ETH",
		"Eacute", "Ecirc", "Egrave", "Euml", "GT", "Iacute", "Icirc", "Igrave", "Iuml", "LT", "Ntilde",
		"Oacute", "Ocirc", "Ograve", "Oslash", "Otilde", "Ouml", "QUOT", "REG", "THORN", "Uacute", "Ucirc",
		"Ugrave", "Uuml", "Yacute", "aacute", "acirc", "acute", "aelig", "agrave", "amp", "aring", "atilde",
		"auml", "brvbar", "ccedil", "cedil", "cent", "copy", "curren", "deg", "divide", "eacute", "ecirc",
		"egrave", "eth", "euml", "frac12", "frac14", "frac34", "gt", "iacute", "icirc", "iexcl", "igrave",
		"iquest", "iuml", "laquo", "lt", "macr", "micro", "middot", "nbsp", "not", "ntilde", "oacute",
		"ocirc", "ograve", "ordf", "ordm", "oslash", "otilde", "ouml", "para", "plusmn", "pound", "quot",
		"raquo", "reg", "sect", "shy", "sup1", "sup2", "sup3", "szlig", "thorn", "times", "uacute", "ucirc",
		"ugrave", "uml", "uuml", "yacute", "yen", "yuml",
	} {
		m[name] = true
	}
	return m
}()

// windows1252 are the characters that numeric references to U+0080 to U+009F
// stand for.
var windows1252 = [32]rune{
	'\u20ac', '\u0081', '\u201a', '\u0192', '\u201e', '\u2026', '\u2020', '\u2021',
	'\u02c6', '\u2030', '\u0160', '\u2039', '\u0152', '\u008d', '\u017d', '\u008f',
	'\u0090', '\u2018', '\u2019', '\u201c', '\u201d', '\u2022', '\u2013', '\u2014',
	'\u02dc', '\u2122', '\u0161', '\u203a', '\u0153', '\u009d', '\u017e', '\u0178',
}

// escaped are the replacements of the characters that appendEscaped escapes.
var escaped = [256]string{'&': "&amp;", '\'': "&#39;", '<': "&lt;", '>': "&gt;", '"': "&#34;", '\r': "&#13;"}

// appendEscaped appends b to dst escaped as html.EscapeString does.
func appendEscaped(dst, b []byte) []byte {
	start := 0
	for i, c := range b {
		if esc := escaped[c]; esc != "" {
			dst = append(dst, b[start:i]...)
			dst = append(dst, esc...)
			start = i + 1
		}
	}
	return append(dst, b[start:]...)
}

// textSpecial are the characters that appendTextEscaped does not copy as is.
var textSpecial = [256]bool{'&': true, '\'': true, '<': true, '>': true, '"': true, '\r': true, 0: true}

// appendTextEscaped appends the text b of a text token, decoded as
// appendDecoded does outside attribute values and then escaped as
// appendEscaped does, in one pass.
func appendTextEscaped(dst, b []byte, nul, refs bool) []byte {
	start := 0
	for i := 0; i < len(b); i++ {
		c := b[i]
		if !textSpecial[c] {
			continue
		}
		dst = append(dst, b[start:i]...)
		start = i + 1
		switch c {
		case '\r':
			dst = append(dst, '\n')
			if i+1 < len(b) && b[i+1] == '\n' {
				i++
				start++
			}
		case 0:
			if nul {
				dst = append(dst, "\ufffd"...)
			} else {
				dst = append(dst, 0)
			}
		case '&':
			if !refs {
				dst = append(dst, "&amp;"...)
				break
			}
			// The references that escaping restores.
			rest := b[i:]
			if ref := knownRef(rest); ref != "" {
				dst = append(dst, escaped[ref[0]]...)
				i += len(ref[1:]) - 1
				start = i + 1
				break
			}
			var buf [16]byte
			v, n := appendEntity(buf[:0], rest, false)
			dst = appendEscaped(dst, v)
			i += n - 1
			start = i + 1
		default:
			dst = append(dst, escaped[c]...)
		}
	}
	return append(dst, b[start:]...)
}

// knownRef returns the character and the reference at the start of b if it
// is one of the references that escaping produces, e.g., "<&lt;" for "&lt;".
func knownRef(b []byte) string {
	for _, ref := range [...]string{"<&lt;", ">&gt;", "&&amp;", "\"&quot;", "\"&#34;", "'&#39;"} {
		if bytes.HasPrefix(b, []byte(ref[1:])) {
			return ref
		}
	}
	return ""
}
