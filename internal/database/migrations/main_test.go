package migrations

import (
	"flag"
	"os"
	"testing"

	"gorm.io/gorm/logger"

	"gogs.io/gogs/internal/applog"
	"gogs.io/gogs/internal/logx"
)

func TestMain(m *testing.M) {
	flag.Parse()

	level := logger.Silent
	if !testing.Verbose() {
		applog.Logger = logx.NewNoop()
	} else {
		level = logger.Info
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	logger.Default = logger.Default.LogMode(level)

	os.Exit(m.Run())
}
