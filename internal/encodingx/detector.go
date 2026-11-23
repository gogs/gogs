// Package encodingx detects the character encoding of text.
//
// It implements the detection algorithm of chardetng
// (https://github.com/hsivonen/chardetng by Henri Sivonen, Copyright Mozilla
// Foundation, licensed under the Apache License, Version 2.0 or the MIT
// license, at your option), Firefox's encoding detector, and reuses its data
// tables and scoring rules with an engine designed for speed. The single-byte
// candidates are scored from byte pair counts collected in one pass, ASCII
// runs are skipped wherever the scoring state allows, and disqualified
// candidates stop doing work.
//
// It implements the detection path without a top-level domain hint and with
// ISO-2022-JP detection allowed.
package encodingx

import (
	"runtime"
	"sync"
	"unicode/utf8"

	"unknwon.dev/x/logx"
)

const (
	firstNormal  = 3
	visualIndex  = 2
	gbkIndex     = 3
	eucJPIndex   = 4
	eucKRIndex   = 5
	shiftJISIdx  = 6
	big5Index    = 7
	numCJK       = 5
	numCandidate = 27
)

// Detector guesses the character encoding of a stream of bytes.
type Detector struct {
	utf8      utf8Validator
	utf8Alive bool
	iso       iso2022JPValidator
	isoAlive  bool

	sb singleByteEngine

	gbk      gbkCandidate
	eucJP    eucJPCandidate
	eucKR    eucKRCandidate
	shiftJIS shiftJISCandidate
	big5     big5Candidate
	cjkAlive [numCJK]bool
	cjkScore [numCJK]int64

	nonASCIISeen bool
	// We need to feed up to two bytes of context before non-ASCII
	// thanks to Spanish n.º.
	lastBeforeNonASCII [2]byte
	lastBeforeLen      int
	escSeen            bool
	closed             bool
}

// New returns a new detector.
func New() *Detector {
	cjkTablesOnce.Do(buildCJKTables)
	d := &Detector{utf8Alive: true, isoAlive: true}
	d.sb.init()
	d.gbk.prev = latinCJOther
	d.eucJP.prev = latinCJOther
	d.eucKR.prev = latinKoreanOther
	d.shiftJIS.prev = latinCJOther
	d.big5.prev = latinCJOther
	for i := range d.cjkAlive {
		d.cjkAlive[i] = true
	}
	return d
}

func (d *Detector) pushBeforeNonASCII(buffer []byte) {
	n := len(buffer)
	if n >= 2 {
		d.lastBeforeNonASCII = [2]byte{buffer[n-2], buffer[n-1]}
		d.lastBeforeLen = 2
	} else if n == 1 {
		switch d.lastBeforeLen {
		case 0:
			d.lastBeforeNonASCII[0] = buffer[0]
			d.lastBeforeLen = 1
		case 1:
			d.lastBeforeNonASCII[1] = buffer[0]
			d.lastBeforeLen = 2
		default:
			d.lastBeforeNonASCII = [2]byte{d.lastBeforeNonASCII[1], buffer[0]}
		}
	}
}

// Inputs of at least parallelMinLen bytes are processed by several
// goroutines when more than one CPU is available, like chardetng with its
// multithreading feature. The guess is the same either way.
const (
	parallelMinLen = 32 << 10
	parallelHead   = 8 << 10
)

func (d *Detector) feedImpl(buffer []byte, last bool) {
	if !d.nonASCIISeen && asciiRunEnd(buffer, 0) < len(buffer) {
		d.nonASCIISeen = true
	}
	if d.isoAlive {
		d.isoAlive = d.iso.feed(buffer, last)
	}
	if d.utf8Alive {
		d.utf8Alive = d.utf8.feed(buffer, last)
		if d.utf8Alive && last {
			// Guess returns without scoring once the whole stream is valid
			// UTF-8.
			return
		}
	}
	if len(buffer) >= parallelMinLen && runtime.GOMAXPROCS(0) > 1 {
		// Most inputs disqualify most CJK candidates early. Find out
		// before paying for goroutines.
		head, rest := buffer[:parallelHead], buffer[parallelHead:]
		d.feedCJK(head, false)
		if d.sb.alive != 0 {
			d.sb.feed(head, true)
		}
		if d.cjkAliveCount() > 0 {
			d.feedParallel(rest, last)
			return
		}
		buffer = rest
	}
	cjkMax := d.feedCJK(buffer, last)
	if d.sb.alive == 0 {
		return
	}
	if last && cjkMax > 0 {
		// A CJK candidate leads. If no single-byte candidate can beat it
		// even with the most favorable stateful terms, the stateful part of
		// the single-byte engine is not needed for this last chunk. The
		// guess is the same either way.
		alive, p1, pos := d.sb.alive, d.sb.p1, d.sb.pos
		d.sb.feedCounts(buffer)
		d.sb.feedCounts(space)
		d.finishSingleByte(buffer, cjkMax, alive, p1, pos)
		return
	}
	d.sb.feed(buffer, true)
	if last {
		// Treat EOF as space-like
		d.sb.feed(space, true)
	}
}

// finishSingleByte completes the last chunk for the single-byte engine
// after feedCounts has processed buffer and the EOF space. alive, p1 and pos
// are the engine's values before feedCounts.
func (d *Detector) finishSingleByte(buffer []byte, cjkMax int64, alive uint32, p1 byte, pos int64) {
	if d.sb.alive == 0 || d.sb.maxUpperBound() <= cjkMax {
		d.sb.outscored = true
		return
	}
	d.sb.alive, d.sb.p1, d.sb.pos = alive, p1, pos
	d.sb.feed(buffer, false)
	d.sb.feed(space, false)
}

func (d *Detector) cjkFeeds() [numCJK]func([]byte, bool) (int64, bool) {
	return [numCJK]func([]byte, bool) (int64, bool){
		d.gbk.feed, d.eucJP.feed, d.eucKR.feed, d.shiftJIS.feed, d.big5.feed,
	}
}

// feedCJK feeds the alive CJK candidates and returns the highest score of
// an alive CJK candidate, or 0 if no score is positive.
func (d *Detector) feedCJK(buffer []byte, last bool) int64 {
	for i, feed := range d.cjkFeeds() {
		if d.cjkAlive[i] {
			score, ok := feed(buffer, last)
			d.cjkAlive[i] = ok
			d.cjkScore[i] += score
		}
	}
	return d.cjkMax()
}

func (d *Detector) cjkMax() int64 {
	var max int64
	for i, alive := range d.cjkAlive {
		if alive && d.cjkScore[i] > max {
			max = d.cjkScore[i]
		}
	}
	return max
}

func (d *Detector) cjkAliveCount() int {
	n := 0
	for _, alive := range d.cjkAlive {
		if alive {
			n++
		}
	}
	return n
}

// feedParallel is the tail of feedImpl with each alive CJK candidate and the
// single-byte engine on its own goroutine.
func (d *Detector) feedParallel(buffer []byte, last bool) {
	var wg sync.WaitGroup
	var scores [numCJK]int64
	var oks [numCJK]bool
	for i, feed := range d.cjkFeeds() {
		if d.cjkAlive[i] {
			wg.Go(func() { scores[i], oks[i] = feed(buffer, last) })
		}
	}
	sbAlive := d.sb.alive != 0
	alive, p1, pos := d.sb.alive, d.sb.p1, d.sb.pos
	if sbAlive {
		if last {
			wg.Go(func() {
				d.sb.feedCounts(buffer)
				d.sb.feedCounts(space)
			})
		} else {
			wg.Go(func() { d.sb.feed(buffer, true) })
		}
	}
	wg.Wait()
	for i := range d.cjkAlive {
		if d.cjkAlive[i] {
			d.cjkAlive[i] = oks[i]
			d.cjkScore[i] += scores[i]
		}
	}
	if sbAlive && last {
		if cjkMax := d.cjkMax(); cjkMax > 0 {
			d.finishSingleByte(buffer, cjkMax, alive, p1, pos)
		} else {
			d.sb.alive, d.sb.p1, d.sb.pos = alive, p1, pos
			d.sb.feed(buffer, false)
			d.sb.feed(space, false)
		}
	}
}

var space = []byte{' '}

// Feed informs the detector of a chunk of input. The concatenation of the
// buffers passed to Feed forms the byte stream. The end of the stream is
// indicated by passing last=true, after which Feed must not be called again.
//
// Feed reports whether the stream has contained at least one non-ASCII byte
// so far.
//
// Feed panics if it has previously been called with last set to true.
func (d *Detector) Feed(buffer []byte, last bool) bool {
	if d.closed {
		panic("Must not feed again after feeding with last equaling true.")
	}
	if last {
		d.closed = true
		// Guess does not score anything for valid UTF-8.
		defer func() { d.sb.release(!d.utf8Alive) }()
	}
	start := 0
	if !d.nonASCIISeen && !d.escSeen {
		upTo := asciiRunEnd(buffer, 0)
		start = upTo
		for i := 0; i < upTo; i++ {
			if buffer[i] == 0x1B {
				d.escSeen = true
				start = i
				break
			}
		}
		if start == len(buffer) {
			d.pushBeforeNonASCII(buffer)
			return d.nonASCIISeen
		}
		if start == 0 || start == 1 {
			lastBefore := d.lastBeforeNonASCII
			n := d.lastBeforeLen
			d.lastBeforeLen = 0
			d.feedImpl(lastBefore[:n], false)
			start = 0
		} else {
			start -= 2
		}
	}
	d.feedImpl(buffer[start:], last)
	return d.nonASCIISeen
}

// Guess returns the WHATWG name of the guessed encoding for the bytes fed so
// far. If allowUTF8 is false, the result is never "UTF-8".
func (d *Detector) Guess(allowUTF8 bool) string {
	if !d.nonASCIISeen && d.escSeen && d.isoAlive {
		return "ISO-2022-JP"
	}
	if d.utf8Alive {
		if allowUTF8 {
			return "UTF-8"
		}
		return "windows-1252"
	}
	sbScores, sbOK, punct := d.sb.scores()
	cjkNames := [numCJK]string{"GBK", "EUC-JP", "EUC-KR", "Shift_JIS", "Big5"}

	encoding := "windows-1252"
	var max int64
	for i := firstNormal; i < numCandidate; i++ {
		var score int64
		var ok bool
		var name string
		if i < firstNormal+numCJK {
			score, ok, name = d.cjkScore[i-firstNormal], d.cjkAlive[i-firstNormal], cjkNames[i-firstNormal]
		} else {
			k := i - 7
			score, ok, name = sbScores[k], sbOK[k], sbData[k].encoding
		}
		if ok && score > max {
			max = score
			encoding = name
		}
	}
	if sbOK[bitVisual] {
		visualScore := sbScores[bitVisual]
		if (visualScore > max || encoding == "windows-1255") && punct[1] > punct[0] {
			encoding = "ISO-8859-8"
		}
	}
	return encoding
}

// Detect returns the best guess of the encoding of the content, as a WHATWG
// encoding name. Content that is not UTF-8 is taken as ansiCharset when it is
// not empty.
func Detect(logger *logx.Logger, content []byte, ansiCharset string) string {
	if utf8.Valid(content) {
		logger.Debug("Detected encoding via fast path", "charset", "UTF-8")
		return "UTF-8"
	} else if isMostlyUTF8(content) {
		// A few invalid bytes in otherwise UTF-8 content, e.g., a truncated
		// character, should not turn the whole file into mojibake.
		logger.Debug("Detected encoding as mostly UTF-8", "charset", "UTF-8")
		return "UTF-8"
	}

	if ansiCharset != "" {
		logger.Debug("Using default ANSI charset", "charset", ansiCharset)
		return ansiCharset
	}

	d := New()
	d.Feed(content, true)
	guess := d.Guess(false)
	logger.Debug("Detected encoding", "charset", guess)
	return guess
}

// isMostlyUTF8 reports whether the content has more than ten times as many
// valid multibyte UTF-8 characters as invalid bytes. Legacy encodings rarely
// form valid multibyte UTF-8 sequences by chance.
func isMostlyUTF8(content []byte) bool {
	var multibyte, invalid int
	for len(content) > 0 {
		r, size := utf8.DecodeRune(content)
		if r == utf8.RuneError && size == 1 {
			invalid++
		} else if size > 1 {
			multibyte++
		}
		content = content[size:]
	}
	return multibyte > 10*invalid
}
