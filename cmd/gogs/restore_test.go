package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	p, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(p)
}

func TestRestoreCustomDir(t *testing.T) {
	t.Run("keeps data directory in place", func(t *testing.T) {
		root := t.TempDir()
		customDir := filepath.Join(root, "custom")
		writeFile(t, filepath.Join(customDir, "conf", "app.ini"), "current")
		writeFile(t, filepath.Join(customDir, "data", "gogs.db"), "current")
		writeFile(t, filepath.Join(customDir, "templates", "home.tmpl"), "current")
		dbBefore, err := os.Stat(filepath.Join(customDir, "data", "gogs.db"))
		require.NoError(t, err)

		srcDir := filepath.Join(root, "archive", "custom")
		writeFile(t, filepath.Join(srcDir, "conf", "app.ini"), "restored")
		writeFile(t, filepath.Join(srcDir, "data", "gogs.db"), "restored")

		require.NoError(t, restoreCustomDir(srcDir, customDir))

		assert.Equal(t, "restored", readFile(t, filepath.Join(customDir, "conf", "app.ini")))
		assert.Equal(t, "current", readFile(t, filepath.Join(customDir, "conf.bak", "app.ini")))
		assert.Equal(t, "current", readFile(t, filepath.Join(customDir, "templates", "home.tmpl")))

		dbAfter, err := os.Stat(filepath.Join(customDir, "data", "gogs.db"))
		require.NoError(t, err)
		assert.True(t, os.SameFile(dbBefore, dbAfter), "the open database file must not be replaced")
		assert.Equal(t, "current", readFile(t, filepath.Join(customDir, "data", "gogs.db")))
		assert.NoDirExists(t, filepath.Join(customDir, "data.bak"))
	})

	for _, test := range []struct {
		name        string
		appDataPath func(t *testing.T, customDir string) string
	}{
		{
			name:        "keeps app data directories in place",
			appDataPath: func(_ *testing.T, customDir string) string { return customDir },
		},
		{
			name: "keeps app data directories in place through symlink",
			appDataPath: func(t *testing.T, customDir string) string {
				link := filepath.Join(filepath.Dir(customDir), "custom-link")
				if err := os.Symlink(customDir, link); err != nil {
					t.Skip("Symlinks are not supported:", err)
				}
				return link
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			customDir := filepath.Join(root, "custom")
			writeFile(t, filepath.Join(customDir, "attachments", "a"), "current")
			before := conf.Server.AppDataPath
			conf.Server.AppDataPath = test.appDataPath(t, customDir)
			t.Cleanup(func() { conf.Server.AppDataPath = before })
			srcDir := filepath.Join(root, "archive", "custom")
			writeFile(t, filepath.Join(srcDir, "attachments", "a"), "restored")
			writeFile(t, filepath.Join(srcDir, "conf", "app.ini"), "restored")

			require.NoError(t, restoreCustomDir(srcDir, customDir))

			// The app data restore replaces it, which fails on a leftover "attachments.bak".
			assert.Equal(t, "current", readFile(t, filepath.Join(customDir, "attachments", "a")))
			assert.NoDirExists(t, filepath.Join(customDir, "attachments.bak"))
			assert.Equal(t, "restored", readFile(t, filepath.Join(customDir, "conf", "app.ini")))
		})
	}

	t.Run("creates missing custom directory", func(t *testing.T) {
		root := t.TempDir()
		customDir := filepath.Join(root, "custom")
		srcDir := filepath.Join(root, "archive", "custom")
		writeFile(t, filepath.Join(srcDir, "conf", "app.ini"), "restored")

		require.NoError(t, restoreCustomDir(srcDir, customDir))

		assert.Equal(t, "restored", readFile(t, filepath.Join(customDir, "conf", "app.ini")))
	})

	t.Run("refuses leftover backup", func(t *testing.T) {
		root := t.TempDir()
		customDir := filepath.Join(root, "custom")
		writeFile(t, filepath.Join(customDir, "conf", "app.ini"), "current")
		writeFile(t, filepath.Join(customDir, "conf.bak", "app.ini"), "leftover")
		srcDir := filepath.Join(root, "archive", "custom")
		writeFile(t, filepath.Join(srcDir, "conf", "app.ini"), "restored")

		err := restoreCustomDir(srcDir, customDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
		assert.Equal(t, "current", readFile(t, filepath.Join(customDir, "conf", "app.ini")))
	})
}

func TestReplacePath(t *testing.T) {
	t.Run("moves current back when the move fails", func(t *testing.T) {
		root := t.TempDir()
		dstPath := filepath.Join(root, "conf")
		writeFile(t, filepath.Join(dstPath, "app.ini"), "current")

		err := replacePath(filepath.Join(root, "missing"), dstPath)
		require.Error(t, err)

		assert.Equal(t, "current", readFile(t, filepath.Join(dstPath, "app.ini")))
		assert.NoDirExists(t, dstPath+".bak")
	})
}
