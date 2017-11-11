// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errors

import "errors"

// New is a wrapper of real errors.New function.
func New(text string) error {
	return errors.New(text)
}
