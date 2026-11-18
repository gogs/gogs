package database

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"gogs.io/gogs/internal/conf"
)

func TestNewTemporaryRepository(t *testing.T) {
	appDataPath := t.TempDir()
	conf.SetMockServer(t, conf.ServerOpts{AppDataPath: appDataPath})

	_, _, err := newTemporaryRepository(context.Background(), filepath.Join(t.TempDir(), "missing"))
	assert.Error(t, err)
	entries, err := os.ReadDir(filepath.Join(appDataPath, "tmp", "repos"))
	assert.NoError(t, err)
	assert.Empty(t, entries, "temporary repository should be removed")
}
