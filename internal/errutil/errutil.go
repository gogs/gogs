// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errutil

// NotFound represents a not found error.
type NotFound interface {
	NotFound() bool
}

// IsNotFound returns true if the error is a not found error.
func IsNotFound(err error) bool {
	e, ok := err.(NotFound)
	return ok && e.NotFound()
}

// Args is a map of key-value pairs to provide additional context of an error.
type Args map[string]interface{}
