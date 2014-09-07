package mahonia

// Converters for the EUC-KR encoding.

import (
	"unicode/utf8"
)

func init() {
	RegisterCharset(&Charset{
		Name: "EUC-KR",
		Aliases: []string{
			"ibm-1363",
			"KS_C_5601-1987",
			"KS_C_5601-1989",
			"KSC_5601",
			"Korean",
			"iso-ir-149",
			"cp1363",
			"5601",
			"ksc",
			"windows-949",
			"ibm-970",
			"cp970",
			"970",
			"cp949",
		},
		NewDecoder: func() Decoder {
			return decodeEucKr
		},
		NewEncoder: func() Encoder {
			eucKrOnce.Do(reverseEucKrTable)
			return encodeEucKr
		},
	})
}

func decodeEucKr(p []byte) (c rune, size int, status Status) {
	if len(p) == 0 {
		return 0, 0, NO_ROOM
	}

	b := p[0]
	if b < 0x80 {
		return rune(b), 1, SUCCESS
	}

	if len(p) < 2 {
		return 0, 0, NO_ROOM
	}

	euc := int(b)<<8 + int(p[1])
	c = rune(eucKrToUnicode[euc])

	if c == 0 {
		return utf8.RuneError, 2, INVALID_CHAR
	}
	return c, 2, SUCCESS
}

func encodeEucKr(p []byte, c rune) (size int, status Status) {
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

	euc := unicodeToEucKr[c]
	if euc == 0 {
		p[0] = '?'
		return 1, INVALID_CHAR
	}

	p[0] = byte(euc >> 8)
	p[1] = byte(euc)
	return 2, SUCCESS
}
