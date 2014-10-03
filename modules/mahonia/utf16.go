package mahonia

import (
	"unicode/utf16"
)

func init() {
	for i := 0; i < len(utf16Charsets); i++ {
		RegisterCharset(&utf16Charsets[i])
	}
}

var utf16Charsets = []Charset{
	{
		Name: "UTF-16",
		NewDecoder: func() Decoder {
			var decodeRune Decoder
			return func(p []byte) (c rune, size int, status Status) {
				if decodeRune == nil {
					// haven't read the BOM yet
					if len(p) < 2 {
						status = NO_ROOM
						return
					}

					switch {
					case p[0] == 0xfe && p[1] == 0xff:
						decodeRune = decodeUTF16beRune
						return 0, 2, STATE_ONLY
					case p[0] == 0xff && p[1] == 0xfe:
						decodeRune = decodeUTF16leRune
						return 0, 2, STATE_ONLY
					default:
						decodeRune = decodeUTF16beRune
					}
				}

				return decodeRune(p)
			}
		},
		NewEncoder: func() Encoder {
			wroteBOM := false
			return func(p []byte, c rune) (size int, status Status) {
				if !wroteBOM {
					if len(p) < 2 {
						status = NO_ROOM
						return
					}

					p[0] = 0xfe
					p[1] = 0xff
					wroteBOM = true
					return 2, STATE_ONLY
				}

				return encodeUTF16beRune(p, c)
			}
		},
	},
	{
		Name:       "UTF-16BE",
		NewDecoder: func() Decoder { return decodeUTF16beRune },
		NewEncoder: func() Encoder { return encodeUTF16beRune },
	},
	{
		Name:       "UTF-16LE",
		NewDecoder: func() Decoder { return decodeUTF16leRune },
		NewEncoder: func() Encoder { return encodeUTF16leRune },
	},
}

func decodeUTF16beRune(p []byte) (r rune, size int, status Status) {
	if len(p) < 2 {
		status = NO_ROOM
		return
	}

	c := rune(p[0])<<8 + rune(p[1])

	if utf16.IsSurrogate(c) {
		if len(p) < 4 {
			status = NO_ROOM
			return
		}

		c2 := rune(p[2])<<8 + rune(p[3])
		c = utf16.DecodeRune(c, c2)

		if c == 0xfffd {
			return c, 2, INVALID_CHAR
		} else {
			return c, 4, SUCCESS
		}
	}

	return c, 2, SUCCESS
}

func encodeUTF16beRune(p []byte, c rune) (size int, status Status) {
	if c < 0x10000 {
		if len(p) < 2 {
			status = NO_ROOM
			return
		}
		p[0] = byte(c >> 8)
		p[1] = byte(c)
		return 2, SUCCESS
	}

	if len(p) < 4 {
		status = NO_ROOM
		return
	}
	s1, s2 := utf16.EncodeRune(c)
	p[0] = byte(s1 >> 8)
	p[1] = byte(s1)
	p[2] = byte(s2 >> 8)
	p[3] = byte(s2)
	return 4, SUCCESS
}

func decodeUTF16leRune(p []byte) (r rune, size int, status Status) {
	if len(p) < 2 {
		status = NO_ROOM
		return
	}

	c := rune(p[1])<<8 + rune(p[0])

	if utf16.IsSurrogate(c) {
		if len(p) < 4 {
			status = NO_ROOM
			return
		}

		c2 := rune(p[3])<<8 + rune(p[2])
		c = utf16.DecodeRune(c, c2)

		if c == 0xfffd {
			return c, 2, INVALID_CHAR
		} else {
			return c, 4, SUCCESS
		}
	}

	return c, 2, SUCCESS
}

func encodeUTF16leRune(p []byte, c rune) (size int, status Status) {
	if c < 0x10000 {
		if len(p) < 2 {
			status = NO_ROOM
			return
		}
		p[1] = byte(c >> 8)
		p[0] = byte(c)
		return 2, SUCCESS
	}

	if len(p) < 4 {
		status = NO_ROOM
		return
	}
	s1, s2 := utf16.EncodeRune(c)
	p[1] = byte(s1 >> 8)
	p[0] = byte(s1)
	p[3] = byte(s2 >> 8)
	p[2] = byte(s2)
	return 4, SUCCESS
}
