package integrationtests

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
)

// TestBackupRestore backs up the server under test with `gogs backup` and
// restores the archive into a new instance with `gogs restore`.
//
// It does not run in parallel because the backup reads every repository, which
// other tests change at the same time.
func TestBackupRestore(t *testing.T) {
	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	repoPath := database.RepoPath(repo.Owner, repo.Name)
	wikiPath := database.WikiPath(repo.Owner, repo.Name)
	gitRun(t, "", "clone", "--bare", repoPath, wikiPath)
	// Gogs runs the custom hooks of a repository only when they are executable.
	hook := filepath.Join(repoPath, "custom_hooks", "pre-receive")
	require.NoError(t, os.MkdirAll(filepath.Dir(hook), 0o755))
	require.NoError(t, os.WriteFile(hook, []byte("#!/bin/sh\n"), 0o755))
	// Git allows names that are not valid UTF-8 in refs, which are files and
	// directories in the repository.
	nonUTF8Ref := "refs/heads/caf\xe9/x"
	gitRun(t, repoPath, "update-ref", nonUTF8Ref, "HEAD")
	dbOwner, err := database.Handle.Users().GetByID(t.Context(), owner.ID)
	require.NoError(t, err)
	// Directories of repositories and their owners are named in lower case,
	// unlike repositories and users.
	upperOwner, err := database.Handle.Users().Create(t.Context(), UniqueName("User-"), UniqueName("backup-")+"@example.com", database.CreateUserOptions{Activated: true})
	require.NoError(t, err)
	upper, err := database.Handle.Repositories().Create(t.Context(), logger, upperOwner, upperOwner, database.CreateRepoOptionsLegacy{
		Name:     UniqueName("Backup-"),
		AutoInit: true,
		Readme:   "Default",
	})
	require.NoError(t, err)
	upperPath := database.RepoPath(upperOwner.Name, upper.Name)
	mirror := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	require.NoError(t, gormDB.Model(&database.Repository{}).Where("id = ?", mirror.ID).Update("is_mirror", true).Error)
	t.Cleanup(func() {
		_ = gormDB.Model(&database.Repository{}).Where("id = ?", mirror.ID).Update("is_mirror", false).Error
	})

	attachments := filepath.Join(conf.Server.AppDataPath, "attachments")
	require.NoError(t, os.MkdirAll(attachments, 0o755))
	attachment := filepath.Join(attachments, UniqueName("backup-"))
	require.NoError(t, os.WriteFile(attachment, []byte("attachment"), 0o644))
	t.Cleanup(func() { _ = os.Remove(attachment) })

	// The backup has its own custom directory, so files planted in it do not
	// change the server under test.
	work := t.TempDir()
	customDir := filepath.Join(work, "custom")
	require.NoError(t, os.Mkdir(customDir, 0o755))
	// Links in the custom directory, e.g., to files kept elsewhere, are backed
	// up as what they link to.
	public := filepath.Join(work, "public")
	require.NoError(t, os.Mkdir(public, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(public, "robots.txt"), []byte("User-agent: *"), 0o644))
	require.NoError(t, os.Symlink(public, filepath.Join(customDir, "public")))
	// Nothing may be written to the system temporary directory, which on some
	// servers is too small for the archive. A file in its place makes any write
	// fail, even for root.
	systemTempDir := filepath.Join(work, "system-temp")
	require.NoError(t, os.WriteFile(systemTempDir, nil, 0o644))
	tempDir := filepath.Join(work, "temp")
	require.NoError(t, os.Mkdir(tempDir, 0o755))
	// The backup creates the target directory when it does not exist.
	target := filepath.Join(work, "backups")

	// runBackup runs `gogs backup` with the custom directory and returns its
	// output.
	runBackup := func(customDir, name string, flags ...string) ([]byte, error) {
		// TestMain points the first argument to the binary of the server.
		cmd := exec.Command(os.Args[0], append([]string{"backup", "--config", serverConfigPath, "--tempdir", tempDir, "--target", target, "--archive-name", name}, flags...)...)
		cmd.Env = append(os.Environ(), "GOGS_CUSTOM="+customDir, "TMPDIR="+systemTempDir)
		return cmd.CombinedOutput()
	}
	backup := func(t *testing.T, name string, flags ...string) string {
		t.Helper()

		out, err := runBackup(customDir, name, flags...)
		require.NoError(t, err, "backup: %s", out)
		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "backup left files in the temporary directory")
		archivePath := filepath.Join(target, name)
		// The archive has the password hashes and tokens of every user.
		fi, err := os.Stat(archivePath)
		require.NoError(t, err)
		assert.Equal(t, os.FileMode(0o600), fi.Mode().Perm())
		return archivePath
	}
	// repoFile is the name in "repositories.zip" of the file at the path.
	repoFile := func(path string) string {
		rel, err := filepath.Rel(filepath.Dir(conf.Repository.Root), path)
		require.NoError(t, err)
		return filepath.ToSlash(rel)
	}

	t.Run("database only", func(t *testing.T) {
		archive, err := zip.OpenReader(backup(t, "database-only.zip", "--database-only"))
		require.NoError(t, err)
		defer func() { _ = archive.Close() }()
		var names []string
		for _, f := range archive.File {
			if f.Name != "gogs-backup/metadata.ini" && !strings.HasPrefix(f.Name, "gogs-backup/db/") {
				names = append(names, f.Name)
			}
		}
		assert.Empty(t, names, "files other than the database")
		_, err = archive.Open("gogs-backup/db/User.json")
		assert.NoError(t, err)
	})

	t.Run("failure", func(t *testing.T) {
		// A link to nothing fails the backup after it has dumped the database.
		customDir := filepath.Join(t.TempDir(), "custom")
		require.NoError(t, os.Mkdir(customDir, 0o755))
		require.NoError(t, os.Symlink(filepath.Join(customDir, "missing"), filepath.Join(customDir, "broken")))
		out, err := runBackup(customDir, "failure.zip")
		require.Error(t, err, "backup: %s", out)

		entries, err := os.ReadDir(tempDir)
		require.NoError(t, err)
		assert.Empty(t, entries, "backup left the database dump in the temporary directory")
		assert.NoFileExists(t, filepath.Join(target, "failure.zip"), "backup left a partial archive with the database")
	})

	t.Run("exclude mirror repositories", func(t *testing.T) {
		archive, err := zip.OpenReader(backup(t, "exclude-mirrors.zip", "--exclude-mirror-repos"))
		require.NoError(t, err)
		defer func() { _ = archive.Close() }()
		f, err := archive.Open("gogs-backup/repositories.zip")
		require.NoError(t, err)
		defer func() { _ = f.Close() }()
		content, err := io.ReadAll(f)
		require.NoError(t, err)
		repos, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		require.NoError(t, err)
		files := make(map[string]bool, len(repos.File))
		for _, f := range repos.File {
			files[f.Name] = true
		}

		for _, path := range []string{repoPath, wikiPath, upperPath} {
			assert.True(t, files[repoFile(filepath.Join(path, "HEAD"))], "repository %q", path)
		}
		assert.False(t, files[repoFile(filepath.Join(database.RepoPath(mirror.Owner, mirror.Name), "HEAD"))], "mirror")
	})

	// An archive of an earlier backup with the same name, e.g., of a version
	// that made it readable by everyone, is overwritten.
	require.NoError(t, os.MkdirAll(target, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(target, "full.zip"), []byte("earlier"), 0o644))
	archivePath := backup(t, "full.zip")

	// The new instance has its own database, work directory and repositories.
	dbName, dropDatabase, err := createPostgresDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = dropDatabase() })
	restoreDir := t.TempDir()
	restoreConfig := filepath.Join(restoreDir, "app.ini")
	require.NoError(t, writeConfig(restoreDir, restoreConfig, "http://localhost/", 0, 0, dbName, conf.Security.SecretKey, false, conf.AntiBotVerificationAltcha))
	restoreTempDir := filepath.Join(restoreDir, "temp")
	require.NoError(t, os.Mkdir(restoreTempDir, 0o755))
	cmd := exec.Command(os.Args[0], "restore", "--config", restoreConfig, "--tempdir", restoreTempDir, "--from", archivePath)
	cmd.Env = append(os.Environ(), "GOGS_WORK_DIR="+restoreDir, "GOGS_CUSTOM="+filepath.Join(restoreDir, "custom"))
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "restore: %s", out)

	dsn, err := postgresDSN(dbName)
	require.NoError(t, err)
	restoredDB, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	defer func() { _ = restoredDB.Close() }()
	var passwd string
	require.NoError(t, restoredDB.QueryRow(`SELECT passwd FROM "user" WHERE id = $1`, owner.ID).Scan(&passwd))
	assert.Equal(t, dbOwner.Password, passwd)

	// restoredPath returns the path in the new instance of the path in the
	// work directory of the server under test.
	restoredPath := func(path string) string {
		rel, err := filepath.Rel(filepath.Dir(conf.Repository.Root), path)
		require.NoError(t, err)
		return filepath.Join(restoreDir, rel)
	}
	for _, path := range []string{repoPath, wikiPath, upperPath, database.RepoPath(mirror.Owner, mirror.Name)} {
		restored := restoredPath(path)
		assert.Equal(t, gitRun(t, path, "rev-parse", "HEAD"), gitRun(t, restored, "rev-parse", "HEAD"), "repository %q", restored)
		gitRun(t, restored, "fsck", "--strict")
	}
	assert.Equal(t, gitRun(t, repoPath, "rev-parse", nonUTF8Ref), gitRun(t, restoredPath(repoPath), "rev-parse", nonUTF8Ref))
	fi, err := os.Stat(restoredPath(hook))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), fi.Mode().Perm())

	content, err := os.ReadFile(filepath.Join(restoreDir, "custom", "public", "robots.txt"))
	require.NoError(t, err)
	assert.Equal(t, "User-agent: *", string(content))
	content, err = os.ReadFile(restoredPath(attachment))
	require.NoError(t, err)
	assert.Equal(t, "attachment", string(content))
}
