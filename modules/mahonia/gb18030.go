package mahonia

import (
	"sync"
)

// Converters for GB18030 encoding.

func init() {
	RegisterCharset(&Charset{
		Name: "GB18030",
		NewDecoder: func() Decoder {
			gb18030Once.Do(buildGB18030Tables)
			return decodeGB18030Rune
		},
		NewEncoder: func() Encoder {
			gb18030Once.Do(buildGB18030Tables)
			return encodeGB18030Rune
		},
	})
}

func decodeGB18030Rune(p []byte) (r rune, size int, status Status) {
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

	if p[0] < 0x81 || p[0] > 0xfe {
		return 0xfffd, 1, INVALID_CHAR
	}

	if p[1] >= 0x40 {
		// 2-byte character
		c := uint16(p[0])<<8 + uint16(p[1])
		r = rune(gbkToUnicode[c])
		if r == 0 {
			r = gbkToUnicodeExtra[c]
		}

		if r != 0 {
			return r, 2, SUCCESS
		}
	} else if p[1] >= 0x30 {
		// 4-byte character
		if len(p) < 4 {
			return 0, 0, NO_ROOM
		}
		if p[2] < 0x81 || p[2] > 0xfe || p[3] < 0x30 || p[3] > 0x39 {
			return 0xfffd, 1, INVALID_CHAR
		}

		code := uint32(p[0])<<24 + uint32(p[1])<<16 + uint32(p[2])<<8 + uint32(p[3])
		lin := gb18030Linear(code)

		if lin <= maxGB18030Linear {
			r = rune(gb18030LinearToUnicode[lin])
			if r != 0 {
				return r, 4, SUCCESS
			}
		}

		for _, rng := range gb18030Ranges {
			if lin >= rng.firstGB && lin <= rng.lastGB {
				return rng.firstRune + rune(lin) - rune(rng.firstGB), 4, SUCCESS
			}
		}
	}

	return 0xfffd, 1, INVALID_CHAR
}

func encodeGB18030Rune(p []byte, r rune) (size int, status Status) {
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

	if len(p) < 4 {
		return 0, NO_ROOM
	}

	if r < 0x10000 {
		f := unicodeToGB18030[r]
		if f != 0 {
			p[0] = byte(f >> 24)
			p[1] = byte(f >> 16)
			p[2] = byte(f >> 8)
			p[3] = byte(f)
			return 4, SUCCESS
		}
	}

	for _, rng := range gb18030Ranges {
		if r >= rng.firstRune && r <= rng.lastRune {
			lin := rng.firstGB + uint32(r) - uint32(rng.firstRune)
			p[0] = byte(lin/(10*126*10)) + 0x81
			p[1] = byte(lin/(126*10)%10) + 0x30
			p[2] = byte(lin/10%126) + 0x81
			p[3] = byte(lin%10) + 0x30
			return 4, SUCCESS
		}
	}

	p[0] = 0x1a
	return 1, INVALID_CHAR
}

var gb18030Once sync.Once

// Mapping from gb18039Linear values to Unicode.
var gb18030LinearToUnicode []uint16

var unicodeToGB18030 []uint32

func buildGB18030Tables() {
	gb18030LinearToUnicode = make([]uint16, maxGB18030Linear+1)
	unicodeToGB18030 = make([]uint32, 65536)
	for _, data := range gb18030Data {
		gb18030LinearToUnicode[gb18030Linear(data.gb18030)] = data.unicode
		unicodeToGB18030[data.unicode] = data.gb18030
	}
}
