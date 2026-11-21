package chardet

import "unicode/utf8"

// The decoders in this file reproduce what chardetng observes from
// encoding_rs's decode_to_utf16_without_replacement when it is called with a
// one-byte source, a two-unit destination and last=false (decodeByte), or with
// an empty source and last=true (finish). The decoding follows the WHATWG
// Encoding Standard, and the Malformed lengths follow encoding_rs 0.8.x, which
// reports an ASCII byte that ends a multi-byte sequence early as not consumed
// and not part of the error. chardetng does not refeed such a byte, so these
// decoders simply return to their initial state without decoding it.

type decodeStep struct {
	malformed    bool  // DecoderResult::Malformed instead of DecoderResult::InputEmpty
	malformedLen uint8 // the first field of DecoderResult::Malformed
	written      int   // number of UTF-16 code units written to dst
	dst          [2]uint16
}

func wrote(cp rune) decodeStep {
	if cp >= 0x10000 {
		cp -= 0x10000
		return decodeStep{written: 2, dst: [2]uint16{uint16(0xD800 + (cp >> 10)), uint16(0xDC00 + (cp & 0x3FF))}}
	}
	return decodeStep{written: 1, dst: [2]uint16{uint16(cp)}}
}

func malformedStep(length uint8) decodeStep {
	return decodeStep{malformed: true, malformedLen: length}
}

// Big5

type big5Decoder struct {
	lead byte // 0 when no lead is pending
}

func (d *big5Decoder) decodeByte(b byte) decodeStep {
	if d.lead != 0 {
		lead := d.lead
		d.lead = 0
		if (b >= 0x40 && b <= 0x7E) || (b >= 0xA1 && b <= 0xFE) {
			offset := byte(0x62)
			if b < 0x7F {
				offset = 0x40
			}
			pointer := int(lead-0x81)*157 + int(b-offset)
			switch pointer {
			case 1133:
				return decodeStep{written: 2, dst: [2]uint16{0x00CA, 0x0304}}
			case 1135:
				return decodeStep{written: 2, dst: [2]uint16{0x00CA, 0x030C}}
			case 1164:
				return decodeStep{written: 2, dst: [2]uint16{0x00EA, 0x0304}}
			case 1166:
				return decodeStep{written: 2, dst: [2]uint16{0x00EA, 0x030C}}
			}
			if pointer < len(indexBig5) && indexBig5[pointer] != 0 {
				return wrote(rune(indexBig5[pointer]))
			}
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	}
	if b < 0x80 {
		return wrote(rune(b))
	}
	if b >= 0x81 && b <= 0xFE {
		d.lead = b
		return decodeStep{}
	}
	return malformedStep(1)
}

func (d *big5Decoder) neutral() bool { return d.lead == 0 }

func (d *big5Decoder) finish() decodeStep {
	if d.lead != 0 {
		d.lead = 0
		return malformedStep(1)
	}
	return decodeStep{}
}

// EUC-KR

type eucKRDecoder struct {
	lead byte
}

func (d *eucKRDecoder) decodeByte(b byte) decodeStep {
	if d.lead != 0 {
		lead := d.lead
		d.lead = 0
		if b >= 0x41 && b <= 0xFE {
			pointer := int(lead-0x81)*190 + int(b-0x41)
			if pointer < len(indexEUCKR) && indexEUCKR[pointer] != 0 {
				return wrote(rune(indexEUCKR[pointer]))
			}
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	}
	if b < 0x80 {
		return wrote(rune(b))
	}
	if b >= 0x81 && b <= 0xFE {
		d.lead = b
		return decodeStep{}
	}
	return malformedStep(1)
}

func (d *eucKRDecoder) neutral() bool { return d.lead == 0 }

func (d *eucKRDecoder) finish() decodeStep {
	if d.lead != 0 {
		d.lead = 0
		return malformedStep(1)
	}
	return decodeStep{}
}

// Shift_JIS

type shiftJISDecoder struct {
	lead byte
}

func (d *shiftJISDecoder) decodeByte(b byte) decodeStep {
	if d.lead != 0 {
		lead := d.lead
		d.lead = 0
		if (b >= 0x40 && b <= 0x7E) || (b >= 0x80 && b <= 0xFC) {
			offset := byte(0x41)
			if b < 0x7F {
				offset = 0x40
			}
			leadOffset := byte(0xC1)
			if lead < 0xA0 {
				leadOffset = 0x81
			}
			pointer := int(lead-leadOffset)*188 + int(b-offset)
			if pointer >= 8836 && pointer <= 10715 {
				return wrote(rune(0xE000 - 8836 + pointer))
			}
			if pointer < len(indexJIS0208) && indexJIS0208[pointer] != 0 {
				return wrote(rune(indexJIS0208[pointer]))
			}
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	}
	if b <= 0x80 {
		return wrote(rune(b))
	}
	if b >= 0xA1 && b <= 0xDF {
		return wrote(0xFF61 - 0xA1 + rune(b))
	}
	if (b >= 0x81 && b <= 0x9F) || (b >= 0xE0 && b <= 0xFC) {
		d.lead = b
		return decodeStep{}
	}
	return malformedStep(1)
}

func (d *shiftJISDecoder) neutral() bool { return d.lead == 0 }

func (d *shiftJISDecoder) finish() decodeStep {
	if d.lead != 0 {
		d.lead = 0
		return malformedStep(1)
	}
	return decodeStep{}
}

// EUC-JP

type eucJPPending int

const (
	eucJPPendingNone eucJPPending = iota
	eucJPPendingJIS0208Lead
	eucJPPendingJIS0212Shift
	eucJPPendingJIS0212Lead
	eucJPPendingHalfWidthKatakana
)

type eucJPDecoder struct {
	pending eucJPPending
	lead    byte
}

func (d *eucJPDecoder) decodeByte(b byte) decodeStep {
	pending := d.pending
	d.pending = eucJPPendingNone
	switch pending {
	case eucJPPendingJIS0208Lead:
		if b >= 0xA1 && b <= 0xFE {
			pointer := int(d.lead-0xA1)*94 + int(b-0xA1)
			if indexJIS0208[pointer] != 0 {
				return wrote(rune(indexJIS0208[pointer]))
			}
			return malformedStep(2)
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	case eucJPPendingJIS0212Shift:
		if b >= 0xA1 && b <= 0xFE {
			d.lead = b
			d.pending = eucJPPendingJIS0212Lead
			return decodeStep{}
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	case eucJPPendingJIS0212Lead:
		if b >= 0xA1 && b <= 0xFE {
			pointer := int(d.lead-0xA1)*94 + int(b-0xA1)
			if pointer < len(indexJIS0212) && indexJIS0212[pointer] != 0 {
				return wrote(rune(indexJIS0212[pointer]))
			}
			return malformedStep(3)
		}
		if b < 0x80 {
			return malformedStep(2)
		}
		return malformedStep(3)
	case eucJPPendingHalfWidthKatakana:
		if b >= 0xA1 && b <= 0xDF {
			return wrote(0xFF61 - 0xA1 + rune(b))
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	}
	if b < 0x80 {
		return wrote(rune(b))
	}
	switch {
	case b >= 0xA1 && b <= 0xFE:
		d.lead = b
		d.pending = eucJPPendingJIS0208Lead
	case b == 0x8F:
		d.pending = eucJPPendingJIS0212Shift
	case b == 0x8E:
		d.pending = eucJPPendingHalfWidthKatakana
	default:
		return malformedStep(1)
	}
	return decodeStep{}
}

func (d *eucJPDecoder) neutral() bool { return d.pending == eucJPPendingNone }

func (d *eucJPDecoder) finish() decodeStep {
	pending := d.pending
	d.pending = eucJPPendingNone
	switch pending {
	case eucJPPendingJIS0208Lead, eucJPPendingJIS0212Shift, eucJPPendingHalfWidthKatakana:
		return malformedStep(1)
	case eucJPPendingJIS0212Lead:
		return malformedStep(2)
	}
	return decodeStep{}
}

// GBK (encoding_rs decodes GBK with its gb18030 decoder.)
//
// encoding_rs keeps an ASCII byte from a failed four-byte sequence as
// "pending ASCII" to emit on the next call. That state is not modeled because
// chardetng either rejects the GBK candidate or replaces the decoder after
// every Malformed result, so the pending byte is never observed.

type gb18030Decoder struct {
	first, second, third byte
	pending              int // number of bytes of a sequence seen so far
}

func gb18030RangesCodePoint(pointer int) (rune, bool) {
	if (pointer > 39419 && pointer < 189000) || pointer > 1237575 {
		return 0, false
	}
	if pointer >= 189000 {
		return rune(0x10000 + pointer - 189000), true
	}
	if pointer == 7457 {
		return 0xE7C7, true
	}
	i := len(indexGB18030Ranges) - 1
	for indexGB18030Ranges[i][0] > uint32(pointer) {
		i--
	}
	return rune(indexGB18030Ranges[i][1]) + rune(pointer) - rune(indexGB18030Ranges[i][0]), true
}

func (d *gb18030Decoder) decodeByte(b byte) decodeStep {
	pending := d.pending
	d.pending = 0
	switch pending {
	case 1:
		if b >= 0x30 && b <= 0x39 {
			d.second = b
			d.pending = 2
			return decodeStep{}
		}
		if (b >= 0x40 && b <= 0x7E) || (b >= 0x80 && b <= 0xFE) {
			offset := byte(0x41)
			if b < 0x7F {
				offset = 0x40
			}
			pointer := int(d.first-0x81)*190 + int(b-offset)
			return wrote(rune(indexGB18030[pointer]))
		}
		if b < 0x80 {
			return malformedStep(1)
		}
		return malformedStep(2)
	case 2:
		if b >= 0x81 && b <= 0xFE {
			d.third = b
			d.pending = 3
			return decodeStep{}
		}
		return malformedStep(1) // Malformed(1, 1) in encoding_rs
	case 3:
		if b >= 0x30 && b <= 0x39 {
			pointer := int(d.first-0x81)*(10*126*10) + int(d.second-0x30)*(10*126) +
				int(d.third-0x81)*10 + int(b-0x30)
			if cp, ok := gb18030RangesCodePoint(pointer); ok {
				return wrote(cp)
			}
			return malformedStep(4)
		}
		return malformedStep(1) // Malformed(1, 2) in encoding_rs
	}
	if b < 0x80 {
		return wrote(rune(b))
	}
	if b == 0x80 {
		return wrote(0x20AC)
	}
	if b <= 0xFE {
		d.first = b
		d.pending = 1
		return decodeStep{}
	}
	return malformedStep(1)
}

func (d *gb18030Decoder) neutral() bool { return d.pending == 0 }

func (d *gb18030Decoder) finish() decodeStep {
	pending := d.pending
	d.pending = 0
	if pending != 0 {
		return malformedStep(uint8(pending))
	}
	return decodeStep{}
}

// UTF-8 (validation only: chardetng only checks for Malformed)

type utf8Validator struct {
	needed, seen int
	lower, upper byte
}

// feed reports whether buffer, appended to the bytes fed so far, contains no
// UTF-8 error. When last is true, an incomplete sequence at the end is an
// error.
func (v *utf8Validator) feed(buffer []byte, last bool) bool {
	for i := 0; i < len(buffer); i++ {
		b := buffer[i]
		if v.needed == 0 {
			if b < 0x80 {
				i = asciiRunEnd(buffer, i) - 1
				continue
			}
			if rest := buffer[i:]; utf8.FullRune(rest) {
				r, size := utf8.DecodeRune(rest)
				if r == utf8.RuneError && size == 1 {
					return false
				}
				i += size - 1
				continue
			}
			v.lower, v.upper = 0x80, 0xBF
			switch {
			case b < 0x80:
			case b >= 0xC2 && b <= 0xDF:
				v.needed = 1
			case b >= 0xE0 && b <= 0xEF:
				switch b {
				case 0xE0:
					v.lower = 0xA0
				case 0xED:
					v.upper = 0x9F
				}
				v.needed = 2
			case b >= 0xF0 && b <= 0xF4:
				switch b {
				case 0xF0:
					v.lower = 0x90
				case 0xF4:
					v.upper = 0x8F
				}
				v.needed = 3
			default:
				return false
			}
			continue
		}
		if b < v.lower || b > v.upper {
			return false
		}
		v.lower, v.upper = 0x80, 0xBF
		v.seen++
		if v.seen == v.needed {
			v.needed, v.seen = 0, 0
		}
	}
	return !(last && v.needed != 0)
}

// ISO-2022-JP (validation only: chardetng only checks for Malformed)

type iso2022JPState int

const (
	iso2022JPASCII iso2022JPState = iota
	iso2022JPRoman
	iso2022JPKatakana
	iso2022JPLeadByte
	iso2022JPTrailByte
	iso2022JPEscapeStart
	iso2022JPEscape
)

type iso2022JPValidator struct {
	state       iso2022JPState
	outputState iso2022JPState
	lead        byte
	outputFlag  bool
}

// feed reports whether buffer, appended to the bytes fed so far, contains no
// ISO-2022-JP error. It follows encoding_rs's decoder, whose error conditions
// match the WHATWG Encoding Standard.
func (v *iso2022JPValidator) feed(buffer []byte, last bool) bool {
	for _, b := range buffer {
		switch v.state {
		case iso2022JPASCII, iso2022JPRoman:
			if b == 0x1B {
				v.state = iso2022JPEscapeStart
				continue
			}
			v.outputFlag = false
			if b > 0x7F || b == 0x0E || b == 0x0F {
				return false
			}
		case iso2022JPKatakana:
			if b == 0x1B {
				v.state = iso2022JPEscapeStart
				continue
			}
			v.outputFlag = false
			if b < 0x21 || b > 0x5F {
				return false
			}
		case iso2022JPLeadByte:
			if b == 0x1B {
				v.state = iso2022JPEscapeStart
				continue
			}
			v.outputFlag = false
			if b < 0x21 || b > 0x7E {
				return false
			}
			v.lead = b
			v.state = iso2022JPTrailByte
		case iso2022JPTrailByte:
			if b == 0x1B {
				return false
			}
			v.state = iso2022JPLeadByte
			if b < 0x21 || b > 0x7E {
				return false
			}
			if indexJIS0208[int(v.lead-0x21)*94+int(b-0x21)] == 0 {
				return false
			}
		case iso2022JPEscapeStart:
			if b != 0x24 && b != 0x28 {
				return false
			}
			v.lead = b
			v.state = iso2022JPEscape
		case iso2022JPEscape:
			var s iso2022JPState
			switch {
			case v.lead == 0x28 && b == 0x42:
				s = iso2022JPASCII
			case v.lead == 0x28 && b == 0x4A:
				s = iso2022JPRoman
			case v.lead == 0x28 && b == 0x49:
				s = iso2022JPKatakana
			case v.lead == 0x24 && (b == 0x40 || b == 0x42):
				s = iso2022JPLeadByte
			default:
				return false
			}
			v.lead = 0
			v.state = s
			v.outputState = s
			if v.outputFlag {
				// An escape sequence immediately following another one.
				return false
			}
			v.outputFlag = true
		}
	}
	if last {
		switch v.state {
		case iso2022JPTrailByte, iso2022JPEscapeStart, iso2022JPEscape:
			return false
		}
	}
	return true
}
