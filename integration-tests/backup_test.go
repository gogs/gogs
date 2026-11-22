package integrationtests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database"
	"gogs.io/gogs/internal/dbx"
)

func TestBackupAndRestore(t *testing.T) {
	t.Parallel()

	owner := seedUser(t, CreateUserOptions{})
	repo := seedRepository(t, owner.ID, CreateRepositoryOptions{AutoInit: true})
	c := newClient(t)
	c.signIn(owner)
	index := issueCreate(t, c, repo, "Restore me", "As I was.")

	// Values that column defaults, create hooks, or the time of the restore
	// would replace if restoring did not insert them as they are.
	err := gormDB.Model(&database.Repository{ID: repo.ID}).UpdateColumns(map[string]any{
		"description":            "Backed up",
		"is_private":             true,
		"enable_wiki":            false,
		"enable_issues":          false,
		"enable_pulls":           false,
		"pulls_allow_merge":      false,
		"pulls_allow_squash":     true,
		"pulls_merge_message":    "Merged",
		"external_tracker_style": "",
		"created_unix":           1588568886,
		"updated_unix":           1588572486,
	}).Error
	require.NoError(t, err)
	err = gormDB.Model(new(database.Issue)).Where("repo_id = ? AND index = ?", repo.ID, index).
		UpdateColumns(map[string]any{"created_unix": 1588568886, "updated_unix": 1588572486}).Error
	require.NoError(t, err)

	dir := t.TempDir()
	gogs := func(args ...string) {
		t.Helper()
		out, err := exec.Command(os.Args[0], args...).CombinedOutput()
		require.NoError(t, err, "gogs %s: %s", strings.Join(args, " "), out)
	}
	gogs("backup", "--config", conf.CustomConf, "--database-only", "--tempdir", dir, "--target", dir, "--archive-name", "backup.zip")

	// Restore into a database of its own, so the server under test keeps its own.
	dbName, dropDatabase, err := createPostgresDatabase()
	require.NoError(t, err)
	t.Cleanup(func() { _ = dropDatabase() })
	config, err := os.ReadFile(conf.CustomConf)
	require.NoError(t, err)
	restoreConfig := filepath.Join(dir, "app.ini")
	nameLine := "NAME = " + conf.Database.Name + "\n"
	require.Contains(t, string(config), nameLine)
	err = os.WriteFile(restoreConfig, []byte(strings.Replace(string(config), nameLine, "NAME = "+dbName+"\n", 1)), 0o600)
	require.NoError(t, err)
	gogs("restore", "--config", restoreConfig, "--database-only", "--tempdir", dir, "--from", filepath.Join(dir, "backup.zip"))

	restoreOpts := conf.Database
	restoreOpts.Name = dbName
	restored, err := dbx.OpenDB(restoreOpts, &gorm.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		if sqlDB, err := restored.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	rows := func(db *gorm.DB, table, where string, args ...any) []string {
		t.Helper()
		var out []string
		err := db.Raw(`SELECT row_to_json(t)::text FROM "`+table+`" t WHERE `+where+` ORDER BY id`, args...).Scan(&out).Error
		require.NoError(t, err)
		return out
	}
	for _, tc := range []struct {
		table string
		where string
		arg   int64
	}{
		{"user", "id = ?", owner.ID},
		{"repository", "id = ?", repo.ID},
		{"issue", "repo_id = ?", repo.ID},
	} {
		want := rows(gormDB, tc.table, tc.where, tc.arg)
		require.NotEmpty(t, want, tc.table)
		assert.Equal(t, want, rows(restored, tc.table, tc.where, tc.arg), "rows of table %q", tc.table)
	}
}
