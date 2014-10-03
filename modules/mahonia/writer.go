package mahonia

import (
	"io"
	"unicode/utf8"
)

// Writer implements character-set encoding for an io.Writer object.
type Writer struct {
	wr     io.Writer
	encode Encoder
	inbuf  []byte
	outbuf []byte
}

// NewWriter creates a new Writer that uses the receiver to encode text.
func (e Encoder) NewWriter(wr io.Writer) *Writer {
	w := new(Writer)
	w.wr = wr
	w.encode = e
	return w
}

// Write encodes and writes the data from p.
func (w *Writer) Write(p []byte) (n int, err error) {
	n = len(p)

	if len(w.inbuf) > 0 {
		w.inbuf = append(w.inbuf, p...)
		p = w.inbuf
	}

	if len(w.outbuf) < len(p) {
		w.outbuf = make([]byte, len(p)+10)
	}

	outpos := 0

	for len(p) > 0 {
		rune, size := utf8.DecodeRune(p)
		if rune == 0xfffd && !utf8.FullRune(p) {
			break
		}

		p = p[size:]

	retry:
		size, status := w.encode(w.outbuf[outpos:], rune)

		if status == NO_ROOM {
			newDest := make([]byte, len(w.outbuf)*2)
			copy(newDest, w.outbuf)
			w.outbuf = newDest
			goto retry
		}

		if status == STATE_ONLY {
			outpos += size
			goto retry
		}

		outpos += size
	}

	w.inbuf = w.inbuf[:0]
	if len(p) > 0 {
		w.inbuf = append(w.inbuf, p...)
	}

	n1, err := w.wr.Write(w.outbuf[0:outpos])

	if err != nil && n1 < n {
		n = n1
	}

	return
}

func (w *Writer) WriteRune(c rune) (size int, err error) {
	if len(w.inbuf) > 0 {
		// There are leftover bytes, a partial UTF-8 sequence.
		w.inbuf = w.inbuf[:0]
		w.WriteRune(0xfffd)
	}

	if w.outbuf == nil {
		w.outbuf = make([]byte, 16)
	}

	outpos := 0

retry:
	size, status := w.encode(w.outbuf[outpos:], c)

	if status == NO_ROOM {
		w.outbuf = make([]byte, len(w.outbuf)*2)
		goto retry
	}

	if status == STATE_ONLY {
		outpos += size
		goto retry
	}

	outpos += size

	return w.wr.Write(w.outbuf[0:outpos])
}
