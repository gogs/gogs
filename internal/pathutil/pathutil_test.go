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
		path   string
		expVal string
	}{
		{
			path:   "../../../readme.txt",
			expVal: "readme.txt",
		},
		{
			path:   "a/../../../readme.txt",
			expVal: "readme.txt",
		},
		{
			path:   "/../a/b/../c/../readme.txt",
			expVal: "a/readme.txt",
		},
		{
			path:   "/a/readme.txt",
			expVal: "a/readme.txt",
		},
		{
			path:   "/",
			expVal: "",
		},

		{
			path:   "/a/b/c/readme.txt",
			expVal: "a/b/c/readme.txt",
		},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, Clean(test.path))
		})
	}
}
