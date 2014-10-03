package mahonia

// Converters for Big 5 encoding.

import (
	"sync"
)

func init() {
	RegisterCharset(&Charset{
		Name:    "Big5",
		Aliases: []string{"csBig5"},
		NewDecoder: func() Decoder {
			return decodeBig5Rune
		},
		NewEncoder: func() Encoder {
			big5Once.Do(reverseBig5Table)
			return encodeBig5Rune
		},
	})
}

func decodeBig5Rune(p []byte) (r rune, size int, status Status) {
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

	c := int(p[0])<<8 + int(p[1])
	c = int(big5ToUnicode[c])
	if c > 0 {
		return rune(c), 2, SUCCESS
	}

	return 0xfffd, 1, INVALID_CHAR
}

func encodeBig5Rune(p []byte, r rune) (size int, status Status) {
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

	if r < 0x10000 {
		c := unicodeToBig5[r]
		if c > 0 {
			p[0] = byte(c >> 8)
			p[1] = byte(c)
			return 2, SUCCESS
		}
	}

	p[0] = '?'
	return 1, INVALID_CHAR
}

var big5Once sync.Once

var unicodeToBig5 []uint16

func reverseBig5Table() {
	unicodeToBig5 = make([]uint16, 65536)

	for big5, unicode := range big5ToUnicode {
		if unicode > 0 {
			unicodeToBig5[unicode] = uint16(big5)
		}
	}
}
