package chardet

import "sync"

// A cjkEntry describes the complete effect of one character on a CJK
// candidate, so that the candidate's feed loop can skip both the decoder and
// the scoring branches. Entries are derived by probing the candidate's
// stepUnit, which is the scoring code of chardetng, from canonical states.
// Characters whose effect does not fit this form (e.g., half-width katakana,
// whose score depends on the previous character) get the entry 0 and take
// the generic path.
//
// Layout:
//
//	bits  0-15  score s (int16)
//	bits 16-17  next value of prev
//	bit  18     s becomes the pending score unless prev is the blocking value
//	bit  19     the pending score is discarded (otherwise it is added)
//	bits 20-23  add cjkLatinAdjacencyPenalty if bit prev is set
//	bits 24-27  add eucKRHanjaAfterHangulPenalty if bit prev is set
//	bit  28     EUC-KR: the character extends the word (otherwise resets it)
//	bit  29     EUC-JP: the character is non-ASCII
//	bit  30     EUC-JP: the character is kana
//	bit  31     the entry is valid
type cjkEntry uint32

const (
	entryPendable cjkEntry = 1 << 18
	entryDiscard  cjkEntry = 1 << 19
	entryWordInc  cjkEntry = 1 << 28
	entryNonASCII cjkEntry = 1 << 29
	entryKana     cjkEntry = 1 << 30
	entryValid    cjkEntry = 1 << 31
)

// applyEntry applies the parts of e that are common to all CJK candidates.
// block is the value of prev that prevents a score from becoming pending. It
// is the same value for every candidate today, but names different states.
//
//nolint:unparam
func applyEntry(e cjkEntry, score int64, prev int, pendSet bool, pendVal int64, block int) (int64, int, bool, int64) {
	if e&entryDiscard != 0 {
		pendSet = false
	} else if pendSet {
		score += pendVal
		pendSet = false
	}
	s := int64(int16(e))
	if e&entryPendable != 0 && prev != block {
		pendSet, pendVal = true, s
	} else {
		score += s
	}
	if (e>>20)&(1<<prev) != 0 {
		score += cjkLatinAdjacencyPenalty
	}
	return score, int(e>>16) & 3, pendSet, pendVal
}

// applyKorean applies the EUC-KR specific parts of e. prev is the value
// before the character.
func applyKorean(e cjkEntry, score int64, prev int, wordLen uint64) (int64, uint64) {
	if (e>>24)&(1<<prev) != 0 {
		score += eucKRHanjaAfterHangulPenalty
	}
	if e&entryWordInc != 0 {
		wordLen++
		if wordLen > 5 {
			score += eucKRLongWordPenalty
		}
	} else {
		wordLen = 0
	}
	return score, wordLen
}

// applyJapanese applies the EUC-JP specific parts of e.
func applyJapanese(e cjkEntry, score int64, nonASCIISeen bool) (int64, bool) {
	if e&entryNonASCII != 0 && !nonASCIISeen {
		nonASCIISeen = true
		if e&entryKana != 0 {
			score += eucJPInitialKanaPenalty
		}
	}
	return score, nonASCIISeen
}

// probeState is the part of a CJK candidate's state that stepUnit reads or
// writes.
type probeState struct {
	prev         int
	pendSet      bool
	pendVal      int64
	wordLen      uint64
	nonASCIISeen bool
	hw           halfWidthKatakana
	hwSeen       bool
	prevByte     byte
	prevPrevByte byte
}

// cjkKind describes one CJK candidate for entry derivation.
type cjkKind struct {
	numPrev  int
	block    int // prev value that prevents pending scores
	korean   bool
	japanese bool // EUC-JP (non-ASCII and kana flags)
	// probe runs the candidate from st over one character: the optional
	// lead byte (lead < 0 for none) and the byte b that completes the
	// character as u. If full is true, probe runs the candidate's step on
	// both bytes instead, and ok reports whether the candidate survives with
	// its decoder back in the initial state.
	probe func(st probeState, lead int, b byte, u uint16, full bool) (score int64, out probeState, ok bool)
	// newDecoder returns a decoder in its initial state.
	newDecoder func() byteDecoder
}

func probeGBK(st probeState, lead int, b byte, u uint16, full bool) (int64, probeState, bool) {
	c := gbkCandidate{prev: latinCJ(st.prev), pending: pendingScore{st.pendVal, st.pendSet}, prevByte: st.prevByte}
	if full {
		if !c.step(byte(lead)) || !c.step(b) || !c.decoder.neutral() {
			return 0, st, false
		}
	} else {
		if lead >= 0 {
			c.advance(byte(lead))
		}
		c.stepUnit(b, u)
	}
	st.prev, st.pendSet, st.pendVal, st.prevByte = int(c.prev), c.pending.set, c.pending.value, c.prevByte
	return c.score, st, true
}

func probeBig5(st probeState, lead int, b byte, u uint16, full bool) (int64, probeState, bool) {
	c := big5Candidate{prev: latinCJ(st.prev), pending: pendingScore{st.pendVal, st.pendSet}, prevByte: st.prevByte}
	if full {
		if !c.step(byte(lead)) || !c.step(b) || !c.decoder.neutral() {
			return 0, st, false
		}
	} else {
		if lead >= 0 {
			c.advance(byte(lead))
		}
		c.stepUnit(b, u)
	}
	st.prev, st.pendSet, st.pendVal, st.prevByte = int(c.prev), c.pending.set, c.pending.value, c.prevByte
	return c.score, st, true
}

func probeShiftJIS(st probeState, lead int, b byte, u uint16, full bool) (int64, probeState, bool) {
	c := shiftJISCandidate{
		prev: latinCJ(st.prev), pending: pendingScore{st.pendVal, st.pendSet}, prevByte: st.prevByte,
		halfWidthKatakanaState: st.hw, halfWidthKatakanaSeen: st.hwSeen,
	}
	if full {
		if !c.step(byte(lead)) || !c.step(b) || !c.decoder.neutral() {
			return 0, st, false
		}
	} else {
		if lead >= 0 {
			c.advance(byte(lead))
		}
		c.stepUnit(b, u)
	}
	st.prev, st.pendSet, st.pendVal, st.prevByte = int(c.prev), c.pending.set, c.pending.value, c.prevByte
	st.hw, st.hwSeen = c.halfWidthKatakanaState, c.halfWidthKatakanaSeen
	return c.score, st, true
}

func probeEUCJP(st probeState, lead int, b byte, u uint16, full bool) (int64, probeState, bool) {
	c := eucJPCandidate{
		prev: latinCJ(st.prev), prevByte: st.prevByte, prevPrevByte: st.prevPrevByte,
		halfWidthKatakanaState: st.hw, nonASCIISeen: st.nonASCIISeen,
	}
	if full {
		if !c.step(byte(lead)) || !c.step(b) || !c.decoder.neutral() {
			return 0, st, false
		}
	} else {
		if lead >= 0 {
			c.advance(byte(lead))
		}
		c.stepUnit(b, u)
	}
	st.prev, st.prevByte, st.prevPrevByte = int(c.prev), c.prevByte, c.prevPrevByte
	st.hw, st.nonASCIISeen = c.halfWidthKatakanaState, c.nonASCIISeen
	return c.score, st, true
}

func probeEUCKR(st probeState, lead int, b byte, u uint16, full bool) (int64, probeState, bool) {
	c := eucKRCandidate{
		prev: latinKorean(st.prev), pending: pendingScore{st.pendVal, st.pendSet}, prevByte: st.prevByte,
		prevWasEUCRange: st.prevByte >= 0xA1 && st.prevByte <= 0xFE, currentWordLen: st.wordLen,
	}
	if full {
		if !c.step(byte(lead)) || !c.step(b) || !c.decoder.neutral() {
			return 0, st, false
		}
	} else {
		if lead >= 0 {
			c.advance(byte(lead))
		}
		c.stepUnit(b, u)
	}
	st.prev, st.pendSet, st.pendVal, st.prevByte = int(c.prev), c.pending.set, c.pending.value, c.prevByte
	st.wordLen = c.currentWordLen
	return c.score, st, true
}

var (
	kindGBK      = &cjkKind{numPrev: 3, block: int(latinCJCJ), probe: probeGBK, newDecoder: func() byteDecoder { return &gb18030Decoder{} }}
	kindBig5     = &cjkKind{numPrev: 3, block: int(latinCJCJ), probe: probeBig5, newDecoder: func() byteDecoder { return &big5Decoder{} }}
	kindShiftJIS = &cjkKind{numPrev: 3, block: int(latinCJCJ), probe: probeShiftJIS, newDecoder: func() byteDecoder { return &shiftJISDecoder{} }}
	kindEUCJP    = &cjkKind{numPrev: 3, block: int(latinCJCJ), japanese: true, probe: probeEUCJP, newDecoder: func() byteDecoder { return &eucJPDecoder{} }}
	kindEUCKR    = &cjkKind{numPrev: 4, block: int(latinKoreanHangul), korean: true, probe: probeEUCKR, newDecoder: func() byteDecoder { return &eucKRDecoder{} }}
)

// cjkTables holds the entries of one candidate for characters that start in
// the decoder's initial state: singles[b] for a byte that decodes on its own
// (including ASCII) and pairs[lead<<8|trail] for two bytes.
//
// For Shift_JIS, halfWidth[b-0xA1][hw][hwSeen] is the entry for the
// half-width katakana byte b in the half-width katakana state hw (and
// halfWidthKatakanaSeen), and halfWidthNext[b-0xA1] is the state after it.
type cjkTables struct {
	singles       [256]cjkEntry
	pairs         [65536]cjkEntry
	halfWidth     [63][3][2]cjkEntry
	halfWidthNext [63]halfWidthKatakana
}

// pendingProbe is a pending score that no character score can produce.
const pendingProbe = 1 << 40

// derive returns the entry for a character, or 0 if the character's effect
// does not fit an entry.
func (k *cjkKind) derive(lead int, b byte, u uint16, full bool) cjkEntry {
	e, _ := k.deriveFrom(lead, b, u, full, dakutenForbidden, false, false)
	return e
}

// deriveFrom returns the entry for a character when the half-width katakana
// state is hw and hwSeen. If halfWidth is false, the character must not be
// half-width katakana. If halfWidth is true, the character must be, and
// deriveFrom also returns the half-width katakana state that follows it.
func (k *cjkKind) deriveFrom(lead int, b byte, u uint16, full bool, hw halfWidthKatakana, hwSeen, halfWidth bool) (cjkEntry, halfWidthKatakana) {
	probe := func(st probeState) (int64, probeState, bool) { return k.probe(st, lead, b, u, full) }

	// Probe from every value of prev. Each score must be the character score
	// s (unless s became pending) plus penalties that depend on prev.
	var scores [4]int64
	var outs [4]probeState
	pendable := false
	for v := 0; v < k.numPrev; v++ {
		st := probeState{prev: v, nonASCIISeen: true, hw: hw, hwSeen: hwSeen}
		score, out, ok := probe(st)
		if !ok {
			return 0, 0
		}
		scores[v], outs[v] = score, out
		pendable = pendable || out.pendSet
	}
	out := outs[k.block]
	for v := 0; v < k.numPrev; v++ {
		o := outs[v]
		if o.pendSet != (pendable && v != k.block) || o.prev != out.prev || o.hw != out.hw ||
			o.hwSeen != out.hwSeen || o.wordLen != out.wordLen || !o.nonASCIISeen {
			return 0, 0
		}
	}
	if halfWidth != out.hwSeen || (!halfWidth && out.hw != dakutenForbidden) {
		return 0, 0
	}
	penalties := [4]int64{0, cjkLatinAdjacencyPenalty, eucKRHanjaAfterHangulPenalty, cjkLatinAdjacencyPenalty + eucKRHanjaAfterHangulPenalty}
	// Backed by an array so that building the tables does not produce
	// garbage for each of the hundreds of thousands of characters.
	var candidatesBuf [len(scores) * len(penalties)]int64
	candidates := candidatesBuf[:0]
	for v := 0; v < k.numPrev; v++ {
		if outs[v].pendSet {
			candidates = append(candidatesBuf[:0], outs[v].pendVal)
			break
		}
		for _, p := range penalties {
			candidates = append(candidates, scores[v]-p)
		}
	}
	var e cjkEntry
	for _, s := range candidates {
		if s != int64(int16(s)) {
			continue
		}
		e = entryValid | cjkEntry(uint16(int16(s))) | cjkEntry(out.prev)<<16
		for v := 0; v < k.numPrev && e != 0; v++ {
			rest := scores[v]
			if outs[v].pendSet {
				if outs[v].pendVal != s {
					e = 0
					break
				}
			} else {
				rest -= s
			}
			switch rest {
			case penalties[0]:
			case penalties[1]:
				e |= cjkEntry(1) << (20 + v)
			case penalties[2]:
				e |= cjkEntry(1) << (24 + v)
			case penalties[3]:
				e |= cjkEntry(1)<<(20+v) | cjkEntry(1)<<(24+v)
			default:
				e = 0
			}
		}
		if e != 0 {
			break
		}
	}
	if e == 0 {
		return 0, 0
	}
	if pendable {
		e |= entryPendable
	}
	if k.korean && out.wordLen == 1 {
		e |= entryWordInc
	}

	base := probeState{prev: k.block, nonASCIISeen: true, hw: hw, hwSeen: hwSeen}
	withPending := base
	withPending.pendSet, withPending.pendVal = true, pendingProbe
	ps, _, ok := probe(withPending)
	if !ok {
		return 0, 0
	}
	switch ps - scores[k.block] {
	case 0:
		e |= entryDiscard
	case pendingProbe:
	default:
		return 0, 0
	}

	if k.japanese {
		unseen := base
		unseen.nonASCIISeen = false
		us, uo, ok := probe(unseen)
		if !ok {
			return 0, 0
		}
		diff := us - scores[k.block]
		if uo.nonASCIISeen {
			e |= entryNonASCII
			if diff == eucJPInitialKanaPenalty {
				e |= entryKana
			} else if diff != 0 {
				return 0, 0
			}
		} else if diff != 0 {
			return 0, 0
		}
	}
	return e, out.hw
}

func (k *cjkKind) buildTables() *cjkTables {
	t := new(cjkTables)
	for b := 0; b < 0x100; b++ {
		d := k.newDecoder()
		first := d.decodeByte(byte(b))
		if first.malformed {
			continue
		}
		if first.written == 1 {
			u := first.dst[0]
			t.singles[b] = k.derive(-1, byte(b), u, false)
			if k == kindShiftJIS && b >= 0xA1 && b <= 0xDF {
				for hw := range 3 {
					for seen := range 2 {
						e, next := k.deriveFrom(-1, byte(b), u, false, halfWidthKatakana(hw), seen == 1, true)
						t.halfWidth[b-0xA1][hw][seen] = e
						t.halfWidthNext[b-0xA1] = next
					}
				}
			}
			continue
		}
		if first.written != 0 {
			continue
		}
		for trail := 0; trail < 0x100; trail++ {
			d2 := k.newDecoder()
			d2.decodeByte(byte(b))
			step := d2.decodeByte(byte(trail))
			if !step.malformed && step.written == 1 {
				t.pairs[b<<8|trail] = k.derive(b, byte(trail), step.dst[0], false)
			} else {
				// An error that the candidate absorbs, or two code units.
				t.pairs[b<<8|trail] = k.derive(b, byte(trail), 0, true)
			}
		}
	}
	return t
}

var (
	cjkTablesOnce                                                   sync.Once
	gbkTables, big5Tables, shiftJISTables, eucJPTables, eucKRTables *cjkTables
)

func buildCJKTables() {
	gbkTables = kindGBK.buildTables()
	big5Tables = kindBig5.buildTables()
	shiftJISTables = kindShiftJIS.buildTables()
	eucJPTables = kindEUCJP.buildTables()
	eucKRTables = kindEUCKR.buildTables()
}
