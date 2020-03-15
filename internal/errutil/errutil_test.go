// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package errutil

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

type notFoundError struct {
	val bool
}

func (notFoundError) Error() string {
	return "not found"
}

func (e notFoundError) NotFound() bool {
	return e.val
}

func TestIsNotFound(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		expVal bool
	}{
		{
			name:   "error does not implement NotFound",
			err:    errors.New("a simple error"),
			expVal: false,
		},
		{
			name:   "error implements NotFound but not a not found",
			err:    notFoundError{val: false},
			expVal: false,
		},
		{
			name:   "error implements NotFound",
			err:    notFoundError{val: true},
			expVal: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsNotFound(test.err))
		})
	}
}
