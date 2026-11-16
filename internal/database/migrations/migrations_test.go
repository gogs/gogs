package migrations

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/dbtest"
)

// latestVersion returns the version of the last migration, which every
// migrated database is on.
func latestVersion(db *gorm.DB) int64 {
	migrations := newMigrations(logx.NewNoopLogger(), db)
	return migrations[len(migrations)-1].Version
}

func gooseVersion(t *testing.T, db *gorm.DB) int64 {
	var version int64
	err := db.Raw("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied").Scan(&version).Error
	require.NoError(t, err)
	return version
}

func TestMigrate(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	t.Parallel()

	t.Run("new database", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateNew")
		require.NoError(t, Migrate(logx.NewNoopLogger(), db))

		tables, err := db.Migrator().GetTables()
		require.NoError(t, err)
		tables = slices.DeleteFunc(tables, func(name string) bool { return name == "sqlite_sequence" })
		assert.Len(t, tables, 37) // 36 tables and "goose_db_version"
		assert.Contains(t, tables, "user")
		assert.Contains(t, tables, "passkey")
		assert.NotContains(t, tables, "version")
		assert.NotContains(t, tables, "protect_branch_whitelist")
		assert.True(t, db.Migrator().HasIndex("attachment", "UQE_attachment_uuid"))
		assert.Equal(t, latestVersion(db), gooseVersion(t, db))

		// Running again changes nothing.
		require.NoError(t, Migrate(logx.NewNoopLogger(), db))
		assert.Equal(t, latestVersion(db), gooseVersion(t, db))
	})

	t.Run("database migrated by the legacy runner from 0.12", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateLegacy")
		require.NoError(t, Migrate(logx.NewNoopLogger(), db))

		// Turn the database into one of Gogs 0.12 that the legacy runner tracked.
		require.NoError(t, db.Migrator().DropTable("goose_db_version", "lfs_object", "passkey"))
		for _, column := range []string{"is_unlisted", "pulls_allow_merge", "pulls_allow_squash", "pulls_delete_branch", "pulls_merge_message", "pulls_squash_message"} {
			require.NoError(t, db.Exec("ALTER TABLE repository DROP COLUMN "+column).Error)
		}
		require.NoError(t, db.Exec("ALTER TABLE attachment DROP COLUMN uploader_id").Error)
		require.NoError(t, db.Migrator().CreateTable(new(Version)))
		require.NoError(t, db.Create(&Version{ID: 1, Version: 19}).Error)
		require.NoError(t, db.Exec("INSERT INTO repository (id, owner_id, lower_name, name) VALUES (1, 1, 'example', 'example')").Error)

		require.NoError(t, Migrate(logx.NewNoopLogger(), db))
		assert.Equal(t, latestVersion(db), gooseVersion(t, db))
		assert.True(t, db.Migrator().HasColumn("repository", "is_unlisted"))
		assert.True(t, db.Migrator().HasTable("lfs_object"))
		assert.True(t, db.Migrator().HasTable("passkey"))
		assert.True(t, db.Migrator().HasColumn("attachment", "uploader_id"))

		// Existing repositories keep allowing merge commits, do not allow squashing,
		// keep head branches, and have no message presets.
		var count int64
		require.NoError(t, db.Table("repository").
			Where("is_unlisted = ? AND pulls_allow_merge = ? AND pulls_allow_squash = ? AND pulls_delete_branch = ? AND pulls_merge_message = '' AND pulls_squash_message = ''", false, true, false, false).
			Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("MySQL database with another default character set", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateCharset")
		if db.Name() != "mysql" {
			t.Skip("MySQL only")
		}
		require.NoError(t, db.Exec("ALTER DATABASE `"+db.Migrator().CurrentDatabase()+"` CHARACTER SET latin1 COLLATE latin1_swedish_ci").Error)
		require.NoError(t, Migrate(logx.NewNoopLogger(), db))

		// Tables that XORM used to create stay utf8mb4, as XORM created them.
		require.NoError(t, db.Exec("INSERT INTO issue (repo_id, `index`, name) VALUES (1, 1, ?)", "Emoji \U0001F600").Error)
	})

	t.Run("pool of one connection", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateOneConn")
		sqlDB, err := db.DB()
		require.NoError(t, err)
		sqlDB.SetMaxOpenConns(1)

		done := make(chan error, 1)
		go func() { done <- Migrate(logx.NewNoopLogger(), db) }()
		select {
		case err := <-done:
			require.Error(t, err)
			assert.Contains(t, err.Error(), "MAX_OPEN_CONNS must be at least 2")
		case <-time.After(time.Minute):
			t.Fatal("Migrate did not return with a pool of one connection")
		}
		assert.False(t, db.Migrator().HasTable("user"))
	})

	t.Run("database older than 0.12", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateTooOld")
		require.NoError(t, db.Migrator().CreateTable(new(Version)))
		require.NoError(t, db.Create(&Version{ID: 1, Version: 18}).Error)

		err := Migrate(logx.NewNoopLogger(), db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "auto-migration from the installed version is no longer supported")
		assert.False(t, db.Migrator().HasTable("user"))
	})
}
