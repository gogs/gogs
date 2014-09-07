package mahonia

// Converters for GBK encoding.

func init() {
	RegisterCharset(&Charset{
		Name:    "GBK",
		Aliases: []string{"GB2312"}, // GBK is a superset of GB2312.
		NewDecoder: func() Decoder {
			return decodeGBKRune
		},
		NewEncoder: func() Encoder {
			return encodeGBKRune
		},
	})
}

func decodeGBKRune(p []byte) (r rune, size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	b := p[0]
	if b < 128 {
		return rune(b), 1, SUCCESS
	}

	if len(p) < 2 {
		status = NO_ROOM
		return
	}

	c := uint16(p[0])<<8 + uint16(p[1])
	r = rune(gbkToUnicode[c])
	if r == 0 {
		r = gbkToUnicodeExtra[c]
	}

	if r != 0 {
		return r, 2, SUCCESS
	}

	return 0xfffd, 1, INVALID_CHAR
}

func encodeGBKRune(p []byte, r rune) (size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	if r < 128 {
		p[0] = byte(r)
		return 1, SUCCESS
	}

	if len(p) < 2 {
		status = NO_ROOM
		return
	}

	var c uint16
	if r < 0x10000 {
		c = unicodeToGBK[r]
	} else {
		c = unicodeToGBKExtra[r]
	}

	if c != 0 {
		p[0] = byte(c >> 8)
		p[1] = byte(c)
		return 2, SUCCESS
	}

	p[0] = 0x1a
	return 1, INVALID_CHAR
}
