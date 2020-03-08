// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"
)

// IsErrRevisionNotExist returns true if the error is git.ErrRevisionNotExist.
func IsErrRevisionNotExist(err error) bool {
	return err == git.ErrRevisionNotExist
}

// IsErrNoMergeBase returns true if the error is git.ErrNoMergeBase.
func IsErrNoMergeBase(err error) bool {
	return err == git.ErrNoMergeBase
}
