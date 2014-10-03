package mahonia

// Converters for ASCII and ISO-8859-1

func init() {
	for i := 0; i < len(asciiCharsets); i++ {
		RegisterCharset(&asciiCharsets[i])
	}
}

var asciiCharsets = []Charset{
	{
		Name:       "US-ASCII",
		NewDecoder: func() Decoder { return decodeASCIIRune },
		NewEncoder: func() Encoder { return encodeASCIIRune },
		Aliases:    []string{"ASCII", "US", "ISO646-US", "IBM367", "cp367", "ANSI_X3.4-1968", "iso-ir-6", "ANSI_X3.4-1986", "ISO_646.irv:1991", "csASCII"},
	},
	{
		Name:       "ISO-8859-1",
		NewDecoder: func() Decoder { return decodeLatin1Rune },
		NewEncoder: func() Encoder { return encodeLatin1Rune },
		Aliases:    []string{"latin1", "ISO Latin 1", "IBM819", "cp819", "ISO_8859-1:1987", "iso-ir-100", "l1", "csISOLatin1"},
	},
}

func decodeASCIIRune(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	b := p[0]
	if b > 127 {
		return 0xfffd, 1, INVALID_CHAR
	}
	return rune(b), 1, SUCCESS
}

func encodeASCIIRune(p []byte, c rune) (size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	if c < 128 {
		p[0] = byte(c)
		return 1, SUCCESS
	}

	p[0] = '?'
	return 1, INVALID_CHAR
}

func decodeLatin1Rune(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	return rune(p[0]), 1, SUCCESS
}

func encodeLatin1Rune(p []byte, c rune) (size int, status Status) {
	if len(p) == 0 {
		status = NO_ROOM
		return
	}

	if c < 256 {
		p[0] = byte(c)
		return 1, SUCCESS
	}

	p[0] = '?'
	return 1, INVALID_CHAR
}
