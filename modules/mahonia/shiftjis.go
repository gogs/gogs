package mahonia

// Converters for the Shift-JIS encoding.

import (
	"unicode/utf8"
)

func init() {
	RegisterCharset(&Charset{
		Name:    "Shift_JIS",
		Aliases: []string{"MS_Kanji", "csShiftJIS", "SJIS", "ibm-943", "windows-31j", "cp932", "windows-932"},
		NewDecoder: func() Decoder {
			return decodeSJIS
		},
		NewEncoder: func() Encoder {
			shiftJISOnce.Do(reverseShiftJISTable)
			return encodeSJIS
		},
	})
}

func decodeSJIS(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		return 0, 0, NO_ROOM
	}

	b := p[0]
	if b < 0x80 {
		return rune(b), 1, SUCCESS
	}

	if 0xa1 <= b && b <= 0xdf {
		return rune(b) + (0xff61 - 0xa1), 1, SUCCESS
	}

	if b == 0x80 || b == 0xa0 {
		return utf8.RuneError, 1, INVALID_CHAR
	}

	if len(p) < 2 {
		return 0, 0, NO_ROOM
	}

	jis := int(b)<<8 + int(p[1])
	c = rune(shiftJISToUnicode[jis])

	if c == 0 {
		return utf8.RuneError, 2, INVALID_CHAR
	}
	return c, 2, SUCCESS
}

func encodeSJIS(p []byte, c rune) (size int, status Status) {
	if len(p) == 0 {
		return 0, NO_ROOM
	}

	if c < 0x80 {
		p[0] = byte(c)
		return 1, SUCCESS
	}

	if 0xff61 <= c && c <= 0xff9f {
		// half-width katakana
		p[0] = byte(c - (0xff61 - 0xa1))
		return 1, SUCCESS
	}

	if len(p) < 2 {
		return 0, NO_ROOM
	}

	if c > 0xffff {
		p[0] = '?'
		return 1, INVALID_CHAR
	}

	jis := unicodeToShiftJIS[c]
	if jis == 0 {
		p[0] = '?'
		return 1, INVALID_CHAR
	}

	p[0] = byte(jis >> 8)
	p[1] = byte(jis)
	return 2, SUCCESS
}
