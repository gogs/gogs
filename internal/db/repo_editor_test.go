// Copyright 2018 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isRepositoryGitPath(t *testing.T) {
	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: filepath.Join(".", ".git"), wantVal: true},
		{path: filepath.Join(".", ".git", ""), wantVal: true},
		{path: filepath.Join(".", ".git", "hooks", "pre-commit"), wantVal: true},
		{path: filepath.Join(".git", "hooks"), wantVal: true},
		{path: filepath.Join("dir", ".git"), wantVal: true},

		{path: filepath.Join(".", ".git."), wantVal: true},
		{path: filepath.Join(".", ".git.", ""), wantVal: true},
		{path: filepath.Join(".", ".git.", "hooks", "pre-commit"), wantVal: true},
		{path: filepath.Join(".git.", "hooks"), wantVal: true},
		{path: filepath.Join("dir", ".git."), wantVal: true},

		{path: filepath.Join(".gitignore"), wantVal: false},
		{path: filepath.Join("dir", ".gitkeep"), wantVal: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitPath(test.path))
		})
	}
}
