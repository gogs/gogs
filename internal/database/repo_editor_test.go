package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsRepositoryGitPath(t *testing.T) {
	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: ".git", wantVal: true},
		{path: "./.git", wantVal: true},
		{path: ".git/hooks/pre-commit", wantVal: true},
		{path: ".git/hooks", wantVal: true},
		{path: "dir/.git", wantVal: true},

		// Case-insensitive file system
		{path: ".Git", wantVal: true},
		{path: "./.Git", wantVal: true},
		{path: ".Git/hooks/pre-commit", wantVal: true},
		{path: ".Git/hooks", wantVal: true},
		{path: "dir/.Git", wantVal: true},

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

		// Windows also strips trailing spaces from each path component
		{path: ".git ", wantVal: true},
		{path: ".git /hooks/pre-commit", wantVal: true},
		{path: ".git /hooks", wantVal: true},
		{path: `.git \hooks`, wantVal: true},
		{path: "dir/.git ", wantVal: true},
		{path: `dir\.git `, wantVal: true},
		// Combinations of trailing dots and spaces
		{path: ".git. ", wantVal: true},
		{path: ".git .", wantVal: true},
		{path: ".git. /hooks", wantVal: true},

		// NTFS alternate data streams resolve to the stream owner
		{path: ".git::$INDEX_ALLOCATION", wantVal: true},
		{path: ".git::$INDEX_ALLOCATION/hooks/pre-commit", wantVal: true},
		{path: `.git::$INDEX_ALLOCATION\hooks\pre-commit`, wantVal: true},
		{path: ".Git:$I30:$INDEX_ALLOCATION/hooks", wantVal: true},
		{path: ".git .::$INDEX_ALLOCATION/hooks", wantVal: true},
		{path: "dir/.git::$INDEX_ALLOCATION", wantVal: true},

		// NTFS 8.3 short name
		{path: "GIT~1", wantVal: true},
		{path: "git~1/hooks/pre-commit", wantVal: true},
		{path: `GIT~1\hooks\pre-commit`, wantVal: true},
		{path: "GIT~1./hooks", wantVal: true},
		{path: "GIT~1::$INDEX_ALLOCATION/hooks/pre-commit", wantVal: true},
		{path: "dir/git~1", wantVal: true},

		{path: `dir\.gitkeep`, wantVal: false},
		{path: ".gitignore ", wantVal: false},
		{path: ".gitignore::$DATA", wantVal: false},
		{path: "git", wantVal: false},
		{path: "git~10", wantVal: false},
		{path: "git~1x/hooks", wantVal: false},
		{path: "git~2/hooks", wantVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitPath(test.path))
		})
	}
}
