package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRestoreCustomKeepsDataDirectory(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "data"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	dbPath := filepath.Join(custom, "data", "gogs.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("original-db"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("old-config"), 0o644))

	before, err := os.Stat(dbPath)
	require.NoError(t, err)

	archiveCustom := filepath.Join(root, "archive", "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(archiveCustom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(archiveCustom, "conf", "app.ini"), []byte("archive-config"), 0o644))

	held, err := holdCustomData(custom)
	require.NoError(t, err)
	require.NoError(t, moveAside(custom))
	require.NoError(t, moveTree(archiveCustom, custom))
	require.NoError(t, restoreHeldCustomData(custom, held))

	got, err := os.ReadFile(filepath.Join(custom, "conf", "app.ini"))
	require.NoError(t, err)
	assert.Equal(t, "archive-config", string(got))

	after, err := os.Stat(filepath.Join(custom, "data", "gogs.db"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	assert.Equal(t, "original-db", mustRead(t, filepath.Join(custom, "data", "gogs.db")))
	_, err = os.Stat(custom + ".restore-data")
	assert.True(t, os.IsNotExist(err))
}

func TestRestoreCustomWithoutDataDirectory(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("old-config"), 0o644))

	archiveCustom := filepath.Join(root, "archive", "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(archiveCustom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(archiveCustom, "conf", "app.ini"), []byte("archive-config"), 0o644))

	held, err := holdCustomData(custom)
	require.NoError(t, err)
	assert.Empty(t, held)
	require.NoError(t, moveAside(custom))
	require.NoError(t, moveTree(archiveCustom, custom))
	require.NoError(t, restoreHeldCustomData(custom, held))

	assert.Equal(t, "archive-config", mustRead(t, filepath.Join(custom, "conf", "app.ini")))
	_, err = os.Stat(custom + ".restore-data")
	assert.True(t, os.IsNotExist(err))
}

func TestRestoreHoldRejectsLeftoverData(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "data"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "data", "gogs.db"), []byte("original-db"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("old-config"), 0o644))
	require.NoError(t, os.MkdirAll(custom+".restore-data", os.ModePerm))

	_, err := holdCustomData(custom)
	require.Error(t, err)
	assert.Contains(t, err.Error(), custom+".restore-data")
	assert.Contains(t, err.Error(), "already exists")
	assert.Equal(t, "old-config", mustRead(t, filepath.Join(custom, "conf", "app.ini")))
	assert.Equal(t, "original-db", mustRead(t, filepath.Join(custom, "data", "gogs.db")))
	info, err := os.Stat(custom + ".restore-data")
	require.NoError(t, err)
	assert.True(t, info.IsDir())
}

func TestRestoreHeldDataWhenArchiveHasData(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "data"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "data", "gogs.db"), []byte("original-db"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("old-config"), 0o644))

	archiveCustom := filepath.Join(root, "archive", "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(archiveCustom, "data"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(archiveCustom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(archiveCustom, "data", "gogs.db"), []byte("archive-db"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(archiveCustom, "conf", "app.ini"), []byte("archive-config"), 0o644))

	held, err := holdCustomData(custom)
	require.NoError(t, err)
	require.NoError(t, moveAside(custom))
	require.NoError(t, moveTree(archiveCustom, custom))

	err = restoreHeldCustomData(custom, held)
	require.EqualError(t, err, "Restored custom directory already contains 'data', so held data remains at '"+held+"'.")
	assert.Equal(t, "original-db", mustRead(t, filepath.Join(held, "gogs.db")))
}

func TestRestoreMoveTreeSameFilesystem(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested"), os.ModePerm))
	srcFile := filepath.Join(src, "nested", "file.txt")
	require.NoError(t, os.WriteFile(srcFile, []byte("moved"), 0o644))
	before, err := os.Stat(srcFile)
	require.NoError(t, err)

	dst := filepath.Join(root, "dst")
	require.NoError(t, moveTree(src, dst))

	_, err = os.Stat(src)
	assert.True(t, os.IsNotExist(err))
	after, err := os.Stat(filepath.Join(dst, "nested", "file.txt"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	assert.Equal(t, "moved", mustRead(t, filepath.Join(dst, "nested", "file.txt")))
}

func TestRestoreCrossDeviceCopy(t *testing.T) {
	linkErr := &os.LinkError{Op: "rename", Old: "src", New: "dst", Err: syscall.EXDEV}
	assert.True(t, isCrossDevice(linkErr))

	root := t.TempDir()
	src := filepath.Join(root, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(src, "nested"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(src, "nested", "file.txt"), []byte("copied"), 0o644))

	dst := filepath.Join(root, "dst")
	require.NoError(t, copyTree(src, dst))
	assert.Equal(t, "copied", mustRead(t, filepath.Join(dst, "nested", "file.txt")))
}

func TestRestoreRollbackAfterFailedInstall(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom")
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "data"), os.ModePerm))
	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	dbPath := filepath.Join(custom, "data", "gogs.db")
	require.NoError(t, os.WriteFile(dbPath, []byte("original-db"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("original-config"), 0o644))
	before, err := os.Stat(dbPath)
	require.NoError(t, err)

	held, err := holdCustomData(custom)
	require.NoError(t, err)
	require.NoError(t, moveAside(custom))

	require.NoError(t, os.MkdirAll(filepath.Join(custom, "conf"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(custom, "conf", "app.ini"), []byte("partial-config"), 0o644))

	require.NoError(t, rollbackCustomInstall(custom, held))

	assert.Equal(t, "original-config", mustRead(t, filepath.Join(custom, "conf", "app.ini")))
	after, err := os.Stat(filepath.Join(custom, "data", "gogs.db"))
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after))
	_, err = os.Stat(custom + ".bak")
	assert.True(t, os.IsNotExist(err))
	_, err = os.Stat(custom + ".restore-data")
	assert.True(t, os.IsNotExist(err))
}

func TestRestoreRollbackDataDirectory(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "attachments")
	require.NoError(t, os.MkdirAll(dirPath, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("original"), 0o644))

	require.NoError(t, moveAside(dirPath))
	require.NoError(t, os.MkdirAll(dirPath, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("partial"), 0o644))

	require.NoError(t, rollbackDataInstall(dirPath))

	assert.Equal(t, "original", mustRead(t, filepath.Join(dirPath, "file.txt")))
	_, err := os.Stat(dirPath + ".bak")
	assert.True(t, os.IsNotExist(err))
}

func TestRestoreDataDirectoryRestoredAfterFailedCopy(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "attachments")
	require.NoError(t, os.MkdirAll(dirPath, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("original"), 0o644))
	require.NoError(t, moveAside(dirPath))

	require.NoError(t, os.MkdirAll(filepath.Join(dirPath, "nested"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "nested", "file.txt"), []byte("partial"), 0o644))

	copyErr := errors.New("copy failed")
	err := removePartialDestination(dirPath, copyErr)
	require.ErrorIs(t, err, copyErr)
	_, statErr := os.Stat(dirPath)
	assert.True(t, os.IsNotExist(statErr))

	require.NoError(t, rollbackDataInstall(dirPath))
	assert.Equal(t, "original", mustRead(t, filepath.Join(dirPath, "file.txt")))
	_, statErr = os.Stat(dirPath + ".bak")
	assert.True(t, os.IsNotExist(statErr))
}

func TestRestoreRollbackDataDirectoryWithoutBackup(t *testing.T) {
	root := t.TempDir()
	dirPath := filepath.Join(root, "attachments")
	require.NoError(t, os.MkdirAll(dirPath, os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dirPath, "file.txt"), []byte("live"), 0o644))

	require.NoError(t, rollbackDataInstall(dirPath))

	assert.Equal(t, "live", mustRead(t, filepath.Join(dirPath, "file.txt")))
}

func TestRestoreRemovePartialDestination(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "partial")
	require.NoError(t, os.MkdirAll(filepath.Join(dst, "nested"), os.ModePerm))
	require.NoError(t, os.WriteFile(filepath.Join(dst, "nested", "file.txt"), []byte("partial"), 0o644))

	copyErr := errors.New("copy failed")
	err := removePartialDestination(dst, copyErr)
	require.ErrorIs(t, err, copyErr)
	assert.Equal(t, "copy failed", err.Error())
	_, statErr := os.Stat(dst)
	assert.True(t, os.IsNotExist(statErr))
}

func TestRestoreRemovePartialDestinationReportsFailure(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "not-a-directory")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o644))
	dst := filepath.Join(blocker, "partial")

	copyErr := errors.New("copy failed")
	err := removePartialDestination(dst, copyErr)
	require.Error(t, err)
	assert.ErrorIs(t, err, copyErr)
	assert.Contains(t, err.Error(), dst)
	assert.Contains(t, err.Error(), "copy failed")
	_, statErr := os.Stat(blocker)
	require.NoError(t, statErr)
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(got)
}
