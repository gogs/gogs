package mahonia

// This file is based on bufio.Reader in the Go standard library,
// which has the following copyright notice:

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

import (
	"io"
	"unicode/utf8"
)

const (
	defaultBufSize = 4096
)

// Reader implements character-set decoding for an io.Reader object.
type Reader struct {
	buf    []byte
	rd     io.Reader
	decode Decoder
	r, w   int
	err    error
}

// NewReader creates a new Reader that uses the receiver to decode text.
func (d Decoder) NewReader(rd io.Reader) *Reader {
	b := new(Reader)
	b.buf = make([]byte, defaultBufSize)
	b.rd = rd
	b.decode = d
	return b
}

// fill reads a new chunk into the buffer.
func (b *Reader) fill() {
	// Slide existing data to beginning.
	if b.r > 0 {
		copy(b.buf, b.buf[b.r:b.w])
		b.w -= b.r
		b.r = 0
	}

	// Read new data.
	n, e := b.rd.Read(b.buf[b.w:])
	b.w += n
	if e != nil {
		b.err = e
	}
}

// Read reads data into p.
// It returns the number of bytes read into p.
// It calls Read at most once on the underlying Reader,
// hence n may be less than len(p).
// At EOF, the count will be zero and err will be os.EOF.
func (b *Reader) Read(p []byte) (n int, err error) {
	n = len(p)
	filled := false
	if n == 0 {
		return 0, b.err
	}
	if b.w == b.r {
		if b.err != nil {
			return 0, b.err
		}
		if n > len(b.buf) {
			// Large read, empty buffer.
			// Allocate a larger buffer for efficiency.
			b.buf = make([]byte, n)
		}
		b.fill()
		filled = true
		if b.w == b.r {
			return 0, b.err
		}
	}

	i := 0
	for i < n {
		rune, size, status := b.decode(b.buf[b.r:b.w])

		if status == STATE_ONLY {
			b.r += size
			continue
		}

		if status == NO_ROOM {
			if b.err != nil {
				rune = 0xfffd
				size = b.w - b.r
				if size == 0 {
					break
				}
				status = INVALID_CHAR
			} else if filled {
				break
			} else {
				b.fill()
				filled = true
				continue
			}
		}

		if i+utf8.RuneLen(rune) > n {
			break
		}

		b.r += size
		if rune < 128 {
			p[i] = byte(rune)
			i++
		} else {
			i += utf8.EncodeRune(p[i:], rune)
		}
	}

	return i, nil
}

// ReadRune reads a single Unicode character and returns the
// rune and its size in bytes.
func (b *Reader) ReadRune() (c rune, size int, err error) {
read:
	c, size, status := b.decode(b.buf[b.r:b.w])

	if status == NO_ROOM && b.err == nil {
		b.fill()
		goto read
	}

	if status == STATE_ONLY {
		b.r += size
		goto read
	}

	if b.r == b.w {
		return 0, 0, b.err
	}

	if status == NO_ROOM {
		c = 0xfffd
		size = b.w - b.r
		status = INVALID_CHAR
	}

	b.r += size
	return c, size, nil
}
