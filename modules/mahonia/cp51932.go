package mahonia

import (
	"unicode/utf8"
)

// Converters for Microsoft's version of the EUC-JP encoding

func init() {
	RegisterCharset(&Charset{
		Name:    "cp51932",
		Aliases: []string{"windows-51932"},
		NewDecoder: func() Decoder {
			return decodeCP51932
		},
		NewEncoder: func() Encoder {
			msJISTable.Reverse()
			return encodeCP51932
		},
	})
}

func decodeCP51932(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		return 0, 0, NO_ROOM
	}

	b := p[0]
	switch {
	case b < 0x80:
		return rune(b), 1, SUCCESS

	case b == 0x8e:
		if len(p) < 2 {
			return 0, 0, NO_ROOM
		}
		b2 := p[1]
		if b2 < 0xa1 || b2 > 0xdf {
			return utf8.RuneError, 1, INVALID_CHAR
		}
		return rune(b2) + (0xff61 - 0xa1), 2, SUCCESS

	case 0xa1 <= b && b <= 0xfe:
		return msJISTable.DecodeHigh(p)
	}

	return utf8.RuneError, 1, INVALID_CHAR
}

func encodeCP51932(p []byte, c rune) (size int, status Status) {
	if len(p) == 0 {
		return 0, NO_ROOM
	}

	if c < 0x80 {
		p[0] = byte(c)
		return 1, SUCCESS
	}

	if len(p) < 2 {
		return 0, NO_ROOM
	}

	if c > 0xffff {
		p[0] = '?'
		return 1, INVALID_CHAR
	}

	if 0xff61 <= c && c <= 0xff9f {
		p[0] = 0x8e
		p[1] = byte(c - (0xff61 - 0xa1))
		return 2, SUCCESS
	}

	return msJISTable.EncodeHigh(p, c)
}
