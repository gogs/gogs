package database

import (
	"flag"
	"os"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"gogs.io/gogs/internal/conf"
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

	os.Exit(m.Run())
}

func newTestDB(t *testing.T, suite string) *gorm.DB {
	return dbtest.NewDB(t, suite, append(Tables, legacyTables...)...)
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
