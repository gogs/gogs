package database

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/testx"
)

func TestMain(m *testing.M) {
	flag.Parse()

	level := logger.Silent
	if !testing.Verbose() {
		// Remove the primary logger and register a noop logger.
		log.Remove(log.DefaultConsoleName)
		err := log.New("noop", testx.InitNoopLogger)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
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

// newTestDB returns a new test database with the schema created by migrations.
func newTestDB(t *testing.T, suite string) *gorm.DB {
	db := dbtest.NewDB(t, suite)
	err := migrations.Migrate(db)
	require.NoError(t, err)
	return db
}

func clearTables(t *testing.T, db *gorm.DB) error {
	if t.Failed() {
		return nil
	}

	for _, t := range Tables {
		err := db.Where("TRUE").Delete(t).Error
		if err != nil {
			return err
		}
	}
	return nil
}
