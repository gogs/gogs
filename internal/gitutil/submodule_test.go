// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package gitutil

import (
	"testing"

	"github.com/gogs/git-module"
	"github.com/stretchr/testify/assert"
)

func TestInferSubmoduleURL(t *testing.T) {
	tests := []struct {
		name      string
		submodule *git.Submodule
		expURL    string
	}{
		{
			name: "HTTPS URL",
			submodule: &git.Submodule{
				URL:    "https://github.com/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL with port",
			submodule: &git.Submodule{
				URL:    "ssh://user@github.com:22/gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com:22/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "SSH URL in SCP syntax",
			submodule: &git.Submodule{
				URL:    "git@github.com:gogs/docs-api.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "http://github.com/gogs/docs-api/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "relative path",
			submodule: &git.Submodule{
				URL:    "../repo2.git",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "https://gogs.example.com/user/repo/../repo2/commit/6b08f76a5313fa3d26859515b30aa17a5faa2807",
		},
		{
			name: "bad URL",
			submodule: &git.Submodule{
				URL:    "ftp://example.com",
				Commit: "6b08f76a5313fa3d26859515b30aa17a5faa2807",
			},
			expURL: "ftp://example.com",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expURL, InferSubmoduleURL("https://gogs.example.com/user/repo", test.submodule))
		})
	}
}
