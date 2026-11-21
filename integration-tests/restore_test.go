package integrationtests

import (
	"archive/zip"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), os.ModePerm))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	p, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(p)
}

// TestBackupRestoreDockerLayout backs up and restores an instance laid out like
// the Docker image, where the app data directory is "data" in the custom
// directory and holds the SQLite database the restore writes to.
func TestBackupRestoreDockerLayout(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	workDir := filepath.Join(root, "app", "gogs")
	customDir := filepath.Join(root, "data", "gogs")
	dataDir := filepath.Join(customDir, "data")
	require.NoError(t, os.MkdirAll(workDir, os.ModePerm))
	require.NoError(t, os.MkdirAll(dataDir, os.ModePerm))
	require.NoError(t, os.Symlink(dataDir, filepath.Join(workDir, "data")))

	u, err := user.Current()
	require.NoError(t, err)
	port, err := freePort()
	require.NoError(t, err)
	writeTestFile(t, filepath.Join(customDir, "conf", "app.ini"), fmt.Sprintf(`RUN_USER = %s

[server]
HTTP_ADDR = 127.0.0.1
HTTP_PORT = %d
DISABLE_SSH = true

[database]
TYPE = sqlite3
PATH = data/gogs.db

[repository]
ROOT = %s

[security]
SECRET_KEY = restore-test

[log]
MODE = file
ROOT_PATH = %s
`, u.Username, port, filepath.Join(root, "repositories"), filepath.Join(customDir, "log")))

	command := func(args ...string) *exec.Cmd {
		cmd := exec.Command(os.Args[0], args...)
		cmd.Dir = root
		// Later values override the ones TestMain sets.
		cmd.Env = append(os.Environ(), "GOGS_WORK_DIR="+workDir, "GOGS_CUSTOM="+customDir)
		return cmd
	}
	gogs := func(args ...string) (string, error) {
		out, err := command(args...).CombinedOutput()
		return string(out), err
	}

	// Only the web server creates the database schema, as on the first start of a
	// new instance.
	server := command("web")
	// The server writes to the file itself, so reading it while the server runs is safe.
	serverLogPath := filepath.Join(root, "server.log")
	serverLogFile, err := os.Create(serverLogPath)
	require.NoError(t, err)
	defer func() { _ = serverLogFile.Close() }()
	serverLog := func() string {
		p, _ := os.ReadFile(serverLogPath)
		return string(p)
	}
	server.Stdout = serverLogFile
	server.Stderr = serverLogFile
	require.NoError(t, server.Start())
	// Kills the server when the test stops before the server exits.
	t.Cleanup(func() { _ = server.Process.Kill() })
	exited := make(chan error, 1)
	go func() { exited <- server.Wait() }()
	require.NoError(t, waitForServer(fmt.Sprintf("http://127.0.0.1:%d", port), exited), "server log: %s", serverLog())
	require.NoError(t, server.Process.Signal(os.Interrupt))
	select {
	case err := <-exited:
		require.NoError(t, err, "server log: %s", serverLog())
	case <-time.After(10 * time.Second):
		t.Fatalf("server did not stop within 10 seconds, log: %s", serverLog())
	}
	createUser := func(name string) {
		t.Helper()
		out, err := gogs("admin", "create-user", "--name", name, "--password", "password", "--email", name+"@example.com")
		require.NoError(t, err, "output: %s", out)
	}
	restore := func() (string, error) {
		return gogs("restore", "--tempdir", root, "--from", filepath.Join(root, "backup.zip"))
	}
	dbPath := filepath.Join(dataDir, "gogs.db")
	userNames := func() []string {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		defer func() { _ = sqlDB.Close() }()
		var names []string
		require.NoError(t, db.Table("user").Order("lower_name").Pluck("lower_name", &names).Error)
		return names
	}

	createUser("alice")
	attachmentPath := filepath.Join(workDir, "data", "attachments", "a", "b", "attachment")
	writeTestFile(t, attachmentPath, "backed up")
	out, err := gogs("backup", "--tempdir", root, "--target", root, "--archive-name", "backup.zip", "--exclude-repos")
	require.NoError(t, err, "output: %s", out)

	createUser("bob")
	writeTestFile(t, attachmentPath, "changed")
	writeTestFile(t, filepath.Join(customDir, "conf", "app.ini.local"), "not backed up")
	writeTestFile(t, filepath.Join(customDir, "public", "robots.txt"), "not backed up")
	dbBefore, err := os.Stat(dbPath)
	require.NoError(t, err)

	out, err = restore()
	require.NoError(t, err, "output: %s", out)

	dbAfter, err := os.Stat(dbPath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(dbBefore, dbAfter), "the database file must be restored in place")
	assert.Equal(t, []string{"alice"}, userNames())
	assert.NoDirExists(t, customDir+".bak")
	assert.NoDirExists(t, filepath.Join(customDir, "data.bak"))

	assert.Equal(t, "backed up", readTestFile(t, attachmentPath))
	assert.Equal(t, "changed", readTestFile(t, filepath.Join(dataDir, "attachments.bak", "a", "b", "attachment")))
	assert.NoFileExists(t, filepath.Join(customDir, "conf", "app.ini.local"))
	assert.Equal(t, "not backed up", readTestFile(t, filepath.Join(customDir, "conf.bak", "app.ini.local")))
	assert.Equal(t, "not backed up", readTestFile(t, filepath.Join(customDir, "public", "robots.txt")))

	t.Run("next backup leaves out what the restore kept", func(t *testing.T) {
		out, err := gogs("backup", "--tempdir", root, "--target", root, "--archive-name", "backup-after-restore.zip", "--exclude-repos")
		require.NoError(t, err, "output: %s", out)

		r, err := zip.OpenReader(filepath.Join(root, "backup-after-restore.zip"))
		require.NoError(t, err)
		defer func() { _ = r.Close() }()
		var names []string
		for _, f := range r.File {
			names = append(names, f.Name)
		}
		assert.Contains(t, names, "gogs-backup/custom/conf/app.ini")
		for _, name := range names {
			assert.NotContains(t, name, ".bak", "archive entry %q", name)
		}
	})

	t.Run("leftover backup stops the restore before it replaces any file", func(t *testing.T) {
		// Only the backup of a path that the restore replaces after the custom files is left behind.
		require.NoError(t, os.RemoveAll(filepath.Join(customDir, "conf.bak")))
		require.NoError(t, os.RemoveAll(filepath.Join(customDir, "log.bak")))
		require.DirExists(t, filepath.Join(dataDir, "attachments.bak"))
		createUser("carol")
		writeTestFile(t, filepath.Join(customDir, "conf", "app.ini.local"), "current")

		out, err := restore()
		require.Error(t, err)
		assert.Contains(t, out, "attachments.bak' already exists")

		assert.Equal(t, []string{"alice", "carol"}, userNames())
		assert.Equal(t, "current", readTestFile(t, filepath.Join(customDir, "conf", "app.ini.local")))
		assert.NoDirExists(t, filepath.Join(customDir, "conf.bak"))
		assert.NoDirExists(t, filepath.Join(customDir, "log.bak"))
	})
}
