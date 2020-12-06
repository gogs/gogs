// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
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

	os.Exit(m.Run())
}

// TODO: Once finished migrating to GORM, we can just use Tables instead of a
// 	subset `tables` for setting up test database.
func newTestDB(t *testing.T, suite string, tables ...interface{}) (*gorm.DB, func() error) {
	dbpath := filepath.Join(os.TempDir(), fmt.Sprintf("gogs-%s-%d.db", suite, time.Now().Unix()))
	now := time.Now().Local().Truncate(time.Second)
	db, err := openDB(
		conf.DatabaseOpts{
			Type: "sqlite3",
			Path: dbpath,
		},
		&gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			NowFunc: func() time.Time { return now },
		},
	)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}

	err = db.Migrator().AutoMigrate(tables...)
	if err != nil {
		t.Fatalf("Failed to auto migrate tables: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}

		if t.Failed() {
			t.Logf("DATABASE %q left intact for inspection", dbpath)
			return
		}

		_ = os.Remove(dbpath)
	})

	return db, func() error {
		if t.Failed() {
			return nil
		}

		m := db.Migrator().(interface {
			RunWithValue(value interface{}, fc func(*gorm.Statement) error) error
		})
		for _, t := range tables {
			err := m.RunWithValue(t, func(stmt *gorm.Statement) error {
				return db.Exec(`DELETE FROM ` + dbutil.QuoteIdentifier(stmt.Table)).Error
			})
			if err != nil {
				return err
			}
		}
		return nil
	}
}
