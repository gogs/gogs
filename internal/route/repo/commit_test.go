package repo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHasMultipleParents(t *testing.T) {
	assert.False(t, hasMultipleParents(nil))
	assert.False(t, hasMultipleParents([]string{"parent-a"}))
	assert.True(t, hasMultipleParents([]string{"parent-a", "parent-b"}))
}
