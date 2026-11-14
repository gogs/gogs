package git

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	assert.IsType(t, &SHA1{}, sha)
	assert.Equal(t, "0eedd79eba4394bbef888c804e899731644367fe", sha.String())

	sha, err = newIDFromString("31a3d8bb18edda3252f7242035d4695ee9b8ddbd47d7c62f656756442c2e85af")
	require.NoError(t, err)
	assert.IsType(t, &SHA256{}, sha)
	assert.Equal(t, "31a3d8bb18edda3252f7242035d4695ee9b8ddbd47d7c62f656756442c2e85af", sha.String())
}

func TestObjectFormat_EmptyTreeID(t *testing.T) {
	ctx := context.Background()
	for _, r := range []*Repository{testrepo, newSHA256TestRepo(t)} {
		objectFormat, err := r.ObjectFormat(ctx)
		require.NoError(t, err)

		want, err := execInput(ctx, r.path, []string{"hash-object", "-t", "tree", "--stdin"}, nil, strings.NewReader(""))
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(string(want)), objectFormat.EmptyTreeID(), objectFormat)
	}
}

func TestIsEmptyID(t *testing.T) {
	assert.True(t, IsEmptyID(ObjectFormatSHA1.EmptyID()))
	assert.True(t, IsEmptyID(ObjectFormatSHA256.EmptyID()))
	assert.Len(t, ObjectFormatSHA256.EmptyID(), 64)
	assert.False(t, IsEmptyID("0000000"))
	assert.False(t, IsEmptyID("0eedd79eba4394bbef888c804e899731644367fe"))
}
