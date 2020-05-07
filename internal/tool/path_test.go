// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package tool

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_IsSameSiteURLPath(t *testing.T) {
	tests := []struct {
		url    string
		expVal bool
	}{
		{url: "//github.com", expVal: false},
		{url: "http://github.com", expVal: false},
		{url: "https://github.com", expVal: false},
		{url: "/\\github.com", expVal: false},

		{url: "/admin", expVal: true},
		{url: "/user/repo", expVal: true},
	}

	for _, test := range tests {
		t.Run(test.url, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsSameSiteURLPath(test.url))
		})
	}
}

func Test_IsMaliciousPath(t *testing.T) {
	tests := []struct {
		path   string
		expVal bool
	}{
		{path: "../../../../../../../../../data/gogs/data/sessions/a/9/a9f0ab6c3ef63dd8", expVal: true},
		{path: "..\\/..\\/../data/gogs/data/sessions/a/9/a9f0ab6c3ef63dd8", expVal: true},
		{path: "data/gogs/../../../../../../../../../data/sessions/a/9/a9f0ab6c3ef63dd8", expVal: true},
		{path: "..\\..\\..\\..\\..\\..\\..\\..\\..\\data\\gogs\\data\\sessions\\a\\9\\a9f0ab6c3ef63dd8", expVal: true},
		{path: "data\\gogs\\..\\..\\..\\..\\..\\..\\..\\..\\..\\data\\sessions\\a\\9\\a9f0ab6c3ef63dd8", expVal: true},

		{path: "data/sessions/a/9/a9f0ab6c3ef63dd8", expVal: false},
		{path: "data\\sessions\\a\\9\\a9f0ab6c3ef63dd8", expVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.expVal, IsMaliciousPath(test.path))
		})
	}
}
