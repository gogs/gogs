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
		path   string
		expVal bool
	}{
		{path: filepath.Join(".", ".git"), expVal: true},
		{path: filepath.Join(".", ".git", ""), expVal: true},
		{path: filepath.Join(".", ".git", "hooks", "pre-commit"), expVal: true},
		{path: filepath.Join(".git", "hooks"), expVal: true},
		{path: filepath.Join("dir", ".git"), expVal: true},

		{path: filepath.Join(".gitignore"), expVal: false},
		{path: filepath.Join("dir", ".gitkeep"), expVal: false},
	}
	for _, test := range tests {
		t.Run("", func(t *testing.T) {
			assert.Equal(t, test.expVal, isRepositoryGitPath(test.path))
		})
	}
}
