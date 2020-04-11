// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package osutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsFile(t *testing.T) {
	tests := []struct {
		path   string
		expVal bool
	}{
		{
			path:   "osutil.go",
			expVal: true,
		}, {
			path:   "../osutil",
			expVal: false,
		}, {
			path:   "not_found",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, IsFile(test.path))
		})
	}
}

func TestIsDir(t *testing.T) {
	tests := []struct {
		path   string
		expVal bool
	}{
		{
			path:   "osutil.go",
			expVal: false,
		}, {
			path:   "../osutil",
			expVal: true,
		}, {
			path:   "not_found",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, IsDir(test.path))
		})
	}
}

func TestIsExist(t *testing.T) {
	tests := []struct {
		path   string
		expVal bool
	}{
		{
			path:   "osutil.go",
			expVal: true,
		}, {
			path:   "../osutil",
			expVal: true,
		}, {
			path:   "not_found",
			expVal: false,
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, IsExist(test.path))
		})
	}
}

func TestCurrentUsername(t *testing.T) {
	// Make sure it does not blow up
	CurrentUsername()
}
