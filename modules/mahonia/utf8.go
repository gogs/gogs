package mahonia

import "unicode/utf8"

func init() {
	RegisterCharset(&Charset{
		Name:       "UTF-8",
		NewDecoder: func() Decoder { return decodeUTF8Rune },
		NewEncoder: func() Encoder { return encodeUTF8Rune },
	})
}

func decodeUTF8Rune(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	if p[0] < 128 {
		return rune(p[0]), 1, SUCCESS
	}

	c, size = utf8.DecodeRune(p)

	if c == 0xfffd {
		if utf8.FullRune(p) {
			status = INVALID_CHAR
			return
		}

		return 0, 0, NO_ROOM
	}

	status = SUCCESS
	return
}

func encodeUTF8Rune(p []byte, c rune) (size int, status Status) {
	size = utf8.RuneLen(c)
	if size > len(p) {
		return 0, NO_ROOM
	}

	return utf8.EncodeRune(p, c), SUCCESS
}
