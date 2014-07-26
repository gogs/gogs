// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package git

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

var (
	IdNotExist = errors.New("sha1 id not exist")
)

type sha1 [20]byte

// Return true if s has the same sha1 as caller.
// Support 40-length-string, []byte, sha1
func (id sha1) Equal(s2 interface{}) bool {
	switch v := s2.(type) {
	case string:
		if len(v) != 40 {
			return false
		}
		return v == id.String()
	case []byte:
		if len(v) != 20 {
			return false
		}
		for i, v := range v {
			if id[i] != v {
				return false
			}
		}
	case sha1:
		for i, v := range v {
			if id[i] != v {
				return false
			}
		}
	default:
		return false
	}
	return true
}

// Return string (hex) representation of the Oid
func (s sha1) String() string {
	result := make([]byte, 0, 40)
	hexvalues := []byte("0123456789abcdef")
	for i := 0; i < 20; i++ {
		result = append(result, hexvalues[s[i]>>4])
		result = append(result, hexvalues[s[i]&0xf])
	}
	return string(result)
}

// Create a new sha1 from a 20 byte slice.
func NewId(b []byte) (sha1, error) {
	var id sha1
	if len(b) != 20 {
		return id, errors.New("Length must be 20")
	}

	for i := 0; i < 20; i++ {
		id[i] = b[i]
	}
	return id, nil
}

// Create a new sha1 from a Sha1 string of length 40.
func NewIdFromString(s string) (sha1, error) {
	s = strings.TrimSpace(s)
	var id sha1
	if len(s) != 40 {
		return id, fmt.Errorf("Length must be 40")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return id, err
	}

	return NewId(b)
}
