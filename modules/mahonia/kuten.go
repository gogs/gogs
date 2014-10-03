package mahonia

import (
	"sync"
	"unicode/utf8"
)

// A kutenTable holds the data for a double-byte character set, arranged by ku
// (区, zone) and ten (点, position). These can be converted to various actual
// encoding schemes.
type kutenTable struct {
	// Data[ku][ten] is the unicode value for the character at that zone and
	// position.
	Data [94][94]uint16

	// FromUnicode holds the ku and ten for each Unicode code point.
	// It is not available until Reverse() has been called.
	FromUnicode [][2]byte

	// once is used to synchronize the generation of FromUnicode.
	once sync.Once
}

// Reverse generates FromUnicode.
func (t *kutenTable) Reverse() {
	t.once.Do(func() {
		t.FromUnicode = make([][2]byte, 65536)
		for ku := range t.Data {
			for ten, unicode := range t.Data[ku] {
				t.FromUnicode[unicode] = [2]byte{byte(ku), byte(ten)}
			}
		}
	})
}

// DecodeLow decodes a character from an encoding that does not have the high
// bit set.
func (t *kutenTable) DecodeLow(p []byte) (c rune, size int, status Status) {
	if len(p) < 2 {
		return 0, 0, NO_ROOM
	}
	ku := p[0] - 0x21
	ten := p[1] - 0x21
	if ku > 93 || ten > 93 {
		return utf8.RuneError, 1, INVALID_CHAR
	}
	u := t.Data[ku][ten]
	if u == 0 {
		return utf8.RuneError, 1, INVALID_CHAR
	}
	return rune(u), 2, SUCCESS
}

// DecodeHigh decodes a character from an encoding that has the high bit set.
func (t *kutenTable) DecodeHigh(p []byte) (c rune, size int, status Status) {
	if len(p) < 2 {
		return 0, 0, NO_ROOM
	}
	ku := p[0] - 0xa1
	ten := p[1] - 0xa1
	if ku > 93 || ten > 93 {
		return utf8.RuneError, 1, INVALID_CHAR
	}
	u := t.Data[ku][ten]
	if u == 0 {
		return utf8.RuneError, 1, INVALID_CHAR
	}
	return rune(u), 2, SUCCESS
}

// EncodeHigh encodes a character in an encoding that has the high bit set.
func (t *kutenTable) EncodeHigh(p []byte, c rune) (size int, status Status) {
	if len(p) < 2 {
		return 0, NO_ROOM
	}
	if c > 0xffff {
		p[0] = '?'
		return 1, INVALID_CHAR
	}
	kuten := t.FromUnicode[c]
	if kuten == [2]byte{0, 0} && c != rune(t.Data[0][0]) {
		p[0] = '?'
		return 1, INVALID_CHAR
	}
	p[0] = kuten[0] + 0xa1
	p[1] = kuten[1] + 0xa1
	return 2, SUCCESS
}
