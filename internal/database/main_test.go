package database

import (
	"flag"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"unknwon.dev/x/logx"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbtest"
)

func TestMain(m *testing.M) {
	flag.Parse()

	level := logger.Silent
	if testing.Verbose() {
		level = logger.Info
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	logger.Default = logger.Default.LogMode(level)

	switch os.Getenv("GOGS_DATABASE_TYPE") {
	case "mysql":
		conf.UseMySQL = true
	case "postgres":
		conf.UsePostgreSQL = true
	default:
		conf.UseSQLite3 = true
	}

	// Tests run in parallel and code such as Markdown rendering reads conf.UI
	// without holding the mock lock, so tests set it here instead of mocking it.
	conf.UI = conf.UIOpts{
		IssuePagingNum: 2,
		User: conf.UIUserOpts{
			NewsFeedPagingNum: 20,
		},
	}

	os.Exit(m.Run())
}

// newTestDB returns a new test database with the schema created by migrations.
func newTestDB(t *testing.T, suite string) *gorm.DB {
	db := dbtest.NewDB(t, suite)
	err := migrations.Migrate(logx.NewNoopLogger(), db)
	require.NoError(t, err)
	return db
}

func clearTables(t *testing.T, db *gorm.DB) error {
	if t.Failed() {
		return nil
	}

	for _, t := range append(Tables, legacyTables...) {
		err := db.Where("TRUE").Delete(t).Error
		if err != nil {
			return err
		}
	}
	return nil
}
