package tool

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicAuthDecode(t *testing.T) {
	username, password, err := BasicAuthDecode(base64.StdEncoding.EncodeToString([]byte("username:password")))
	require.NoError(t, err)
	assert.Equal(t, "username", username)
	assert.Equal(t, "password", password)
}

func TestBasicAuthDecode_InvalidBase64(t *testing.T) {
	username, password, err := BasicAuthDecode("not_base64")
	require.Error(t, err)
	assert.Empty(t, username)
	assert.Empty(t, password)
}

func TestBasicAuthDecode_MalformedCredentials(t *testing.T) {
	assert.NotPanics(t, func() {
		username, password, err := BasicAuthDecode(base64.StdEncoding.EncodeToString([]byte("username")))
		require.Error(t, err)
		assert.Empty(t, username)
		assert.Empty(t, password)
	})
}
