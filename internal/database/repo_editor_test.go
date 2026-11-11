package database

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

		// NTFS stream suffixes resolve to the stream owner
		{path: ".git::$INDEX_ALLOCATION", wantVal: true},
		{path: ".git::$INDEX_ALLOCATION/hooks/pre-commit", wantVal: true},
		{path: `.git::$INDEX_ALLOCATION\hooks\pre-commit`, wantVal: true},
		{path: ".Git:$I30:$INDEX_ALLOCATION/hooks", wantVal: true},
		{path: ".git .::$INDEX_ALLOCATION/hooks", wantVal: true},
		{path: "dir/.git::$INDEX_ALLOCATION", wantVal: true},

		{path: `dir\.gitkeep`, wantVal: false},
		{path: ".gitignore ", wantVal: false},
		{path: ".gitignore::$DATA", wantVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitPath(test.path))
		})
	}
}

func TestIsRepositoryGitDirInPath(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ".git", "hooks"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(base, "dir"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(base, "dir", "file"), nil, 0o600))

	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: ".git", wantVal: true},
		{path: ".git/hooks", wantVal: true},
		{path: ".git/hooks/pre-commit", wantVal: true},

		{path: "", wantVal: false},
		{path: "dir", wantVal: false},
		{path: "dir/file", wantVal: false},
		{path: "dir/new/file", wantVal: false},
		{path: "missing/.git", wantVal: false},
	}

	// Aliases resolve to the same directory on case-insensitive file systems.
	if _, err := os.Stat(filepath.Join(base, ".GIT")); err == nil {
		tests = append(tests, struct {
			path    string
			wantVal bool
		}{path: ".GIT/hooks/pre-commit", wantVal: true})
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, isRepositoryGitDirInPath(base, test.path))
		})
	}
}
