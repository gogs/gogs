// Copyright 2015 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
)

var (
	// ObjectReader implemented ReadCloser
	_ io.ReadCloser = new(readCloser)
	_ io.ReaderAt   = new(readAter)
)

type readCloser struct {
	r io.Reader
	c io.Closer
}

func (o *readCloser) Read(p []byte) (n int, err error) {
	return o.r.Read(p)
}

func (o *readCloser) Close() error {
	return o.c.Close()
}

func newReadCloser(r io.Reader, c io.Closer) io.ReadCloser {
	return &readCloser{r, c}
}

type bufReadCloser struct {
	r *bytes.Buffer
}

func (o *bufReadCloser) Read(p []byte) (n int, err error) {
	return o.r.Read(p)
}

func (o *bufReadCloser) Close() error {
	return nil
}

func newBufReadCloser(buf []byte) io.ReadCloser {
	o := new(bufReadCloser)
	o.r = bytes.NewBuffer(buf)
	return o
}

type readAter struct {
	buf []byte
}

func (o *readAter) ReadAt(p []byte, off int64) (n int, err error) {
	if int(off) >= len(o.buf) {
		err = io.EOF
		return
	}

	length := len(p)
	if length == 0 {
		return
	}

	if length > len(o.buf[off:]) {
		length = len(o.buf[off:])
	}

	copy(p, o.buf[off:])

	n = length
	return
}

// readerDecompressed reads deflated object from the file.
func readerDecompressed(r io.Reader, inflatedSize int64) (io.ReadCloser, error) {
	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("new zlib reader: %v", err)
	}

	return newReadCloser(io.LimitReader(zr, inflatedSize), zr), nil
}

// buf must be large enough to read the number.
func readerLittleEndianBase128Number(r io.Reader) (int64, int) {
	zpos := 0
	buf := []byte{0}
	n, err := r.Read(buf)
	if err != nil {
		return 0, n
	}

	length := int64(buf[0] & 0x7f)
	shift := uint64(0)
	for buf[0]&0x80 > 0 {
		shift += 7

		n, err := r.Read(buf)
		if err != nil {
			return 0, zpos + n
		}

		zpos += n
		length |= int64(buf[0]&0x7f) << shift
	}
	zpos += 1
	return length, zpos
}

func readerApplyDelta(br io.ReaderAt, dr io.Reader, resultLen int64) (res []byte, err error) {
	var (
		resultpos uint64
	)

	buf := []byte{0}
	res = make([]byte, resultLen)

	read := func(r io.Reader) (ret bool) {
		var n int
		n, err = r.Read(buf)
		if err == io.EOF {
			err = nil
			return
		}
		if n == 0 || err != nil {
			return
		}
		ret = true
		return
	}

	readAt := func(r io.ReaderAt, off int64) (ret bool) {
		var n int
		n, err = r.ReadAt(buf, off)
		if err == io.EOF {
			err = nil
			return
		}
		if n == 0 || err != nil {
			return
		}
		ret = true
		return
	}

	for {
		// two modes: copy and insert. copy reads offset and len from the delta
		// instructions and copy len bytes from offset into the resulting object
		// insert takes up to 127 bytes and insert them into the
		// resulting object

		if !read(dr) {
			return
		}
		opcode := buf[0]

		if opcode&0x80 > 0 {
			// Copy from base to dest
			copy_offset := uint64(0)
			copy_length := uint64(0)
			shift := uint(0)
			for i := 0; i < 4; i++ {
				if opcode&0x01 > 0 {
					if !read(dr) {
						return
					}
					copy_offset |= uint64(buf[0]) << shift
				}
				opcode >>= 1
				shift += 8
			}

			shift = 0
			for i := 0; i < 3; i++ {
				if opcode&0x01 > 0 {
					if !read(dr) {
						return
					}
					copy_length |= uint64(buf[0]) << shift
				}
				opcode >>= 1
				shift += 8
			}
			if copy_length == 0 {
				copy_length = 1 << 16
			}

			brOffset := int64(copy_offset)
			for i := uint64(0); i < copy_length; i++ {
				if !readAt(br, brOffset) {
					return
				}
				res[resultpos] = buf[0]
				resultpos++
				brOffset++
			}
		} else if opcode > 0 {
			// insert n bytes at the end of the resulting object. n==opcode
			for i := 0; i < int(opcode); i++ {
				if !read(dr) {
					return
				}
				res[resultpos] = buf[0]
				resultpos++
			}
		} else {
			return nil, errors.New("[readerApplyDelta] opcode == 0")
		}
	}
	// TODO: check if resultlen == resultpos
	return
}
