package chardet

type byteDecoder interface {
	decodeByte(b byte) decodeStep
}

type latinCJ int

const (
	latinCJASCIILetter latinCJ = iota
	latinCJCJ
	latinCJOther
)

type halfWidthKatakana int

const (
	dakutenForbidden halfWidthKatakana = iota
	dakutenAllowed
	dakutenOrHandakutenAllowed
)

type latinKorean int

const (
	latinKoreanASCIILetter latinKorean = iota
	latinKoreanHangul
	latinKoreanHanja
	latinKoreanOther
)

func isASCIILetterUnit(u uint16) bool {
	return (u >= 'a' && u <= 'z') || (u >= 'A' && u <= 'Z')
}

func isASCIILetterByte(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// extraSimplified, extraKanji and extraHangul map a code unit to chardetng's
// cjk_extra_score for the corresponding frequency table.
var extraSimplified, extraKanji, extraHangul [65536]uint8

func init() {
	for i := range 128 {
		score := uint8((128 - i) / 16)
		// The first occurrence wins, like Iterator::position.
		if extraSimplified[frequentSimplified[i]] == 0 {
			extraSimplified[frequentSimplified[i]] = score
		}
		if extraKanji[frequentKanji[i]] == 0 {
			extraKanji[frequentKanji[i]] = score
		}
		if extraHangul[frequentHangul[i]] == 0 {
			extraHangul[frequentHangul[i]] = score
		}
	}
}

// Shift_JIS and Big5
func problematicLead(b byte) bool {
	switch {
	case b >= 0x91 && b <= 0x97, b == 0x9A, b == 0x8A, b == 0x9B, b == 0x8B, b == 0x9E, b == 0x8E, b == 0xB0:
		return true
	}
	return false
}

// GBK and EUC-KR
func moreProblematicLead(b byte) bool {
	return problematicLead(b) || b == 0x82 || b == 0x84 || b == 0x85 || b == 0xA0
}

// pendingScore is the port of Option<i64>.
type pendingScore struct {
	value int64
	set   bool
}

// take returns the pending score, if any, and clears it.
func (p *pendingScore) take() int64 {
	v := p.value
	if !p.set {
		v = 0
	}
	*p = pendingScore{}
	return v
}

func (p *pendingScore) discard() {
	*p = pendingScore{}
}

type gbkCandidate struct {
	score    int64 // score accumulated by the current feed call
	decoder  gb18030Decoder
	prevByte byte
	prev     latinCJ
	pending  pendingScore
}

func (c *gbkCandidate) maybeSetAsPending(s int64) int64 {
	if c.prev == latinCJCJ || !moreProblematicLead(c.prevByte) {
		return s
	}
	c.pending = pendingScore{value: s, set: true}
	return 0
}

func (c *gbkCandidate) feed(buffer []byte, last bool) (int64, bool) {
	tables := gbkTables
	score := int64(0)
	prev := int(c.prev)
	prevByte := c.prevByte
	pendSet, pendVal := c.pending.set, c.pending.value
	neutral := c.decoder.neutral()
	for i := 0; i < len(buffer); {
		b := buffer[i]
		if neutral {
			e := tables.singles[b]
			n := 1
			if e == 0 && b >= 0x80 && i+1 < len(buffer) {
				e = tables.pairs[int(b)<<8|int(buffer[i+1])]
				n = 2
			}
			if e != 0 {
				score, prev, pendSet, pendVal = applyEntry(e, score, prev, pendSet, pendVal, int(latinCJCJ))
				last := buffer[i+n-1]
				prevByte = last
				i += n
				if b < 0x80 {
					// Bytes inside an ASCII run only reset state that the
					// first byte already reset, so only the first and the
					// last two bytes need processing.
					if j := asciiRunEnd(buffer, i); j-i >= 3 {
						i = j - 2
					}
				}
				continue
			}
		}
		c.score = score
		c.prev = latinCJ(prev)
		c.prevByte = prevByte
		c.pending = pendingScore{value: pendVal, set: pendSet}
		ok := c.step(b)
		neutral = c.decoder.neutral()
		score = c.score
		prev = int(c.prev)
		prevByte = c.prevByte
		pendSet, pendVal = c.pending.set, c.pending.value
		if !ok {
			return 0, false
		}
		i++
	}
	c.score = score
	c.prev = latinCJ(prev)
	c.prevByte = prevByte
	c.pending = pendingScore{value: pendVal, set: pendSet}
	if last && c.decoder.finish().malformed {
		return 0, false
	}
	return score, true
}

func (c *gbkCandidate) step(b byte) bool {
	step := c.decoder.decodeByte(b)
	if step.written == 1 && !step.malformed {
		c.stepUnit(b, step.dst[0])
		return true
	}
	score := c.score
	if step.written == 2 {
		score += c.pending.take()
		u := step.dst[0]
		if u >= 0xDB80 && u <= 0xDBFF {
			score += gbkPUAPenalty
			c.prev = latinCJOther
		} else if u >= 0xD480 && u < 0xD880 {
			score += gbkScorePerNonEUC
			if c.prev == latinCJASCIILetter {
				score += cjkLatinAdjacencyPenalty
			}
			c.prev = latinCJCJ
		} else {
			score += cjkOther
			c.prev = latinCJOther
		}
	}
	if step.malformed {
		if (c.prevByte == 0xA0 || c.prevByte == 0xFE || c.prevByte == 0xFD) && (b < 0x80 || b == 0xFF) {
			// Mac OS Chinese Simplified single-byte that conflicts with code page GBK lead byte
			// followed by ASCII or a non-conflicting single-byte extension.
			c.pending.discard() // Just in case
			score += gbkSingleByteExtensionPenalty
			if isASCIILetterByte(b) {
				c.prev = latinCJASCIILetter
			} else if b == 0xFF {
				score += gbkSingleByteExtensionPenalty
				c.prev = latinCJOther
			} else {
				c.prev = latinCJOther
			}
			// The GBK decoder has the pending ASCII concept, which is
			// a problem with this trickery, so let's reset the state.
			c.decoder = gb18030Decoder{}
		} else if step.malformedLen == 1 && b == 0xFF {
			// Mac OS Chinese Simplified single-byte extension that doesn't conflict with lead bytes
			c.pending.discard() // Just in case
			score += gbkSingleByteExtensionPenalty
			c.prev = latinCJOther
			// The GBK decoder has the pending ASCII concept, which is
			// a problem with this trickery, so let's reset the state.
			c.decoder = gb18030Decoder{}
		} else {
			c.score = score
			return false
		}
	}
	c.advance(b)
	c.score = score
	return true
}

// stepUnit is step for a byte that completes a character decoding to the
// single code unit u.
func (c *gbkCandidate) stepUnit(b byte, u uint16) {
	score := c.score
	if isASCIILetterUnit(u) {
		c.pending.discard()
		if c.prev == latinCJCJ {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJASCIILetter
	} else if u == 0x20AC {
		// euro sign
		c.pending.discard()
		// Should there even be a penalty?
		c.prev = latinCJOther
	} else if u >= 0x4E00 && u <= 0x9FA5 {
		score += c.pending.take()
		if b >= 0xA1 && b <= 0xFE {
			switch {
			case c.prevByte >= 0xA1 && c.prevByte <= 0xD7:
				score += gbkScorePerLevel1
				score += int64(extraSimplified[u])
			case c.prevByte >= 0xD8 && c.prevByte <= 0xFE:
				score += gbkScorePerLevel2
			default:
				score += gbkScorePerNonEUC
			}
		} else {
			score += c.maybeSetAsPending(gbkScorePerNonEUC)
		}
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if (u >= 0x3400 && u < 0xA000) || (u >= 0xF900 && u < 0xFB00) {
		score += c.pending.take()
		// XXX score?
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if u >= 0xE000 && u < 0xF900 {
		score += c.pending.take()
		// Treat the GB18030-required PUA mappings as non-EUC ideographs.
		switch {
		case u >= 0xE78D && u <= 0xE796, u >= 0xE816 && u <= 0xE818,
			u == 0xE81E, u == 0xE826, u == 0xE82B, u == 0xE82C, u == 0xE831,
			u == 0xE832, u == 0xE83B, u == 0xE843, u == 0xE854, u == 0xE855, u == 0xE864:
			score += gbkScorePerNonEUC
			if c.prev == latinCJASCIILetter {
				score += cjkLatinAdjacencyPenalty
			}
			c.prev = latinCJCJ
		default:
			score += gbkPUAPenalty
			c.prev = latinCJOther
		}
	} else {
		switch {
		case u == 0x3000, // Distinct from Korean, space
			u == 0x3001, // Distinct from Korean, enumeration comma
			u == 0x3002, // Distinct from Korean, full stop
			u == 0xFF08, // Distinct from Korean, parenthesis
			u == 0xFF09, // Distinct from Korean, parenthesis
			u == 0xFF01, // Distinct from Japanese, exclamation
			u == 0xFF0C, // Distinct from Japanese, comma
			u == 0xFF1B, // Distinct from Japanese, semicolon
			u == 0xFF1F: // Distinct from Japanese, question
			score += c.pending.take()
			score += cjPunctuation
		case u <= 0x7F:
			c.pending.discard()
		default:
			score += c.pending.take()
			score += cjkOther
		}
		c.prev = latinCJOther
	}
	c.score = score
	c.advance(b)
}

// advance records b as the last byte processed.
func (c *gbkCandidate) advance(b byte) {
	c.prevByte = b
}

type shiftJISCandidate struct {
	score                  int64 // score accumulated by the current feed call
	decoder                shiftJISDecoder
	halfWidthKatakanaSeen  bool
	halfWidthKatakanaState halfWidthKatakana
	prev                   latinCJ
	prevByte               byte
	pending                pendingScore
}

func (c *shiftJISCandidate) maybeSetAsPending(s int64) int64 {
	if c.prev == latinCJCJ || !problematicLead(c.prevByte) {
		return s
	}
	c.pending = pendingScore{value: s, set: true}
	return 0
}

func (c *shiftJISCandidate) feed(buffer []byte, last bool) (int64, bool) {
	tables := shiftJISTables
	score := int64(0)
	prev := int(c.prev)
	prevByte := c.prevByte
	pendSet, pendVal := c.pending.set, c.pending.value
	hw := c.halfWidthKatakanaState
	hwSeen := c.halfWidthKatakanaSeen
	neutral := c.decoder.neutral()
	for i := 0; i < len(buffer); {
		b := buffer[i]
		if neutral {
			e := tables.singles[b]
			n := 1
			if e == 0 && b >= 0x80 && i+1 < len(buffer) {
				e = tables.pairs[int(b)<<8|int(buffer[i+1])]
				n = 2
			}
			nextHW := dakutenForbidden
			if e == 0 && b >= 0xA1 && b <= 0xDF {
				seen := 0
				if hwSeen {
					seen = 1
				}
				e = tables.halfWidth[b-0xA1][hw][seen]
				nextHW = tables.halfWidthNext[b-0xA1]
				n = 1
			}
			if e != 0 {
				hw = nextHW
				hwSeen = hwSeen || b >= 0xA1 && b <= 0xDF
				score, prev, pendSet, pendVal = applyEntry(e, score, prev, pendSet, pendVal, int(latinCJCJ))
				last := buffer[i+n-1]
				prevByte = last
				i += n
				if b < 0x80 {
					// Bytes inside an ASCII run only reset state that the
					// first byte already reset, so only the first and the
					// last two bytes need processing.
					if j := asciiRunEnd(buffer, i); j-i >= 3 {
						i = j - 2
					}
				}
				continue
			}
		}
		c.score = score
		c.prev = latinCJ(prev)
		c.prevByte = prevByte
		c.pending = pendingScore{value: pendVal, set: pendSet}
		c.halfWidthKatakanaState = hw
		c.halfWidthKatakanaSeen = hwSeen
		ok := c.step(b)
		neutral = c.decoder.neutral()
		score = c.score
		prev = int(c.prev)
		prevByte = c.prevByte
		pendSet, pendVal = c.pending.set, c.pending.value
		hw = c.halfWidthKatakanaState
		hwSeen = c.halfWidthKatakanaSeen
		if !ok {
			return 0, false
		}
		i++
	}
	c.score = score
	c.prev = latinCJ(prev)
	c.prevByte = prevByte
	c.pending = pendingScore{value: pendVal, set: pendSet}
	c.halfWidthKatakanaState = hw
	c.halfWidthKatakanaSeen = hwSeen
	if last && c.decoder.finish().malformed {
		return 0, false
	}
	return score, true
}

func (c *shiftJISCandidate) step(b byte) bool {
	step := c.decoder.decodeByte(b)
	if step.written == 1 && !step.malformed {
		c.stepUnit(b, step.dst[0])
		return true
	}
	score := c.score
	if step.malformed {
		p := c.prevByte
		if ((p >= 0x81 && p <= 0x9F) || (p >= 0xE0 && p <= 0xFC)) &&
			((b >= 0x40 && b <= 0x7E) || (b >= 0x80 && b <= 0xFC)) &&
			!((p == 0x82 && b >= 0xFA) ||
				(p == 0x84 && ((b >= 0xDD && b <= 0xE4) || b >= 0xFB)) ||
				(p == 0x86 && b >= 0xF2 && b <= 0xFA) ||
				(p == 0x87 && b >= 0x77 && b <= 0x7D) ||
				(p == 0xFC && b >= 0xF5)) {
			// Shift_JIS2004 or MacJapanese
			score += c.pending.take()
			score += shiftJISExtensionPenalty
			// Approximate boundary
			if p < 0x87 {
				c.prev = latinCJOther
			} else {
				if c.prev == latinCJASCIILetter {
					score += cjkLatinAdjacencyPenalty
				}
				c.prev = latinCJCJ
			}
		} else if step.malformedLen == 1 && (b == 0xA0 || b >= 0xFD) {
			c.pending.discard() // Just in case
			score += shiftJISSingleByteExtensionPenalty
			c.prev = latinCJOther
		} else {
			c.score = score
			return false
		}
	}
	c.advance(b)
	c.score = score
	return true
}

// stepUnit is step for a byte that completes a character decoding to the
// single code unit u.
func (c *shiftJISCandidate) stepUnit(b byte, u uint16) {
	score := c.score
	halfWidthKatakanaState := c.halfWidthKatakanaState
	c.halfWidthKatakanaState = dakutenForbidden
	if isASCIILetterUnit(u) {
		c.pending.discard()
		if c.prev == latinCJCJ {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJASCIILetter
	} else if u >= 0xFF61 && u <= 0xFF9F {
		if !c.halfWidthKatakanaSeen {
			c.halfWidthKatakanaSeen = true
			// To avoid misdetecting title-length inputs
			score += shiftJISInitialHalfWidthKatakanaPenalty
		}
		c.pending.discard()
		score += halfWidthKatakanaScore

		if (u >= 0xFF76 && u <= 0xFF84) || u == 0xFF73 {
			c.halfWidthKatakanaState = dakutenAllowed
		} else if u >= 0xFF8A && u <= 0xFF8E {
			c.halfWidthKatakanaState = dakutenOrHandakutenAllowed
		} else if u == 0xFF9E {
			if halfWidthKatakanaState == dakutenForbidden {
				score += implausibilityPenalty
			} else {
				score += halfWidthKatakanaVoicingScore
			}
		} else if u == 0xFF9F {
			if halfWidthKatakanaState != dakutenOrHandakutenAllowed {
				score += implausibilityPenalty
			} else {
				score += halfWidthKatakanaVoicingScore
			}
		}

		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if u >= 0x3040 && u < 0x3100 {
		score += c.pending.take()
		score += shiftJISScorePerKana
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if (u >= 0x3400 && u < 0xA000) || (u >= 0xF900 && u < 0xFB00) {
		score += c.pending.take()
		if c.prevByte < 0x98 || (c.prevByte == 0x98 && b < 0x73) {
			score += c.maybeSetAsPending(shiftJISScorePerLevel1Kanji + int64(extraKanji[u]))
		} else {
			score += c.maybeSetAsPending(shiftJISScorePerLevel2Kanji)
		}
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if u >= 0xE000 && u < 0xF900 {
		score += c.pending.take()
		score += shiftJISPUAPenalty
		c.prev = latinCJOther
	} else {
		switch {
		case u == 0x3000, // Distinct from Korean, space
			u == 0x3001, // Distinct from Korean, enumeration comma
			u == 0x3002, // Distinct from Korean, full stop
			u == 0xFF08, // Distinct from Korean, parenthesis
			u == 0xFF09: // Distinct from Korean, parenthesis
			score += c.pending.take()
			// Not really needed for CJK distinction
			// but let's give non-zero score for these
			// common byte pairs anyway.
			score += cjPunctuation
		case u <= 0x7F:
			c.pending.discard()
		case u == 0x80:
			// This is a control character that overlaps euro
			// in windows-1252 and happens to be a non-error
			// is Shift_JIS.
			c.pending.discard()
			score += implausibilityPenalty
		default:
			score += c.pending.take()
			score += cjkOther
		}
		c.prev = latinCJOther
	}
	c.score = score
	c.advance(b)
}

// advance records b as the last byte processed.
func (c *shiftJISCandidate) advance(b byte) {
	c.prevByte = b
}

type eucJPCandidate struct {
	score                  int64 // score accumulated by the current feed call
	decoder                eucJPDecoder
	nonASCIISeen           bool
	halfWidthKatakanaState halfWidthKatakana
	prev                   latinCJ
	prevByte               byte
	prevPrevByte           byte
}

func (c *eucJPCandidate) feed(buffer []byte, last bool) (int64, bool) {
	tables := eucJPTables
	score := int64(0)
	prev := int(c.prev)
	prevByte := c.prevByte
	var pendSet bool
	var pendVal int64
	hw := c.halfWidthKatakanaState
	prevPrevByte := c.prevPrevByte
	seen := c.nonASCIISeen
	neutral := c.decoder.neutral()
	for i := 0; i < len(buffer); {
		b := buffer[i]
		if neutral && prevByte != 0x8F {
			e := tables.singles[b]
			n := 1
			if e == 0 && b >= 0x80 && i+1 < len(buffer) {
				e = tables.pairs[int(b)<<8|int(buffer[i+1])]
				n = 2
			}
			if e != 0 {
				if n == 2 {
					prevByte = b
				}
				hw = dakutenForbidden
				score, seen = applyJapanese(e, score, seen)
				score, prev, pendSet, pendVal = applyEntry(e, score, prev, pendSet, pendVal, int(latinCJCJ))
				last := buffer[i+n-1]
				prevPrevByte, prevByte = prevByte, last
				i += n
				if b < 0x80 {
					// Bytes inside an ASCII run only reset state that the
					// first byte already reset, so only the first and the
					// last two bytes need processing.
					if j := asciiRunEnd(buffer, i); j-i >= 3 {
						i = j - 2
					}
				}
				continue
			}
		}
		c.score = score
		c.prev = latinCJ(prev)
		c.prevByte = prevByte
		c.halfWidthKatakanaState = hw
		c.prevPrevByte = prevPrevByte
		c.nonASCIISeen = seen
		ok := c.step(b)
		neutral = c.decoder.neutral()
		score = c.score
		prev = int(c.prev)
		prevByte = c.prevByte
		hw = c.halfWidthKatakanaState
		prevPrevByte = c.prevPrevByte
		seen = c.nonASCIISeen
		if !ok {
			return 0, false
		}
		i++
	}
	c.score = score
	c.prev = latinCJ(prev)
	c.prevByte = prevByte
	c.halfWidthKatakanaState = hw
	c.prevPrevByte = prevPrevByte
	c.nonASCIISeen = seen
	if last && c.decoder.finish().malformed {
		return 0, false
	}
	return score, true
}

func (c *eucJPCandidate) step(b byte) bool {
	step := c.decoder.decodeByte(b)
	if step.written == 1 && !step.malformed {
		c.stepUnit(b, step.dst[0])
		return true
	}
	score := c.score
	if step.malformed {
		p, pp := c.prevByte, c.prevPrevByte
		if b >= 0xA1 && b <= 0xFE && p >= 0xA1 && p <= 0xFE &&
			((pp != 0x8F &&
				!(p == 0xA8 && b >= 0xDF && b <= 0xE6) &&
				!(p == 0xAC && b >= 0xF4 && b <= 0xFC) &&
				!(p == 0xAD && b >= 0xD8 && b <= 0xDE)) ||
				(pp == 0x8F &&
					p != 0xA2 && p != 0xA6 && p != 0xA7 && p != 0xA9 &&
					p != 0xAA && p != 0xAB && p != 0xED &&
					!(p == 0xFE && b >= 0xF7))) {
			score += eucJPExtensionPenalty
			if c.prev == latinCJASCIILetter {
				score += cjkLatinAdjacencyPenalty
			}
			c.prev = latinCJCJ
		} else {
			c.score = score
			return false
		}
	}
	c.advance(b)
	c.score = score
	return true
}

// stepUnit is step for a byte that completes a character decoding to the
// single code unit u.
func (c *eucJPCandidate) stepUnit(b byte, u uint16) {
	score := c.score
	halfWidthKatakanaState := c.halfWidthKatakanaState
	c.halfWidthKatakanaState = dakutenForbidden
	if !c.nonASCIISeen && u >= 0x80 {
		c.nonASCIISeen = true
		if u >= 0x3040 && u < 0x3100 {
			// Remove the kana advantage over initial Big5
			// hanzi.
			score += eucJPInitialKanaPenalty
		}
	}
	if isASCIILetterUnit(u) {
		if c.prev == latinCJCJ {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJASCIILetter
	} else if u >= 0xFF61 && u <= 0xFF9F {
		score += halfWidthKatakanaScore

		if (u >= 0xFF76 && u <= 0xFF84) || u == 0xFF73 {
			c.halfWidthKatakanaState = dakutenAllowed
		} else if u >= 0xFF8A && u <= 0xFF8E {
			c.halfWidthKatakanaState = dakutenOrHandakutenAllowed
		} else if u == 0xFF9E {
			if halfWidthKatakanaState == dakutenForbidden {
				score += implausibilityPenalty
			} else {
				score += halfWidthKatakanaVoicingScore
			}
		} else if u == 0xFF9F {
			if halfWidthKatakanaState != dakutenOrHandakutenAllowed {
				score += implausibilityPenalty
			} else {
				score += halfWidthKatakanaVoicingScore
			}
		}

		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJOther
	} else if (u >= 0x3041 && u <= 0x3093) || (u >= 0x30A1 && u <= 0x30F6) {
		switch u {
		case 0x3090, // hiragana wi
			0x3091, // hiragana we
			0x30F0, // katakana wi
			0x30F1: // katakana we
			// Remove advantage over Big5 Hanzi
			score += eucJPScorePerNearObsoleteKana
		default:
			score += eucJPScorePerKana
		}
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else if (u >= 0x3400 && u < 0xA000) || (u >= 0xF900 && u < 0xFB00) {
		if c.prevPrevByte == 0x8F {
			score += eucJPScorePerOtherKanji
		} else if c.prevByte < 0xD0 {
			score += eucJPScorePerLevel1Kanji
			score += int64(extraKanji[u])
		} else {
			score += eucJPScorePerLevel2Kanji
		}
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else {
		switch {
		case u == 0x3000, // Distinct from Korean, space
			u == 0x3001, // Distinct from Korean, enumeration comma
			u == 0x3002, // Distinct from Korean, full stop
			u == 0xFF08, // Distinct from Korean, parenthesis
			u == 0xFF09: // Distinct from Korean, parenthesis
			score += cjPunctuation
		case u <= 0x7F:
		default:
			score += cjkOther
		}
		c.prev = latinCJOther
	}
	c.score = score
	c.advance(b)
}

// advance records b as the last byte processed.
func (c *eucJPCandidate) advance(b byte) {
	c.prevPrevByte = c.prevByte
	c.prevByte = b
}

type big5Candidate struct {
	score    int64 // score accumulated by the current feed call
	decoder  big5Decoder
	prev     latinCJ
	prevByte byte
	pending  pendingScore
}

func (c *big5Candidate) maybeSetAsPending(s int64) int64 {
	if c.prev == latinCJCJ || !problematicLead(c.prevByte) {
		return s
	}
	c.pending = pendingScore{value: s, set: true}
	return 0
}

func (c *big5Candidate) feed(buffer []byte, last bool) (int64, bool) {
	tables := big5Tables
	score := int64(0)
	prev := int(c.prev)
	prevByte := c.prevByte
	pendSet, pendVal := c.pending.set, c.pending.value
	neutral := c.decoder.neutral()
	for i := 0; i < len(buffer); {
		b := buffer[i]
		if neutral {
			e := tables.singles[b]
			n := 1
			if e == 0 && b >= 0x80 && i+1 < len(buffer) {
				e = tables.pairs[int(b)<<8|int(buffer[i+1])]
				n = 2
			}
			if e != 0 {
				score, prev, pendSet, pendVal = applyEntry(e, score, prev, pendSet, pendVal, int(latinCJCJ))
				last := buffer[i+n-1]
				prevByte = last
				i += n
				if b < 0x80 {
					// Bytes inside an ASCII run only reset state that the
					// first byte already reset, so only the first and the
					// last two bytes need processing.
					if j := asciiRunEnd(buffer, i); j-i >= 3 {
						i = j - 2
					}
				}
				continue
			}
		}
		c.score = score
		c.prev = latinCJ(prev)
		c.prevByte = prevByte
		c.pending = pendingScore{value: pendVal, set: pendSet}
		ok := c.step(b)
		neutral = c.decoder.neutral()
		score = c.score
		prev = int(c.prev)
		prevByte = c.prevByte
		pendSet, pendVal = c.pending.set, c.pending.value
		if !ok {
			return 0, false
		}
		i++
	}
	c.score = score
	c.prev = latinCJ(prev)
	c.prevByte = prevByte
	c.pending = pendingScore{value: pendVal, set: pendSet}
	if last && c.decoder.finish().malformed {
		return 0, false
	}
	return score, true
}

func (c *big5Candidate) step(b byte) bool {
	step := c.decoder.decodeByte(b)
	if step.written == 1 && !step.malformed {
		c.stepUnit(b, step.dst[0])
		return true
	}
	score := c.score
	if step.written == 2 {
		score += c.pending.take()
		if step.dst[0] == 0xCA || step.dst[0] == 0xEA {
			score += cjkOther
			c.prev = latinCJOther
		} else {
			score += c.maybeSetAsPending(big5ScorePerOtherHanzi)
			if c.prev == latinCJASCIILetter {
				score += cjkLatinAdjacencyPenalty
			}
			c.prev = latinCJCJ
		}
	}
	if step.malformed {
		if c.prevByte >= 0x81 && c.prevByte <= 0xFE && ((b >= 0x40 && b <= 0x7E) || (b >= 0xA1 && b <= 0xFE)) {
			// The byte pair is in the Big5 range but unmapped.
			// Treat as PUA to avoid rejecting Big5-UAO, etc.
			// We don't reprocess `b` even if ASCII, since it's
			// logically part of the pair.
			score += c.pending.take()
			score += big5PUAPenalty
			// Assume Hanzi semantics
			if c.prev == latinCJASCIILetter {
				score += cjkLatinAdjacencyPenalty
			}
			c.prev = latinCJCJ
		} else if (c.prevByte == 0xA0 || c.prevByte == 0xFD || c.prevByte == 0xFE) && (b < 0x80 || b == 0xFF) {
			// Mac OS Chinese Traditional single-byte that conflicts with code page Big5 lead byte
			// followed by ASCII or a non-conflicting single-byte extension.
			c.pending.discard() // Just in case
			score += big5SingleByteExtensionPenalty
			if isASCIILetterByte(b) {
				c.prev = latinCJASCIILetter
			} else if b == 0xFF {
				score += big5SingleByteExtensionPenalty
				c.prev = latinCJOther
			} else {
				c.prev = latinCJOther
			}
		} else if step.malformedLen == 1 && b == 0xFF {
			// Mac OS Chinese Traditional single-byte extension that doesn't conflict with lead bytes
			c.pending.discard() // Just in case
			score += big5SingleByteExtensionPenalty
			c.prev = latinCJOther
		} else {
			c.score = score
			return false
		}
	}
	c.advance(b)
	c.score = score
	return true
}

// stepUnit is step for a byte that completes a character decoding to the
// single code unit u.
func (c *big5Candidate) stepUnit(b byte, u uint16) {
	score := c.score
	if isASCIILetterUnit(u) {
		c.pending.discard()
		if c.prev == latinCJCJ {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJASCIILetter
	} else if (u >= 0x3400 && u < 0xA000) || (u >= 0xF900 && u < 0xFB00) {
		score += c.pending.take()
		if c.prevByte >= 0xA4 && c.prevByte <= 0xC6 {
			score += c.maybeSetAsPending(big5ScorePerLevel1Hanzi)
		} else {
			score += c.maybeSetAsPending(big5ScorePerOtherHanzi)
		}
		if c.prev == latinCJASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinCJCJ
	} else {
		switch {
		case u == 0x3000, // Distinct from Korean, space
			u == 0x3001, // Distinct from Korean, enumeration comma
			u == 0x3002, // Distinct from Korean, full stop
			u == 0xFF08, // Distinct from Korean, parenthesis
			u == 0xFF09, // Distinct from Korean, parenthesis
			u == 0xFF01, // Distinct from Japanese, exclamation
			u == 0xFF0C, // Distinct from Japanese, comma
			u == 0xFF1B, // Distinct from Japanese, semicolon
			u == 0xFF1F: // Distinct from Japanese, question
			score += c.pending.take()
			// Not really needed for CJK distinction
			// but let's give non-zero score for these
			// common byte pairs anyway.
			score += cjPunctuation
		case u <= 0x7F:
			c.pending.discard()
		default:
			score += c.pending.take()
			score += cjkOther
		}
		c.prev = latinCJOther
	}
	c.score = score
	c.advance(b)
}

// advance records b as the last byte processed.
func (c *big5Candidate) advance(b byte) {
	c.prevByte = b
}

type eucKRCandidate struct {
	score           int64 // score accumulated by the current feed call
	decoder         eucKRDecoder
	prevByte        byte
	prevWasEUCRange bool
	prev            latinKorean
	currentWordLen  uint64
	pending         pendingScore
}

func (c *eucKRCandidate) maybeSetAsPending(s int64) int64 {
	if c.prev == latinKoreanHangul || !moreProblematicLead(c.prevByte) {
		return s
	}
	c.pending = pendingScore{value: s, set: true}
	return 0
}

func (c *eucKRCandidate) feed(buffer []byte, last bool) (int64, bool) {
	tables := eucKRTables
	score := int64(0)
	prev := int(c.prev)
	prevByte := c.prevByte
	pendSet, pendVal := c.pending.set, c.pending.value
	wordLen := c.currentWordLen
	neutral := c.decoder.neutral()
	for i := 0; i < len(buffer); {
		b := buffer[i]
		if neutral {
			e := tables.singles[b]
			n := 1
			if e == 0 && b >= 0x80 && i+1 < len(buffer) {
				e = tables.pairs[int(b)<<8|int(buffer[i+1])]
				n = 2
			}
			if e != 0 {
				score, wordLen = applyKorean(e, score, prev, wordLen)
				score, prev, pendSet, pendVal = applyEntry(e, score, prev, pendSet, pendVal, int(latinKoreanHangul))
				last := buffer[i+n-1]
				prevByte = last
				i += n
				if b < 0x80 {
					// Bytes inside an ASCII run only reset state that the
					// first byte already reset, so only the first and the
					// last two bytes need processing.
					if j := asciiRunEnd(buffer, i); j-i >= 3 {
						i = j - 2
					}
				}
				continue
			}
		}
		c.score = score
		c.prev = latinKorean(prev)
		c.prevByte = prevByte
		c.pending = pendingScore{value: pendVal, set: pendSet}
		c.currentWordLen = wordLen
		c.prevWasEUCRange = prevByte >= 0xA1 && prevByte <= 0xFE
		ok := c.step(b)
		neutral = c.decoder.neutral()
		score = c.score
		prev = int(c.prev)
		prevByte = c.prevByte
		pendSet, pendVal = c.pending.set, c.pending.value
		wordLen = c.currentWordLen
		if !ok {
			return 0, false
		}
		i++
	}
	c.score = score
	c.prev = latinKorean(prev)
	c.prevByte = prevByte
	c.pending = pendingScore{value: pendVal, set: pendSet}
	c.currentWordLen = wordLen
	c.prevWasEUCRange = prevByte >= 0xA1 && prevByte <= 0xFE
	if last && c.decoder.finish().malformed {
		return 0, false
	}
	return score, true
}

func (c *eucKRCandidate) step(b byte) bool {
	step := c.decoder.decodeByte(b)
	if step.written == 1 && !step.malformed {
		c.stepUnit(b, step.dst[0])
		return true
	}
	score := c.score
	if step.malformed {
		p := c.prevByte
		if (p == 0xC9 || p == 0xFE) && b >= 0xA1 && b <= 0xFE {
			score += c.pending.take()
			// The byte pair is in code page 949 EUDC range
			score += eucKRPUAPenalty
			// Assume Hanja semantics
			switch c.prev {
			case latinKoreanASCIILetter:
				score += cjkLatinAdjacencyPenalty
			case latinKoreanHangul:
				score += eucKRHanjaAfterHangulPenalty
			}
			c.prev = latinKoreanHanja
			c.currentWordLen++
			if c.currentWordLen > 5 {
				score += eucKRLongWordPenalty
			}
		} else if (p == 0xA1 || (p >= 0xA3 && p <= 0xA8) || (p >= 0xAA && p <= 0xAD)) && (b >= 0x7B && b <= 0x7D) {
			score += c.pending.take()
			// MacKorean symbols in range not part of code page 949
			score += eucKRMacKoreanPenalty
			c.prev = latinKoreanOther
			c.currentWordLen = 0
		} else if (p >= 0x81 && p <= 0x84) && (b <= 0x80 || b == 0xFF) {
			// MacKorean single-byte that conflicts with code page 949 lead byte
			// followed by ASCII or a non-conflicting single-byte extension.
			c.pending.discard() // Just in case
			score += eucKRSingleByteExtensionPenalty
			if isASCIILetterByte(b) {
				c.prev = latinKoreanASCIILetter
			} else if b == 0x80 || b == 0xFF {
				score += eucKRSingleByteExtensionPenalty
				c.prev = latinKoreanOther
			} else {
				c.prev = latinKoreanOther
			}
			c.currentWordLen = 0
		} else if step.malformedLen == 1 && (b == 0x80 || b == 0xFF) {
			// MacKorean single-byte extensions that don't conflict with lead bytes
			c.pending.discard() // Just in case
			score += eucKRSingleByteExtensionPenalty
			c.prev = latinKoreanOther
			c.currentWordLen = 0
		} else {
			c.score = score
			return false
		}
	}
	c.advance(b)
	c.score = score
	return true
}

// stepUnit is step for a byte that completes a character decoding to the
// single code unit u.
func (c *eucKRCandidate) stepUnit(b byte, u uint16) {
	inEUCRange := b >= 0xA1 && b <= 0xFE
	score := c.score
	if isASCIILetterUnit(u) {
		c.pending.discard()
		switch c.prev {
		case latinKoreanHangul, latinKoreanHanja:
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinKoreanASCIILetter
		c.currentWordLen = 0
	} else if u >= 0xAC00 && u <= 0xD7A3 {
		score += c.pending.take()
		if c.prevWasEUCRange && inEUCRange {
			score += eucKRScorePerEUCHangul
			score += int64(extraHangul[u])
		} else {
			score += c.maybeSetAsPending(eucKRScorePerNonEUCHangul)
		}
		if c.prev == latinKoreanASCIILetter {
			score += cjkLatinAdjacencyPenalty
		}
		c.prev = latinKoreanHangul
		c.currentWordLen++
		if c.currentWordLen > 5 {
			score += eucKRLongWordPenalty
		}
	} else if (u >= 0x4E00 && u < 0xAC00) || (u >= 0xF900 && u <= 0xFA0B) {
		score += c.pending.take()
		score += eucKRScorePerHanja
		switch c.prev {
		case latinKoreanASCIILetter:
			score += cjkLatinAdjacencyPenalty
		case latinKoreanHangul:
			score += eucKRHanjaAfterHangulPenalty
		}
		c.prev = latinKoreanHanja
		c.currentWordLen++
		if c.currentWordLen > 5 {
			score += eucKRLongWordPenalty
		}
	} else {
		if u >= 0x80 {
			score += c.pending.take()
			score += cjkOther
		} else {
			c.pending.discard()
		}
		c.prev = latinKoreanOther
		c.currentWordLen = 0
	}
	c.score = score
	c.advance(b)
}

// advance records b as the last byte processed.
func (c *eucKRCandidate) advance(b byte) {
	c.prevWasEUCRange = b >= 0xA1 && b <= 0xFE
	c.prevByte = b
}
