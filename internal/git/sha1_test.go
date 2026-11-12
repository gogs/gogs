package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewID(t *testing.T) {
	sha, err := newID([]byte("000000"))
	assert.EqualError(t, err, "length must be 20")
	assert.Nil(t, sha)
}

func TestNewIDFromString(t *testing.T) {
	sha, err := newIDFromString("000000")
	assert.EqualError(t, err, "length must be 40")
	assert.Nil(t, sha)
}
