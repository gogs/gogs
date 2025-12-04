// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"flag"
	"fmt"
	"os"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	_ "modernc.org/sqlite"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbtest"
	"gogs.io/gogs/internal/testutil"
)

func TestMain(m *testing.M) {
	flag.Parse()

	level := logger.Silent
	if !testing.Verbose() {
		// Remove the primary logger and register a noop logger.
		log.Remove(log.DefaultConsoleName)
		err := log.New("noop", testutil.InitNoopLogger)
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
