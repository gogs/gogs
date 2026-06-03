package database

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHasGitBasename(t *testing.T) {
	// hasGitBasename receives paths that have already been normalized by
	// pathx.Clean at the route layer: no leading slashes, no "..", and
	// backslashes converted to forward slashes.
	tests := []struct {
		path    string
		wantVal bool
	}{
		{path: ".git", wantVal: true},
		{path: "dir/.git", wantVal: true},
		{path: "a/b/c/.git", wantVal: true},

		// Case-insensitive file system.
		{path: ".Git", wantVal: true},
		{path: "dir/.Git", wantVal: true},

		// Windows treats ".git." the same as ".git".
		{path: ".git.", wantVal: true},
		{path: "dir/.git.", wantVal: true},

		{path: ".gitignore", wantVal: false},
		{path: "dir/.gitkeep", wantVal: false},

		// With separate-gitdir, ".git" is a regular file at the worktree root,
		// so paths that name it as an intermediate component cannot resolve to
		// the redirector — the kernel rejects them with ENOTDIR.
		{path: ".git/hooks", wantVal: false},
		{path: ".git/hooks/pre-commit", wantVal: false},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			assert.Equal(t, test.wantVal, hasGitBasename(test.path))
		})
	}
}

func TestRejectSymlinkTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "regular"), []byte("x"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(root, "dir"), 0o700))
	require.NoError(t, os.Symlink("regular", filepath.Join(root, "link-to-file")))
	require.NoError(t, os.Symlink("dir", filepath.Join(root, "link-to-dir")))
	require.NoError(t, os.Symlink("missing", filepath.Join(root, "dangling")))

	r, err := os.OpenRoot(root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	tests := []struct {
		name      string
		relPath   string
		wantError bool
	}{
		{name: "missing file is allowed", relPath: "new-file", wantError: false},
		{name: "regular file is allowed", relPath: "regular", wantError: false},
		{name: "directory is allowed", relPath: "dir", wantError: false},
		{name: "symlink to file is rejected", relPath: "link-to-file", wantError: true},
		{name: "symlink to directory is rejected", relPath: "link-to-dir", wantError: true},
		{name: "dangling symlink is rejected", relPath: "dangling", wantError: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := rejectSymlinkTarget(r, tc.relPath)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestOsRootRefusesEscapingSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink semantics differ on Windows")
	}

	parent := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(parent, "outside"), []byte("victim"), 0o600))

	root := filepath.Join(parent, "root")
	require.NoError(t, os.Mkdir(root, 0o700))
	require.NoError(t, os.Symlink("../outside", filepath.Join(root, "escape")))

	r, err := os.OpenRoot(root)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r.Close() })

	_, err = r.OpenFile("escape", os.O_WRONLY|os.O_TRUNC, 0o600)
	assert.Error(t, err, "os.Root must refuse to follow a symlink that escapes the root")

	victim, readErr := os.ReadFile(filepath.Join(parent, "outside"))
	require.NoError(t, readErr)
	assert.Equal(t, "victim", string(victim), "the file outside the root must be untouched")
}
