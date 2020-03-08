// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"os"
	"testing"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
)

func TestIsErrRevisionNotExist(t *testing.T) {
	assert.True(t, IsErrRevisionNotExist(git.ErrRevisionNotExist))
	assert.False(t, IsErrRevisionNotExist(os.ErrNotExist))
}

func TestIsErrNoMergeBase(t *testing.T) {
	assert.True(t, IsErrNoMergeBase(git.ErrNoMergeBase))
	assert.False(t, IsErrNoMergeBase(os.ErrNotExist))
}
