package mahonia

import "unicode/utf8"

// Translate enables a Decoder to implement go-charset's Translator interface.
func (d Decoder) Translate(data []byte, eof bool) (n int, cdata []byte, err error) {
	cdata = make([]byte, len(data)+1)
	destPos := 0

	for n < len(data) {
		rune, size, status := d(data[n:])

		switch status {
		case STATE_ONLY:
			n += size
			continue

		case NO_ROOM:
			if !eof {
				return n, cdata[:destPos], nil
			}
			rune = 0xfffd
			n = len(data)

		default:
			n += size
		}

		if rune < 128 {
			if destPos >= len(cdata) {
				cdata = doubleLength(cdata)
			}
			cdata[destPos] = byte(rune)
			destPos++
		} else {
			if destPos+utf8.RuneLen(rune) > len(cdata) {
				cdata = doubleLength(cdata)
			}
			destPos += utf8.EncodeRune(cdata[destPos:], rune)
		}
	}

	return n, cdata[:destPos], nil
}

func doubleLength(b []byte) []byte {
	b2 := make([]byte, 2*len(b))
	copy(b2, b)
	return b2
}
