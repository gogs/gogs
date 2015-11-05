// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"bytes"
	"compress/zlib"
	"container/list"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
)

func readIdxFile(path string) (*idxFile, error) {
	ifile := &idxFile{
		indexpath: path,
		packpath:  path[0:len(path)-3] + "pack",
	}

	idx, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if !bytes.HasPrefix(idx, []byte{255, 't', 'O', 'c'}) {
		return nil, errors.New("not version 2 index file")
	}
	pos := 8
	var fanout [256]uint32
	for i := 0; i < 256; i++ {
		// TODO: use range
		fanout[i] = uint32(idx[pos])<<24 + uint32(idx[pos+1])<<16 + uint32(idx[pos+2])<<8 + uint32(idx[pos+3])
		pos += 4
	}
	numObjects := int(fanout[255])
	ids := make([]sha1, numObjects)

	for i := 0; i < numObjects; i++ {
		for j := 0; j < 20; j++ {
			ids[i][j] = idx[pos+j]
		}
		pos = pos + 20
	}
	// skip crc32 and offsetValues4
	pos += 8 * numObjects

	excessLen := len(idx) - 258*4 - 28*numObjects - 40
	var offsetValues8 []uint64
	if excessLen > 0 {
		// We have an index table, so let's read it first
		offsetValues8 = make([]uint64, excessLen/8)
		for i := 0; i < excessLen/8; i++ {
			offsetValues8[i] = uint64(idx[pos])<<070 + uint64(idx[pos+1])<<060 + uint64(idx[pos+2])<<050 + uint64(idx[pos+3])<<040 + uint64(idx[pos+4])<<030 + uint64(idx[pos+5])<<020 + uint64(idx[pos+6])<<010 + uint64(idx[pos+7])
			pos = pos + 8
		}
	}
	ifile.offsetValues = make(map[sha1]uint64, numObjects)
	pos = 258*4 + 24*numObjects
	for i := 0; i < numObjects; i++ {
		offset := uint32(idx[pos])<<24 + uint32(idx[pos+1])<<16 + uint32(idx[pos+2])<<8 + uint32(idx[pos+3])
		offset32ndbit := offset & 0x80000000
		offset31bits := offset & 0x7FFFFFFF
		if offset32ndbit == 0x80000000 {
			// it's an index entry
			ifile.offsetValues[ids[i]] = offsetValues8[offset31bits]
		} else {
			ifile.offsetValues[ids[i]] = uint64(offset31bits)
		}
		pos = pos + 4
	}
	// sha1Packfile := idx[pos : pos+20]
	// sha1Index := idx[pos+21 : pos+40]
	fi, err := os.Open(ifile.packpath)
	if err != nil {
		return nil, err
	}
	defer fi.Close()

	packVersion := make([]byte, 8)
	_, err = fi.Read(packVersion)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(packVersion, []byte{'P', 'A', 'C', 'K'}) {
		return nil, errors.New("pack file does not start with 'PACK'")
	}
	ifile.packversion = uint32(packVersion[4])<<24 + uint32(packVersion[5])<<16 + uint32(packVersion[6])<<8 + uint32(packVersion[7])
	return ifile, nil
}

// readLenInPackFile returns the object length in a packfile.
// It is a bit more difficult than just reading the bytes.
// The first byte has the length in its lowest four bits,
// and if bit 7 is set, it means 'more' bytes will follow.
// These are added to the »left side« of the length
func readLenInPackFile(buf []byte) (int, int) {
	advance := 0
	shift := [...]byte{0, 4, 11, 18, 25, 32, 39, 46, 53, 60}
	length := int(buf[advance] & 0x0F)
	for buf[advance]&0x80 > 0 {
		advance += 1
		length += (int(buf[advance]&0x7F) << shift[advance])
	}
	advance++
	return length, advance
}

// readObjectBytes reads from a pack file (given by path) at position offset.
// If this is a non-delta object, the (inflated) bytes are just returned,
// if the object is a deltafied-object, we have to apply the delta to base objects
// before hand.
func readObjectBytes(path string, indexfiles map[string]*idxFile, offset uint64, sizeonly bool) (ObjectType, int64, io.ReadCloser, error) {
	offsetInt := int64(offset)
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("open object file: %v", err)
	}
	defer file.Close()

	pos, err := file.Seek(offsetInt, os.SEEK_SET)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("seek file: %v", err)
	} else if pos != offsetInt {
		return 0, 0, nil, fmt.Errorf("seek went wrong (pos : offset): %d != %d", pos, offsetInt)
	}

	buf := make([]byte, 1024)
	n, err := file.Read(buf)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("read buf: %v", err)
	} else if n == 0 {
		return 0, 0, nil, fmt.Errorf("nothing read from pack file")
	}

	l, p := readLenInPackFile(buf)
	pos = int64(p)
	length := int64(l)

	var (
		dataRc           io.ReadCloser
		baseObjectOffset uint64
	)

	objType := ObjectType(buf[0] & 0x70)
	switch objType {
	case OBJECT_COMMIT, OBJECT_TREE, OBJECT_BLOB, OBJECT_TAG:
		if sizeonly {
			// if we are only interested in the size of the object,
			// we don't need to do more expensive stuff
			return objType, length, nil, nil
		}

		if _, err = file.Seek(offsetInt+pos, os.SEEK_SET); err != nil {
			return 0, 0, nil, fmt.Errorf("seek file (second): %v", offsetInt, err)
		}

		dataRc, err = readerDecompressed(file, length)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("readerDecompressed: %v", err)
		}
		return objType, length, dataRc, nil
		// data, err = readCompressedDataFromFile(file, offsetInt+pos, length)

	case 0x60:
		// DELTA_ENCODED object w/ offset to base
		// Read the offset first, then calculate the starting point
		// of the base object
		num := int64(buf[pos]) & 0x7f
		for buf[pos]&0x80 > 0 {
			pos = pos + 1
			num = ((num + 1) << 7) | int64(buf[pos]&0x7f)
		}
		baseObjectOffset = uint64(offsetInt - num)
		pos = pos + 1

	case 0x70:
		// DELTA_ENCODED object w/ base BINARY_OBJID
		id, err := NewID(buf[pos : pos+20])
		if err != nil {
			return 0, 0, nil, fmt.Errorf("new ID: %v", err)
		}

		pos = pos + 20

		f := indexfiles[path[0:len(path)-4]+"idx"]
		var ok bool
		if baseObjectOffset, ok = f.offsetValues[id]; !ok {
			return 0, 0, nil, fmt.Errorf("base object: not implemented yet")
		}
	}

	var (
		base   []byte
		baseRc io.ReadCloser
	)
	objType, _, baseRc, err = readObjectBytes(path, indexfiles, baseObjectOffset, false)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("readObjectBytes: %v", err)
	}
	defer baseRc.Close()

	base, err = ioutil.ReadAll(baseRc)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("read all baseRc: %v", err)
	}

	_, err = file.Seek(offsetInt+pos, os.SEEK_SET)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("seek file (third): %v", err)
	}

	rc, err := readerDecompressed(file, length)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("readerDecompressed (second): %v", err)
	}

	zpos := 0
	// This is the length of the base object. Do we need to know it?
	_, bytesRead := readerLittleEndianBase128Number(rc)
	//log.Println(zpos, bytesRead)
	zpos += bytesRead

	resultObjectLength, bytesRead := readerLittleEndianBase128Number(rc)
	zpos += bytesRead

	if sizeonly {
		// if we are only interested in the size of the object,
		// we don't need to do more expensive stuff
		return objType, resultObjectLength, dataRc, nil
	}

	data, err := readerApplyDelta(&readAter{base}, rc, resultObjectLength)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("readerApplyDelta: %v", err)
	}
	return objType, resultObjectLength, newBufReadCloser(data), nil
}

// Return length as integer from zero terminated string
// and the beginning of the real object
func getLengthZeroTerminated(b []byte) (int64, int64) {
	i := 0

	for b[i] != 0 {
		i++
	}
	pos := i
	i--

	var (
		length int64
		pow    int64 = 1
	)
	for i >= 0 {
		length = length + (int64(b[i])-48)*pow
		pow = pow * 10
		i--
	}
	return length, int64(pos) + 1
}

// readObjectFile reads the contents of the object file at path,
// and returns the content type, the contents of the file.
func readObjectFile(path string, sizeonly bool) (ObjectType, int64, io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("open object file: %v", err)
	}

	needClose := true
	defer func() {
		if needClose || sizeonly {
			fmt.Println(file.Close())
		}
	}()

	r, err := zlib.NewReader(file)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("new zlib reader: %v", err)
	}

	firstBufferSize := int64(1024)

	buf := make([]byte, firstBufferSize)
	if _, err = r.Read(buf); err != nil {
		return 0, 0, nil, fmt.Errorf("read buf: %v", err)
	}

	spacePos := int64(bytes.IndexByte(buf, ' '))

	var objType ObjectType
	switch ObjectTypeName(buf[:spacePos]) {
	case OBJETC_TYPE_NAME_BLOB:
		objType = OBJECT_BLOB
	case OBJETC_TYPE_NAME_TREE:
		objType = OBJECT_TREE
	case OBJETC_TYPE_NAME_COMMIT:
		objType = OBJECT_COMMIT
	case OBJETC_TYPE_NAME_TAG:
		objType = OBJECT_TAG
	}

	// length starts at the position after the space
	length, objstart := getLengthZeroTerminated(buf[spacePos+1:])
	if sizeonly {
		return objType, length, nil, nil
	}

	objstart += spacePos + 1

	if _, err = file.Seek(0, os.SEEK_SET); err != nil {
		return 0, 0, nil, fmt.Errorf("seek file: %v", err)
	}

	rc, err := readerDecompressed(file, length+objstart)
	if err != nil {
		return 0, 0, nil, fmt.Errorf("readerDecompressed: %v", err)
	}

	_, err = io.Copy(ioutil.Discard, io.LimitReader(rc, objstart))
	if err != nil {
		return 0, 0, nil, fmt.Errorf("copy to discard: %v", err)
	}

	needClose = false
	return objType, length, newReadCloser(io.LimitReader(rc, length), file), nil
}

const prettyLogFormat = `--pretty=format:%H`

func parsePrettyFormatLog(repo *Repository, logByts []byte) (*list.List, error) {
	l := list.New()
	if len(logByts) == 0 {
		return l, nil
	}

	parts := bytes.Split(logByts, []byte{'\n'})

	for _, commitId := range parts {
		commit, err := repo.GetCommit(string(commitId))
		if err != nil {
			return nil, err
		}
		l.PushBack(commit)
	}

	return l, nil
}

func RefEndName(refStr string) string {
	index := strings.LastIndex(refStr, "/")
	if index != -1 {
		return refStr[index+1:]
	}
	return refStr
}

// filepathFromSHA1 generates and returns object file path
// based on given root path and sha1 ID.
// It assumes the object is stored in its own file (i.e not in a pack file),
// and does not test if the file exists.
func filepathFromSHA1(rootdir, sha1 string) string {
	return filepath.Join(rootdir, "objects", sha1[:2], sha1[2:])
}

// isDir returns true if given path is a directory,
// or returns false when it's a file or does not exist.
func isDir(dir string) bool {
	f, err := os.Stat(dir)
	if err != nil {
		return false
	}
	return f.IsDir()
}

// isFile returns true if given path is a file,
// or returns false when it's a directory or does not exist.
func isFile(filepath string) bool {
	f, err := os.Stat(filepath)
	if err != nil {
		return false
	}
	return !f.IsDir()
}

func concatenateError(err error, stderr string) error {
	if len(stderr) == 0 {
		return err
	}
	return fmt.Errorf("%v: %s", err, stderr)
}
