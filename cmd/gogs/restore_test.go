package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/osx"
)

func TestMoveAside(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "custom")
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))

	require.NoError(t, moveAside(dir))
	assert.False(t, osx.Exist(dir))
	assert.True(t, osx.IsDir(dir+".bak"))

	// A leftover backup from an earlier restore is never overwritten.
	require.NoError(t, os.MkdirAll(dir, os.ModePerm))
	err := moveAside(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
	assert.True(t, osx.IsDir(dir))
}
