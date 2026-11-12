package git

import (
	"encoding/hex"
	"strings"
	"sync"

	"github.com/cockroachdb/errors"
)

// ObjectFormat is the hash algorithm of a Git repository.
type ObjectFormat string

// A list of object formats.
const (
	ObjectFormatSHA1   ObjectFormat = "sha1"
	ObjectFormatSHA256 ObjectFormat = "sha256"
)

// EmptyID returns the ID with all zeros in the object format, which Git uses as
// the old ID of a created reference and the new ID of a deleted one.
func (f ObjectFormat) EmptyID() string {
	if f == ObjectFormatSHA256 {
		return strings.Repeat("0", 64)
	}
	return strings.Repeat("0", 40)
}

// IsEmptyID returns true if the ID is the empty ID of any object format.
func IsEmptyID(id string) bool {
	return id == ObjectFormatSHA1.EmptyID() || id == ObjectFormatSHA256.EmptyID()
}

// Oid is the ID of a Git object, i.e., a *SHA1 or a *SHA256 depending on the
// object format of the repository.
type Oid interface {
	// String returns the hex representation of the ID.
	String() string
}

// SHA1 is the SHA-1 hash of a Git object.
type SHA1 struct {
	bytes [20]byte

	str     string
	strOnce sync.Once
}

// String returns string (hex) representation of the SHA1.
func (s *SHA1) String() string {
	s.strOnce.Do(func() {
		s.str = hex.EncodeToString(s.bytes[:])
	})
	return s.str
}

// SHA256 is the SHA-256 hash of a Git object.
type SHA256 struct {
	bytes [32]byte

	str     string
	strOnce sync.Once
}

// String returns string (hex) representation of the SHA256.
func (s *SHA256) String() string {
	s.strOnce.Do(func() {
		s.str = hex.EncodeToString(s.bytes[:])
	})
	return s.str
}

// mustID always returns a new Oid from a 20-byte (SHA-1) or 32-byte (SHA-256)
// slice with no validation of input.
func mustID(b []byte) Oid {
	if len(b) == 32 {
		var id SHA256
		copy(id.bytes[:], b)
		return &id
	}
	var id SHA1
	copy(id.bytes[:], b)
	return &id
}

// newID returns a new Oid from a 20-byte (SHA-1) or 32-byte (SHA-256) slice.
func newID(b []byte) (Oid, error) {
	if len(b) != 20 && len(b) != 32 {
		return nil, errors.New("length must be 20 or 32")
	}
	return mustID(b), nil
}

// MustIDFromString always returns a new Oid from a hex ID with no validation of
// input.
func MustIDFromString(s string) Oid {
	b, _ := hex.DecodeString(s)
	return mustID(b)
}

// newIDFromString returns a new Oid from a hex ID of length 40 (SHA-1) or 64
// (SHA-256).
func newIDFromString(s string) (Oid, error) {
	s = strings.TrimSpace(s)
	if len(s) != 40 && len(s) != 64 {
		return nil, errors.New("length must be 40 or 64")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, err
	}
	return newID(b)
}
