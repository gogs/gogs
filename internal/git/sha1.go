package git

import (
	"encoding/hex"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
)

// EmptyID is an ID with empty SHA-1 hash.
const EmptyID = "0000000000000000000000000000000000000000"

// SHA1 is the SHA-1 hash of a Git object.
type SHA1 struct {
	bytes [20]byte

	str     string
	strOnce sync.Once
}

// String returns string (hex) representation of the SHA1.
func (s *SHA1) String() string {
	s.strOnce.Do(func() {
		result := make([]byte, 0, 40)
		hexvalues := []byte("0123456789abcdef")
		for i := 0; i < 20; i++ {
			result = append(result, hexvalues[s.bytes[i]>>4])
			result = append(result, hexvalues[s.bytes[i]&0xf])
		}
		s.str = string(result)
	})
	return s.str
}

// mustID always returns a new SHA1 from the first 20 bytes of the slice with no
// validation of input.
func mustID(b []byte) *SHA1 {
	var id SHA1
	for i := 0; i < 20; i++ {
		id.bytes[i] = b[i]
	}
	return &id
}

// newID returns a new SHA1 from a 20-byte slice.
func newID(b []byte) (*SHA1, error) {
	if len(b) != 20 {
		return nil, errors.New("length must be 20")
	}
	return mustID(b), nil
}

// MustIDFromString always returns a new sha from a ID with no validation of
// input.
func MustIDFromString(s string) *SHA1 {
	b, _ := hex.DecodeString(s)
	return mustID(b)
}

// newIDFromString returns a new SHA1 from a ID string of length 40.
func newIDFromString(s string) (*SHA1, error) {
	s = strings.TrimSpace(s)
	if len(s) != 40 {
		return nil, errors.New("length must be 40")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return newID(b)
}
