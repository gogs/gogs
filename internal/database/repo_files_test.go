package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTemporaryRepository(t *testing.T) {
	appDataPath := t.TempDir()

	_, _, err := newTemporaryRepository(context.Background(), appDataPath, filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
	entries, err := os.ReadDir(filepath.Join(appDataPath, "tmp", "repos"))
	assert.NoError(t, err)
	assert.Empty(t, entries, "temporary repository should be removed")
}
