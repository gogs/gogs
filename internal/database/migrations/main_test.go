package migrations

import (
	"flag"
	"os"
	"testing"

	"gorm.io/gorm/logger"
)

func TestMain(m *testing.M) {
	flag.Parse()

	level := logger.Silent
	if testing.Verbose() {
		level = logger.Info
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	logger.Default = logger.Default.LogMode(level)

	os.Exit(m.Run())
}
