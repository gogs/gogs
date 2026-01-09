package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInTest(t *testing.T) {
	assert.True(t, InTest)
}
