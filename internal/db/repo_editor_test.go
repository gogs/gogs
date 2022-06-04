// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isRepositoryGitPath(t *testing.T) {
	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: ".git", wantVal: true},
		{path: "./.git", wantVal: true},
		{path: ".git/hooks/pre-commit", wantVal: true},
		{path: ".git/hooks", wantVal: true},
		{path: "dir/.git", wantVal: true},

		{path: ".gitignore", wantVal: false},
		{path: "dir/.gitkeep", wantVal: false},

		// Windows-specific
		{path: `.git\`, wantVal: true},
		{path: `.git\hooks\pre-commit`, wantVal: true},
		{path: `.git\hooks`, wantVal: true},
		{path: `dir\.git`, wantVal: true},

		{path: `.\.git.`, wantVal: true},
		{path: `.\.git.\`, wantVal: true},
		{path: `.git.\hooks\pre-commit`, wantVal: true},
		{path: `.git.\hooks`, wantVal: true},
		{path: `dir\.git.`, wantVal: true},

		{path: "./.git.", wantVal: true},
		{path: "./.git./", wantVal: true},
		{path: ".git./hooks/pre-commit", wantVal: true},
		{path: ".git./hooks", wantVal: true},
		{path: "dir/.git.", wantVal: true},

		{path: `dir\.gitkeep`, wantVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitPath(test.path))
		})
	}
}
