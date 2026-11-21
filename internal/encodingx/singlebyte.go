package encodingx

import (
	"encoding/binary"
	"math"
	"math/bits"
	"sync"
)

// The single-byte engine evaluates all 20 single-byte candidates in one pass.
//
// chardetng's per-candidate scores are sums of terms that depend only on a
// pair of adjacent bytes (the probability table lookup, the Latin adjacency
// penalty, the lower-to-upper case transition penalty, the IBM866 0xA0 rule
// and the Hebrew punctuation counts). The engine counts every adjacent byte
// pair that involves a non-ASCII byte and turns the counts into scores in
// Guess through a precomputed per-candidate pair score table.
//
// The remaining stateful rules are tracked exactly, for all candidates at
// once, with one bit per candidate:
//   - disqualification by a byte of class 255,
//   - the Latin "lower case after two upper case" penalty (a byte triple),
//   - the non-Latin word case state machine (capitalization bonus, KOI8-U all
//     caps penalty, mixed case penalty per letter),
//   - whether a non-Latin word of at least two letters has ended,
//   - the Latin penalty for long runs of non-ASCII bytes (the same for all
//     Latin candidates),
//   - the windows-1252 ordinal and copyright state machine (scalar).

const numSB = 20

type sbKind uint8

const (
	kindLatin sbKind = iota
	kindNonLatinCased
	kindCaseless
	kindArabicFrench
	kindLogical
	kindVisual
)

var sbKinds = [numSB]sbKind{
	kindVisual, kindLatin, kindNonLatinCased, kindLatin, kindLatin,
	kindArabicFrench, kindLatin, kindLatin, kindCaseless, kindLogical,
	kindNonLatinCased, kindNonLatinCased, kindLatin, kindLatin, kindNonLatinCased,
	kindNonLatinCased, kindCaseless, kindLatin, kindLatin, kindNonLatinCased,
}

var sbData = [numSB]*singleByteData{
	&singleByteDataTable[iso88598Index],
	&singleByteDataTable[windows1252Index],
	&singleByteDataTable[windows1251Index],
	&singleByteDataTable[windows1250Index],
	&singleByteDataTable[iso88592Index],
	&singleByteDataTable[windows1256Index],
	&singleByteDataTable[windows1252IcelandicIndex],
	&singleByteDataTable[windows1254Index],
	&singleByteDataTable[windows874Index],
	&singleByteDataTable[windows1255Index],
	&singleByteDataTable[windows1253Index],
	&singleByteDataTable[iso88597Index],
	&singleByteDataTable[windows1257Index],
	&singleByteDataTable[iso885913Index],
	&singleByteDataTable[koi8UIndex],
	&singleByteDataTable[ibm866Index],
	&singleByteDataTable[iso88596Index],
	&singleByteDataTable[windows1258Index],
	&singleByteDataTable[iso88594Index],
	&singleByteDataTable[iso88595Index],
}

const (
	bitVisual  = 0
	bit1252    = 1
	bitLogical = 9
	bitKOI8U   = 14
	bitIBM866  = 15
	allSB      = 1<<numSB - 1
)

var (
	latinOnlyMask   uint32 // Latin candidates (non-ASCII run penalty)
	nonLatinMask    uint32 // NonLatinCased candidates
	longestWordMask uint32 // candidates that need a word of at least two letters
)

// byteInfo holds, for one byte value, one bit per single-byte candidate.
type byteInfo struct {
	dead   uint32 // class 255
	latUp  uint32 // upper case letter for the Latin case rules
	latLo  uint32 // lower case letter for the Latin case rules
	word   uint32 // non-Latin alphabetic (word letter)
	nlLat  uint32 // Latin letter class (NonLatinCased candidates)
	nlUp   uint32 // upper case non-Latin letter (NonLatinCased candidates)
	nlLo   uint32 // lower case non-Latin letter (NonLatinCased candidates)
	nlWord uint32 // non-Latin letter (NonLatinCased candidates)
	// nlSpace is set for the NonLatinCased candidates that treat the byte as
	// space, i.e., neither a non-Latin nor a Latin letter.
	nlSpace uint32
	// ord[state] is the next windows-1252 ordinal state in the low 4 bits
	// and the index of the bonus in ordBonusValues in the high 4 bits.
	ord [13]uint8
}

var byteInfos [256]byteInfo

// ordBonusValues are the bonuses of the windows-1252 ordinal state machine.
var ordBonusValues = [4]int64{0, ordinalBonus, ordinalBonus - implausibilityPenalty, copyrightBonus}

func init() {
	for k := 0; k < numSB; k++ {
		bit := uint32(1) << k
		switch sbKinds[k] {
		case kindLatin:
			latinOnlyMask |= bit
		case kindArabicFrench:
			longestWordMask |= bit
		case kindNonLatinCased:
			nonLatinMask |= bit
			longestWordMask |= bit
		default:
			longestWordMask |= bit
		}
	}
	for b := 0; b < 256; b++ {
		bi := &byteInfos[b]
		for k := 0; k < numSB; k++ {
			bit := uint32(1) << k
			d := sbData[k]
			class := d.classify(byte(b))
			if class == 255 {
				bi.dead |= bit
				continue
			}
			cc := class & 0x7F
			upper := class>>7 != 0
			switch sbKinds[k] {
			case kindLatin:
				if d.isLatinAlphabetic(cc) {
					if upper {
						bi.latUp |= bit
					} else {
						bi.latLo |= bit
					}
				}
			case kindArabicFrench:
				if cc == latinLetter {
					if upper {
						bi.latUp |= bit
					} else {
						bi.latLo |= bit
					}
				}
				if d.isNonLatinAlphabetic(cc, true) {
					bi.word |= bit
				}
			case kindNonLatinCased:
				if d.isNonLatinAlphabetic(cc, false) {
					bi.word |= bit
					if upper {
						bi.nlUp |= bit
					}
				} else if cc == latinLetter {
					bi.nlLat |= bit
				}
			default:
				if d.isNonLatinAlphabetic(cc, false) {
					bi.word |= bit
				}
			}
		}
		bi.nlWord = bi.word & nonLatinMask
		bi.nlLo = bi.nlWord &^ bi.nlUp
		bi.nlSpace = nonLatinMask &^ (bi.nlWord | bi.nlLat)
		cc := sbData[bit1252].classify(byte(b)) & 0x7F
		for s := 0; s < 13; s++ {
			next, bonus := ordinalStep(ordinalState(s), byte(b), cc)
			i := 0
			for ordBonusValues[i] != bonus {
				i++
			}
			bi.ord[s] = uint8(next) | uint8(i)<<4
		}
	}
}

// ordinalStep is the windows-1252 ordinal state machine of chardetng's
// LatinCandidate.
func ordinalStep(state ordinalState, b byte, cc uint8) (ordinalState, int64) {
	isRoman := cc == 9 || cc == 22 || cc == 24
	switch state {
	case ordinalOther:
		if cc == 0 {
			return ordinalSpace, 0
		}
		return ordinalOther, 0
	case ordinalSpace:
		switch {
		case cc == 0:
			return ordinalSpace, 0
		case b == 0xAA || b == 0xBA:
			return ordinalOrdinalExpectingSpace, 0
		case b == 'M' || b == 'D' || b == 'S':
			return ordinalFeminineAbbreviationStartLetter, 0
		case b == 'N':
			return ordinalUpperN, 0
		case b == 'n':
			return ordinalLowerN, 0
		case cc == asciiDigit:
			return ordinalDigit, 0
		case isRoman:
			return ordinalRoman, 0
		case b == 0xA9:
			return ordinalCopyright, 0
		}
		return ordinalOther, 0
	case ordinalOrdinalExpectingSpace:
		if cc == 0 {
			return ordinalSpace, ordinalBonus
		}
		return ordinalOther, 0
	case ordinalOrdinalExpectingSpaceUndoImplausibility:
		if cc == 0 {
			return ordinalSpace, ordinalBonus - implausibilityPenalty
		}
		return ordinalOther, 0
	case ordinalOrdinalExpectingSpaceOrDigit:
		switch cc {
		case 0:
			return ordinalSpace, ordinalBonus
		case asciiDigit:
			return ordinalOther, ordinalBonus
		}
		return ordinalOther, 0
	case ordinalOrdinalExpectingSpaceOrDigitUndoImplausibily:
		switch cc {
		case 0:
			return ordinalSpace, ordinalBonus - implausibilityPenalty
		case asciiDigit:
			return ordinalOther, ordinalBonus - implausibilityPenalty
		}
		return ordinalOther, 0
	case ordinalUpperN:
		switch {
		case b == 0xAA:
			return ordinalOrdinalExpectingSpaceUndoImplausibility, 0
		case b == 0xBA:
			return ordinalOrdinalExpectingSpaceOrDigitUndoImplausibily, 0
		case b == '.':
			return ordinalPeriodAfterN, 0
		case cc == 0:
			return ordinalSpace, 0
		}
		return ordinalOther, 0
	case ordinalLowerN:
		switch {
		case b == 0xBA:
			return ordinalOrdinalExpectingSpaceOrDigitUndoImplausibily, 0
		case b == '.':
			return ordinalPeriodAfterN, 0
		case cc == 0:
			return ordinalSpace, 0
		}
		return ordinalOther, 0
	case ordinalFeminineAbbreviationStartLetter:
		if b == 0xAA {
			return ordinalOrdinalExpectingSpaceUndoImplausibility, 0
		} else if cc == 0 {
			return ordinalSpace, 0
		}
		return ordinalOther, 0
	case ordinalDigit:
		switch {
		case b == 0xAA || b == 0xBA:
			return ordinalOrdinalExpectingSpace, 0
		case cc == 0:
			return ordinalSpace, 0
		case cc == asciiDigit:
			return ordinalDigit, 0
		}
		return ordinalOther, 0
	case ordinalRoman:
		switch {
		case b == 0xAA || b == 0xBA:
			return ordinalOrdinalExpectingSpaceUndoImplausibility, 0
		case cc == 0:
			return ordinalSpace, 0
		case isRoman:
			return ordinalRoman, 0
		}
		return ordinalOther, 0
	case ordinalPeriodAfterN:
		if b == 0xBA {
			return ordinalOrdinalExpectingSpaceOrDigit, 0
		} else if cc == 0 {
			return ordinalSpace, 0
		}
		return ordinalOther, 0
	case ordinalCopyright:
		if cc == 0 {
			return ordinalSpace, copyrightBonus
		}
		return ordinalOther, 0
	}
	panic("unreachable")
}

// ordinalReset reports whether b moves the ordinal state machine to Space
// from every state that an ASCII byte can lead to.
func ordinalReset(b byte) bool {
	return b < 0x80 && b != '.' && latinASCII[b] == 0
}

func isASCIIPunctuation(b byte) bool {
	switch b {
	case '.', ',', ':', ';', '?', '!':
		return true
	}
	return false
}

// pairScore is the part of a candidate's per-byte score that depends only on
// the previous byte p and the current byte b. It also reports whether the
// pair counts as plausible punctuation (Logical and Visual candidates).
func pairScore(k int, p, b byte) (int64, bool) {
	d := sbData[k]
	pc, class := d.classify(p), d.classify(b)
	if pc == 255 || class == 255 {
		return 0, false
	}
	pcc, cc := pc&0x7F, class&0x7F
	ascii, prevASCII := b < 0x80, p < 0x80
	asciiPair := ascii && prevASCII
	var score int64
	punct := false
	adjacency := func(w1256 bool) {
		if pcc == latinLetter && d.isNonLatinAlphabetic(cc, w1256) {
			score += latinAdjacencyPenalty
		} else if cc == latinLetter && d.isNonLatinAlphabetic(pcc, w1256) {
			score += latinAdjacencyPenalty
		}
	}
	switch sbKinds[k] {
	case kindLatin:
		if d.isLatinAlphabetic(cc) && class>>7 != 0 && d.isLatinAlphabetic(pcc) && pc>>7 == 0 && !asciiPair {
			score += implausibleLatinCaseTransitionPenalty
		}
		asciiIshPair := asciiPair || (ascii && pcc == 0) || (cc == 0 && prevASCII)
		if !asciiIshPair {
			score += d.score(cc, pcc, false)
		}
	case kindArabicFrench:
		if cc == latinLetter && class>>7 != 0 && pcc == latinLetter && pc>>7 == 0 && !asciiPair {
			score += implausibleLatinCaseTransitionPenalty
		}
		if !asciiPair {
			score += d.score(cc, pcc, true)
			adjacency(true)
		}
	case kindNonLatinCased:
		if !asciiPair {
			isA0, prevWasA0 := b == 0xA0, p == 0xA0
			if !(k == bitIBM866 && ((isA0 && (prevWasA0 || pcc == 0)) || cc == 0 && prevWasA0)) {
				score += d.score(cc, pcc, false)
			}
			adjacency(false)
		}
	case kindCaseless:
		if !asciiPair {
			score += d.score(cc, pcc, false)
			adjacency(false)
		}
	case kindLogical:
		if !asciiPair {
			score += d.score(cc, pcc, false)
			punct = cc == 0 && d.isNonLatinAlphabetic(pcc, false) && isASCIIPunctuation(b)
			adjacency(false)
		}
	case kindVisual:
		if !asciiPair {
			score += d.score(cc, pcc, false)
			punct = d.isNonLatinAlphabetic(cc, false) && pcc == 0 && isASCIIPunctuation(p)
			adjacency(false)
		}
	}
	return score, punct
}

// pairTables are built on first use: pairScores[key*numSB+k] is pairScore
// for candidate k and key = prev<<8 | cur.
var (
	pairTablesOnce sync.Once
	pairScores     []int16
)

func buildPairTables() {
	pairScores = make([]int16, 65536*numSB)
	for key := 0; key < 65536; key++ {
		p, b := byte(key>>8), byte(key)
		if p < 0x80 && b < 0x80 {
			continue
		}
		for k := 0; k < numSB; k++ {
			s, _ := pairScore(k, p, b)
			pairScores[key*numSB+k] = int16(s)
		}
	}
}

// denseThreshold is the number of recorded pairs after which pair counts
// move from a list to a dense table.
const denseThreshold = 2048

type singleByteEngine struct {
	alive uint32
	p1    byte
	pos   int64

	prevWord, prevPrevWord uint32
	committed              uint32 // a word of at least two letters has ended
	prevUp, prevPrevUp     uint32 // latUp of the previous two bytes

	// NonLatinCased case states, one-hot per candidate.
	sSpace, sUpper, sLower, sUL, sAC, sMix uint32
	wordStart                              [numSB]int64
	caseScore                              [numSB]int64 // NonLatinCased word case bonuses and penalties
	uuHits                                 [numSB]int64 // Latin "lower after two upper" penalties

	runStart    int64 // position of the first byte of the current non-ASCII run
	latinRunPen int64 // Latin non-ASCII run penalty of the completed runs

	ord      uint8
	ordScore int64

	punct [2]uint64 // plausible punctuation of the Logical and Visual candidates

	// outscored is set when no single-byte candidate can win, in which case
	// the stateful part of the last chunk has been skipped.
	outscored bool

	pairs  []uint16       // pair keys, possibly repeated, before going dense
	counts *[65536]uint32 // dense pair counts, from countsPool
	keys   []uint16       // distinct keys of counts

	// released is set once the input has ended and the pair counts have been
	// folded into pairSums, the pair terms of the scores.
	released bool
	pairSums [numSB]int64
}

// countsPool holds zeroed dense pair count tables, so that each detection
// does not allocate its own 256 KB table.
var countsPool = sync.Pool{New: func() any { return new([65536]uint32) }}

func (e *singleByteEngine) init() {
	e.alive = allSB
	e.p1 = ' '
	e.sSpace = nonLatinMask
	e.ord = uint8(ordinalSpace)
}

// runPenaltySum is the sum of LatinCandidate's non-ASCII penalties of the
// first n+1 bytes after an ASCII byte, when the first n of them are
// non-ASCII.
func runPenaltySum(n int64) int64 {
	switch {
	case n <= 2:
		return 0
	case n == 3:
		return -5
	}
	return -25 - 200*(n-4)
}

func (e *singleByteEngine) goDense() {
	e.counts = countsPool.Get().(*[65536]uint32)
	for _, k := range e.pairs {
		c := &e.counts[k]
		if *c == 0 {
			e.keys = append(e.keys, k)
		}
		*c++
	}
	e.pairs = nil
}

// feed processes buf. It is equivalent to running each single-byte candidate
// of chardetng over buf, except that the pairwise score terms are only
// counted, to be scored in scores.
//
// If counts is false, the pair counts and the Latin run penalty are not
// updated because feedCounts has already processed buf.
func (e *singleByteEngine) feed(buf []byte, counts bool) {
	alive := e.alive
	p1 := e.p1
	pos := e.pos
	prevWord, prevPrevWord, committed := e.prevWord, e.prevPrevWord, e.committed
	prevUp, prevPrevUp := e.prevUp, e.prevPrevUp
	sSpace, sUpper, sLower, sUL, sAC, sMix := e.sSpace, e.sUpper, e.sLower, e.sUL, e.sAC, e.sMix
	ord, ordScore := e.ord, e.ordScore
	dense := e.counts
	for i := 0; i < len(buf); i++ {
		b := buf[i]
		if b < 0x80 && p1 < 0x80 {
			// An ASCII byte after an ASCII byte. Within such a run, only the
			// ordinal state machine and the context for the next byte
			// change. ASCII bytes are never non-Latin letters, so every
			// NonLatinCased candidate ends up in Mix after a Latin letter and
			// in Space otherwise, without any bonus or penalty because no word
			// letters precede.
			j := asciiRunEnd(buf, i)
			last := buf[j-1]
			prev := p1
			if j-i >= 2 {
				prev = buf[j-2]
			}
			if byteInfos[last].nlLat != 0 {
				sSpace, sMix = 0, nonLatinMask
			} else {
				sSpace, sMix = nonLatinMask, 0
			}
			sUpper, sLower, sUL, sAC = 0, 0, 0, 0
			prevWord, prevPrevWord = 0, 0
			prevUp, prevPrevUp = byteInfos[last].latUp, byteInfos[prev].latUp
			if alive&(1<<bit1252) != 0 {
				// No ordinal bonus can occur inside the run. A space-like
				// byte other than '.' resets the state to Space.
				start := i
				for k := j - 1; k >= i; k-- {
					if ordinalReset(buf[k]) {
						ord = uint8(ordinalSpace)
						start = k + 1
						break
					}
				}
				for _, c := range buf[start:j] {
					ord = byteInfos[c].ord[ord] & 0xF
				}
			}
			p1 = last
			pos += int64(j - i)
			i = j - 1
			continue
		}

		bi := &byteInfos[b]
		alive &^= bi.dead

		if counts {
			key := uint16(p1)<<8 | uint16(b)
			if dense == nil {
				e.pairs = append(e.pairs, key)
				if len(e.pairs) >= denseThreshold {
					e.goDense()
					dense = e.counts
				}
			} else {
				c := &dense[key]
				if *c == 0 {
					e.keys = append(e.keys, key)
				}
				*c++
			}
		}
		if b < 0x80 {
			// p1 is non-ASCII here.
			if counts {
				e.latinRunPen += runPenaltySum(pos - e.runStart)
			}
			// Logical: punctuation after a Hebrew letter.
			if prevWord&alive&(1<<bitLogical) != 0 && isASCIIPunctuation(b) {
				e.punct[0]++
			}
		} else if p1 < 0x80 {
			if counts {
				e.runStart = pos
			}
			// Visual: a Hebrew letter after punctuation.
			if bi.word&alive&(1<<bitVisual) != 0 && isASCIIPunctuation(p1) {
				e.punct[1]++
			}
		}
		if m := bi.latLo & prevUp & prevPrevUp & alive; m != 0 {
			for ; m != 0; m &= m - 1 {
				e.uuHits[bits.TrailingZeros32(m)]++
			}
		}

		word := bi.word
		if ends := bi.nlSpace & prevWord; ends&(sUL|sAC|sMix) != 0 {
			e.emitWordEnd(ends, sUL, sAC, sMix, pos)
		}
		if st := bi.nlWord &^ prevWord; st != 0 {
			for ; st != 0; st &= st - 1 {
				e.wordStart[bits.TrailingZeros32(st)] = pos
			}
		}
		up, lo := bi.nlUp, bi.nlLo
		sSpace, sUpper, sLower, sUL, sAC, sMix =
			bi.nlSpace,
			up&sSpace,
			lo&(sSpace|sLower),
			lo&(sUpper|sUL),
			up&(sUpper|sAC),
			bi.nlLat|lo&(sMix|sAC)|up&(sLower|sUL|sMix)

		committed |= prevWord & prevPrevWord &^ word
		prevPrevWord, prevWord = prevWord, word

		// The ordinal state machine only matters while windows-1252 is
		// alive, but running it regardless is cheaper than checking.
		t := bi.ord[ord]
		ord = t & 0xF
		ordScore += ordBonusValues[t>>4]

		prevPrevUp, prevUp = prevUp, bi.latUp
		p1 = b
		pos++
	}
	e.alive = alive
	e.p1 = p1
	e.pos = pos
	e.prevWord, e.prevPrevWord, e.committed = prevWord, prevPrevWord, committed
	e.prevUp, e.prevPrevUp = prevUp, prevPrevUp
	e.sSpace, e.sUpper, e.sLower, e.sUL, e.sAC, e.sMix = sSpace, sUpper, sLower, sUL, sAC, sMix
	e.ord, e.ordScore = ord, ordScore
}

// feedCounts does the part of feed that the upper bound of upperBound
// needs: disqualification, the pair counts and the Latin run penalty.
func (e *singleByteEngine) feedCounts(buf []byte) {
	alive, p1, pos := e.alive, e.p1, e.pos
	for i := 0; i < len(buf); i++ {
		b := buf[i]
		if b < 0x80 && p1 < 0x80 {
			j := asciiRunEnd(buf, i)
			p1 = buf[j-1]
			pos += int64(j - i)
			i = j - 1
			continue
		}
		alive &^= byteInfos[b].dead
		if b < 0x80 {
			e.latinRunPen += runPenaltySum(pos - e.runStart)
		} else if p1 < 0x80 {
			e.runStart = pos
		}
		key := uint16(p1)<<8 | uint16(b)
		if e.counts == nil {
			e.pairs = append(e.pairs, key)
			if len(e.pairs) >= denseThreshold {
				e.goDense()
			}
		} else {
			c := &e.counts[key]
			if *c == 0 {
				e.keys = append(e.keys, key)
			}
			*c++
		}
		p1 = b
		pos++
	}
	e.alive, e.p1, e.pos = alive, p1, pos
}

// maxUpperBound returns an upper bound of the final score of every alive
// single-byte candidate, assuming that the stateful part of feed has run up
// to the point where feedCounts took over. The pair terms and the Latin run
// penalty are exact. The stateful terms are bounded: the Latin case penalty
// only lowers scores, the capitalization bonus applies at most once per pair
// of a letter and a space-like byte, and an ordinal bonus needs one of the
// bytes 0xAA, 0xBA and 0xA9.
func (e *singleByteEngine) maxUpperBound() int64 {
	scores, ok, _ := e.scoresWith(true)
	var wordEnds [numSB]int64
	var ordinalBytes int64
	visit := func(key uint16, n uint32) {
		p, b := byte(key>>8), byte(key)
		if m := byteInfos[p].nlWord & byteInfos[b].nlSpace; m != 0 {
			for ; m != 0; m &= m - 1 {
				wordEnds[bits.TrailingZeros32(m)] += int64(n)
			}
		}
		if b == 0xAA || b == 0xBA || b == 0xA9 {
			ordinalBytes += int64(n)
		}
	}
	e.visitPairs(visit)
	max := int64(math.MinInt64)
	for k := range numSB {
		if !ok[k] {
			continue
		}
		ub := scores[k] - e.uuHits[k]*implausibleLatinCaseTransitionPenalty
		if sbKinds[k] == kindNonLatinCased {
			ub += nonLatinCapitalizationBonus * wordEnds[k]
		}
		if k == bit1252 {
			ub += (ordinalBonus - implausibilityPenalty) * ordinalBytes
		}
		if ub > max {
			max = ub
		}
	}
	return max
}

// release folds the pair counts into pairSums and returns the dense table to
// countsPool. It is called once no more input can arrive. If fold is false,
// the scores are never needed and the counts are dropped.
func (e *singleByteEngine) release(fold bool) {
	if fold && e.alive != 0 && !e.outscored {
		pairTablesOnce.Do(buildPairTables)
		e.visitPairs(func(key uint16, n uint32) {
			row := pairScores[int(key)*numSB : int(key)*numSB+numSB]
			for k := range numSB {
				e.pairSums[k] += int64(row[k]) * int64(n)
			}
		})
	}
	e.released = true
	if e.counts != nil {
		// Every non-zero entry is in keys, which is much cheaper than
		// clearing the whole table.
		for _, key := range e.keys {
			e.counts[key] = 0
		}
		countsPool.Put(e.counts)
	}
	e.pairs, e.counts, e.keys = nil, nil, nil
}

func (e *singleByteEngine) visitPairs(visit func(key uint16, n uint32)) {
	if e.counts == nil {
		for _, key := range e.pairs {
			visit(key, 1)
		}
	} else {
		for _, key := range e.keys {
			visit(key, e.counts[key])
		}
	}
}

// emitWordEnd applies the NonLatinCased bonuses and penalties for the words
// that end at position pos for the candidates in m.
func (e *singleByteEngine) emitWordEnd(m, sUL, sAC, sMix uint32, pos int64) {
	for ; m != 0; m &= m - 1 {
		k := bits.TrailingZeros32(m)
		bit := uint32(1) << k
		switch {
		case sUL&bit != 0:
			e.caseScore[k] += nonLatinCapitalizationBonus
		case sAC&bit != 0:
			if k == bitKOI8U {
				e.caseScore[k] += nonLatinAllCapsPenalty
			}
		case sMix&bit != 0:
			e.caseScore[k] += nonLatinMixedCasePenalty * (pos - e.wordStart[k])
		}
	}
}

// asciiRunEnd returns the index of the first non-ASCII byte in buf at or
// after i, or len(buf).
func asciiRunEnd(buf []byte, i int) int {
	for i+8 <= len(buf) {
		if v := binary.LittleEndian.Uint64(buf[i:]) & 0x8080808080808080; v != 0 {
			return i + bits.TrailingZeros64(v)/8
		}
		i += 8
	}
	for i < len(buf) && buf[i] < 0x80 {
		i++
	}
	return i
}

// scores returns the scores of the single-byte candidates and the plausible
// punctuation counts of the Logical and Visual candidates. ok[k] is false for
// disqualified candidates.
func (e *singleByteEngine) scores() (scores [numSB]int64, ok [numSB]bool, punct [2]uint64) {
	if e.outscored {
		return scores, ok, punct
	}
	return e.scoresWith(false)
}

// scoresWith is scores. If ignoreWords is true, the requirement of a word of
// at least two letters is ignored.
func (e *singleByteEngine) scoresWith(ignoreWords bool) (scores [numSB]int64, ok [numSB]bool, punct [2]uint64) {
	punct = e.punct
	if e.alive == 0 {
		return scores, ok, punct
	}
	if e.released {
		scores = e.pairSums
	} else {
		pairTablesOnce.Do(buildPairTables)
		e.visitPairs(func(key uint16, n uint32) {
			row := pairScores[int(key)*numSB : int(key)*numSB+numSB]
			for k := range numSB {
				scores[k] += int64(row[k]) * int64(n)
			}
		})
	}
	latinRunPen := e.latinRunPen
	if e.p1 >= 0x80 {
		// The current run has not ended.
		latinRunPen += runPenaltySum(e.pos - e.runStart - 1)
	}
	for k := range numSB {
		bit := uint32(1) << k
		if e.alive&bit == 0 {
			continue
		}
		if !ignoreWords && longestWordMask&bit != 0 && e.committed&bit == 0 {
			continue
		}
		ok[k] = true
		if latinOnlyMask&bit != 0 {
			scores[k] += latinRunPen
		}
		scores[k] += e.uuHits[k]*implausibleLatinCaseTransitionPenalty + e.caseScore[k]
		if k == bit1252 {
			scores[k] += e.ordScore
		}
	}
	return scores, ok, punct
}

type ordinalState int

const (
	ordinalOther ordinalState = iota
	ordinalSpace
	ordinalPeriodAfterN
	ordinalOrdinalExpectingSpace
	ordinalOrdinalExpectingSpaceUndoImplausibility
	ordinalOrdinalExpectingSpaceOrDigit
	ordinalOrdinalExpectingSpaceOrDigitUndoImplausibily
	ordinalUpperN
	ordinalLowerN
	ordinalFeminineAbbreviationStartLetter
	ordinalDigit
	ordinalRoman
	ordinalCopyright
)
