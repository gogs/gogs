package osutil

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
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
			assert.Equal(t, test.expVal, errutil.IsNotFound(NewError(test.err)))
		})
	}
}
