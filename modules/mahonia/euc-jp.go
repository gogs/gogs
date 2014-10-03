package mahonia

import (
	"unicode/utf8"
)

// Converters for the EUC-JP encoding

func init() {
	RegisterCharset(&Charset{
		Name:    "EUC-JP",
		Aliases: []string{"extended_unix_code_packed_format_for_japanese", "cseucpkdfmtjapanese"},
		NewDecoder: func() Decoder {
			return decodeEucJP
		},
		NewEncoder: func() Encoder {
			jis0208Table.Reverse()
			jis0212Table.Reverse()
			return encodeEucJP
		},
	})
}

func decodeEucJP(p []byte) (c rune, size int, status Status) {
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

	case b == 0x8f:
		if len(p) < 3 {
			return 0, 0, NO_ROOM
		}
		c, size, status = jis0212Table.DecodeHigh(p[1:3])
		if status == SUCCESS {
			size = 3
		}
		return

	case 0xa1 <= b && b <= 0xfe:
		return jis0208Table.DecodeHigh(p)
	}

	return utf8.RuneError, 1, INVALID_CHAR
}

func encodeEucJP(p []byte, c rune) (size int, status Status) {
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

	size, status = jis0208Table.EncodeHigh(p, c)
	if status == SUCCESS {
		return size, status
	}

	size, status = jis0212Table.EncodeHigh(p[1:], c)
	switch status {
	case SUCCESS:
		p[0] = 0x8f
		return size + 1, SUCCESS

	case INVALID_CHAR:
		p[0] = '?'
		return 1, INVALID_CHAR
	}
	return size, status
}
