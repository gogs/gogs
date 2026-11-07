package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPathsToMoveAside(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(root, "gogs-backup")
	customDir := filepath.Join(root, "custom")
	appDataPath := filepath.Join(root, "data")
	for _, dir := range []string{
		customDir,
		filepath.Join(archivePath, "data", "attachments"),
		filepath.Join(archivePath, "data", "avatars"),
		filepath.Join(appDataPath, "attachments"),
		filepath.Join(appDataPath, "repo-avatars"),
	} {
		require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	}

	// "avatars" is only in the archive and "repo-avatars" is only on disk, so neither is moved aside.
	want := []string{customDir, filepath.Join(appDataPath, "attachments")}
	assert.Equal(t, want, pathsToMoveAside(archivePath, customDir, appDataPath))

	require.NoError(t, os.RemoveAll(customDir))
	assert.Equal(t, []string{filepath.Join(appDataPath, "attachments")}, pathsToMoveAside(archivePath, customDir, appDataPath))
}
