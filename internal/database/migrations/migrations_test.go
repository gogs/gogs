package migrations

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/dbtest"
)

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
		require.NoError(t, Migrate(db))

		tables, err := db.Migrator().GetTables()
		require.NoError(t, err)
		tables = slices.DeleteFunc(tables, func(name string) bool { return name == "sqlite_sequence" })
		assert.Len(t, tables, 37) // 36 tables and "goose_db_version"
		assert.Contains(t, tables, "user")
		assert.NotContains(t, tables, "version")
		assert.True(t, db.Migrator().HasIndex("attachment", "UQE_attachment_uuid"))
		assert.Equal(t, int64(23), gooseVersion(t, db))

		// Running again changes nothing.
		require.NoError(t, Migrate(db))
		assert.Equal(t, int64(23), gooseVersion(t, db))
	})

	t.Run("database migrated by the legacy runner from 0.12", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateLegacy")
		require.NoError(t, Migrate(db))

		// Turn the database into one of Gogs 0.12 that the legacy runner tracked.
		require.NoError(t, db.Migrator().DropTable("goose_db_version", "lfs_object"))
		require.NoError(t, db.Exec("ALTER TABLE repository DROP COLUMN is_unlisted").Error)
		require.NoError(t, db.Migrator().CreateTable(new(Version)))
		require.NoError(t, db.Create(&Version{ID: 1, Version: 19}).Error)
		require.NoError(t, db.Exec("INSERT INTO repository (id, owner_id, lower_name, name) VALUES (1, 1, 'example', 'example')").Error)

		require.NoError(t, Migrate(db))
		assert.Equal(t, int64(23), gooseVersion(t, db))
		assert.True(t, db.Migrator().HasColumn("repository", "is_unlisted"))
		assert.True(t, db.Migrator().HasTable("lfs_object"))

		var count int64
		require.NoError(t, db.Table("repository").Where("is_unlisted = ?", false).Count(&count).Error)
		assert.Equal(t, int64(1), count)
	})

	t.Run("database older than 0.12", func(t *testing.T) {
		db := dbtest.NewDB(t, "migrateTooOld")
		require.NoError(t, db.Migrator().CreateTable(new(Version)))
		require.NoError(t, db.Create(&Version{ID: 1, Version: 18}).Error)

		err := Migrate(db)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Gogs has stopped supporting auto-migration from your previously installed version")
		assert.False(t, db.Migrator().HasTable("user"))
	})
}
