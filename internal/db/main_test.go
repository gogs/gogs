// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"flag"
	"fmt"
	"io/ioutil"
	stdlog "log"
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

	var w logger.Writer
	level := logger.Silent
	if !testing.Verbose() {
		// Remove the primary logger and register a noop logger.
		log.Remove(log.DefaultConsoleName)
		err := log.New("noop", testutil.InitNoopLogger)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		w = &dbutil.Logger{Writer: ioutil.Discard}
	} else {
		w = stdlog.New(os.Stdout, "\r\n", stdlog.LstdFlags)
		level = logger.Info
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	logger.Default = logger.New(w, logger.Config{
		SlowThreshold: 100 * time.Millisecond,
		LogLevel:      level,
	})

	os.Exit(m.Run())
}

// clearTables removes all rows from given tables.
func clearTables(t *testing.T, db *gorm.DB, tables ...interface{}) error {
	if t.Failed() {
		return nil
	}

	for _, t := range tables {
		err := db.Where("TRUE").Delete(t).Error
		if err != nil {
			return err
		}
	}
	return nil
}

func initTestDB(t *testing.T, suite string, tables ...interface{}) *gorm.DB {
	t.Helper()

	dbpath := filepath.Join(os.TempDir(), fmt.Sprintf("gogs-%s-%d.db", suite, time.Now().Unix()))
	now := time.Now().UTC().Truncate(time.Second)
	db, err := openDB(
		conf.DatabaseOpts{
			Type: "sqlite3",
			Path: dbpath,
		},
		&gorm.Config{
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			NowFunc: func() time.Time {
				return now
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}

		if t.Failed() {
			t.Logf("Database %q left intact for inspection", dbpath)
			return
		}

		_ = os.Remove(dbpath)
	})

	err = db.Migrator().AutoMigrate(tables...)
	if err != nil {
		t.Fatal(err)
	}

	return db
}
