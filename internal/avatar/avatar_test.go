package avatar

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerate(t *testing.T) {
	alice, err := Generate(Person, "alice")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(alice), "<svg "))

	again, err := Generate(Person, "alice")
	require.NoError(t, err)
	assert.Equal(t, alice, again, "same seed draws a different avatar")

	bob, err := Generate(Person, "bob")
	require.NoError(t, err)
	assert.NotEqual(t, alice, bob, "different seeds draw the same avatar")

	org, err := Generate(Organization, "alice")
	require.NoError(t, err)
	assert.NotEqual(t, alice, org, "organizations are drawn like people")
}
