// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE.gogs file.

package gitutil

import (
	"os"
	"testing"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/errutil"
)

func TestError_NotFound(t *testing.T) {
	tests := []struct {
		err    error
		expVal bool
	}{
		{err: git.ErrSubmoduleNotExist, expVal: true},
		{err: git.ErrRevisionNotExist, expVal: true},
		{err: git.ErrNoMergeBase, expVal: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, errutil.IsNotFound(NewError(test.err)))
		})
	}
}

func TestIsErrRevisionNotExist(t *testing.T) {
	assert.True(t, IsErrRevisionNotExist(git.ErrRevisionNotExist))
	assert.False(t, IsErrRevisionNotExist(os.ErrNotExist))
}

func TestIsErrNoMergeBase(t *testing.T) {
	assert.True(t, IsErrNoMergeBase(git.ErrNoMergeBase))
	assert.False(t, IsErrNoMergeBase(os.ErrNotExist))
}
