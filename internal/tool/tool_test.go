package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBasicAuthDecode(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		user, pass, err := BasicAuthDecode("Zm9vOmJhcg==")
		assert.NoError(t, err)
		assert.Equal(t, "foo", user)
		assert.Equal(t, "bar", pass)
	})

	t.Run("password contains a colon", func(t *testing.T) {
		user, pass, err := BasicAuthDecode("Zm9vOmJhcjpiYXo=")
		assert.NoError(t, err)
		assert.Equal(t, "foo", user)
		assert.Equal(t, "bar:baz", pass)
	})

	t.Run("no colon does not panic", func(t *testing.T) {
		user, pass, err := BasicAuthDecode("Zm9vYmFy")
		assert.NoError(t, err)
		assert.Equal(t, "foobar", user)
		assert.Empty(t, pass)
	})

	t.Run("invalid base64", func(t *testing.T) {
		_, _, err := BasicAuthDecode("not base64!")
		assert.Error(t, err)
	})
}
