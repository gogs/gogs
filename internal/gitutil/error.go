// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"github.com/gogs/git-module"

	"gogs.io/gogs/internal/errutil"
)

var _ errutil.NotFound = (*Error)(nil)

// Error is a wrapper of a Git error, which handles not found.
type Error struct {
	error
}

func (e Error) NotFound() bool {
	return IsErrSubmoduleNotExist(e.error) ||
		IsErrRevisionNotExist(e.error)

}

// NewError wraps given error.
func NewError(err error) error {
	return Error{error: err}
}

// IsErrSubmoduleNotExist returns true if the error is git.ErrSubmoduleNotExist.
func IsErrSubmoduleNotExist(err error) bool {
	return err == git.ErrSubmoduleNotExist
}

// IsErrRevisionNotExist returns true if the error is git.ErrRevisionNotExist.
func IsErrRevisionNotExist(err error) bool {
	return err == git.ErrRevisionNotExist
}

// IsErrNoMergeBase returns true if the error is git.ErrNoMergeBase.
func IsErrNoMergeBase(err error) bool {
	return err == git.ErrNoMergeBase
}
