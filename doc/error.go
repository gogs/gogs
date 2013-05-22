// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package doc

import (
	"errors"
)

var (
	errNotModified   = errors.New("package not modified")
	errUpdateTimeout = errors.New("update timeout")
)

type NotFoundError struct {
	Message string
}

func (e NotFoundError) Error() string {
	return e.Message
}

func isNotFound(err error) bool {
	_, ok := err.(NotFoundError)
	return ok
}

type RemoteError struct {
	Host string
	err  error
}

func (e *RemoteError) Error() string {
	return e.err.Error()
}
