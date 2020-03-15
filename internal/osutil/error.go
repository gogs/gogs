// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package osutil

import (
	"os"

	"gogs.io/gogs/internal/errutil"
)

var _ errutil.NotFound = (*Error)(nil)

// Error is a wrapper of an OS error, which handles not found.
type Error struct {
	error
}

func (e Error) NotFound() bool {
	return e.error == os.ErrNotExist
}

// NewError wraps given error.
func NewError(err error) error {
	return Error{error: err}
}
