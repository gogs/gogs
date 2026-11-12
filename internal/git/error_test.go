package git

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
		{err: ErrSubmoduleNotExist, expVal: true},
		{err: ErrRevisionNotExist, expVal: true},
		{err: ErrNoMergeBase, expVal: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, errx.IsNotFound(NewError(test.err)))
		})
	}
}

func TestIsErrRevisionNotExist(t *testing.T) {
	assert.True(t, IsErrRevisionNotExist(ErrRevisionNotExist))
	assert.False(t, IsErrRevisionNotExist(os.ErrNotExist))
}

func TestIsErrNoMergeBase(t *testing.T) {
	assert.True(t, IsErrNoMergeBase(ErrNoMergeBase))
	assert.False(t, IsErrNoMergeBase(os.ErrNotExist))
}
