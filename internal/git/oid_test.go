package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewID(t *testing.T) {
	sha, err := newID([]byte("000000"))
	assert.EqualError(t, err, "length must be 20 or 32")
	assert.Nil(t, sha)
}

func TestNewIDFromString(t *testing.T) {
	sha, err := newIDFromString("000000")
	assert.EqualError(t, err, "length must be 40 or 64")
	assert.Nil(t, sha)

	sha, err = newIDFromString("0eedd79eba4394bbef888c804e899731644367fe")
	assert.NoError(t, err)
	assert.IsType(t, &SHA1{}, sha)
	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", sha.String())

	sha, err = newIDFromString("31a3d8bb18edda3252f7242035d4695ee9b8ddbd47d7c62f656756442c2e85af")
	assert.NoError(t, err)
	assert.IsType(t, &SHA256{}, sha)
	assert.Equal(t, "31a3d8bb18edda3252f7242035d4695ee9b8ddbd47d7c62f656756442c2e85af", sha.String())
}

func TestIsEmptyID(t *testing.T) {
	assert.True(t, IsEmptyID(ObjectFormatSHA1.EmptyID()))
	assert.True(t, IsEmptyID(ObjectFormatSHA256.EmptyID()))
	assert.Len(t, ObjectFormatSHA256.EmptyID(), 64)
	assert.False(t, IsEmptyID("0000000"))
	assert.False(t, IsEmptyID("0eedd79eba4394bbef888c804e899731644367fe"))
}
