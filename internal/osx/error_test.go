package osx

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errx"
)

func TestError_NotFound(t *testing.T) {
	tests := []struct {
		err    error
		expVal bool
	}{
		{err: os.ErrNotExist, expVal: true},
		{err: os.ErrClosed, expVal: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, errx.IsNotFound(NewError(test.err)))
		})
	}
}
