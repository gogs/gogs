// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package pathutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClean(t *testing.T) {
	tests := []struct {
		path    string
		wantVal string
	}{
		{
			path:    "../../../readme.txt",
			wantVal: "readme.txt",
		},
		{
			path:    "a/../../../readme.txt",
			wantVal: "readme.txt",
		},
		{
			path:    "/../a/b/../c/../readme.txt",
			wantVal: "a/readme.txt",
		},
		{
			path:    "/a/readme.txt",
			wantVal: "a/readme.txt",
		},
		{
			path:    "/",
			wantVal: "",
		},

		{
			path:    "/a/b/c/readme.txt",
			wantVal: "a/b/c/readme.txt",
		},

		// Windows-specific
		{
			path:    `..\..\..\readme.txt`,
			wantVal: "readme.txt",
		},
		{
			path:    `a\..\..\..\readme.txt`,
			wantVal: "readme.txt",
		},
		{
			path:    `\..\a\b\..\c\..\readme.txt`,
			wantVal: "a/readme.txt",
		},
		{
			path:    `\a\readme.txt`,
			wantVal: "a/readme.txt",
		},
		{
			path:    `..\..\..\../README.md`,
			wantVal: "README.md",
		},
		{
			path:    `\`,
			wantVal: "",
		},

		{
			path:    `\a\b\c\readme.txt`,
			wantVal: `a/b/c/readme.txt`,
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, Clean(test.path))
		})
	}
}
