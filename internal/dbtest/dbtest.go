// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package dbtest

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
)

// NewDB creates a new test database and initializes the given list of tables
// for the suite. The test database is dropped after testing is completed unless
// failed.
func NewDB(t *testing.T, suite string, tables ...any) *gorm.DB {
	dbType := os.Getenv("GOGS_DATABASE_TYPE")

	var dbName string
	var dbOpts conf.DatabaseOpts
	var cleanup func(db *gorm.DB)
	switch dbType {
	case "mysql":
		dbOpts = conf.DatabaseOpts{
			Type:     "mysql",
			Host:     os.ExpandEnv("$MYSQL_HOST:$MYSQL_PORT"),
			Name:     dbName,
			User:     os.Getenv("MYSQL_USER"),
			Password: os.Getenv("MYSQL_PASSWORD"),
		}

		dsn, err := dbutil.NewDSN(dbOpts)
		require.NoError(t, err)

		sqlDB, err := sql.Open("mysql", dsn)
		require.NoError(t, err)

		// Set up test database
		dbName = fmt.Sprintf("gogs-%s-%d", suite, time.Now().Unix())
		_, err = sqlDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName))
		require.NoError(t, err)

		_, err = sqlDB.Exec(fmt.Sprintf("CREATE DATABASE `%s`", dbName))
		require.NoError(t, err)

		dbOpts.Name = dbName

		cleanup = func(db *gorm.DB) {
			testDB, err := db.DB()
			if err == nil {
				_ = testDB.Close()
			}

			_, _ = sqlDB.Exec(fmt.Sprintf("DROP DATABASE `%s`", dbName))
			_ = sqlDB.Close()
		}
	case "postgres":
		dbOpts = conf.DatabaseOpts{
			Type:     "postgres",
			Host:     os.ExpandEnv("$PGHOST:$PGPORT"),
			Name:     dbName,
			Schema:   "public",
			User:     os.Getenv("PGUSER"),
			Password: os.Getenv("PGPASSWORD"),
			SSLMode:  os.Getenv("PGSSLMODE"),
		}

		dsn, err := dbutil.NewDSN(dbOpts)
		require.NoError(t, err)

		sqlDB, err := sql.Open("pgx", dsn)
		require.NoError(t, err)

		// Set up test database
		dbName = fmt.Sprintf("gogs-%s-%d", suite, time.Now().Unix())
		_, err = sqlDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %q", dbName))
		require.NoError(t, err)

		_, err = sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %q", dbName))
		require.NoError(t, err)

		dbOpts.Name = dbName

		cleanup = func(db *gorm.DB) {
			testDB, err := db.DB()
			if err == nil {
				_ = testDB.Close()
			}

			_, _ = sqlDB.Exec(fmt.Sprintf(`DROP DATABASE %q`, dbName))
			_ = sqlDB.Close()
		}
	case "sqlite":
		dbName = filepath.Join(os.TempDir(), fmt.Sprintf("gogs-%s-%d.db", suite, time.Now().Unix()))
		dbOpts = conf.DatabaseOpts{
			Type: "sqlite",
			Path: dbName,
		}
		cleanup = func(db *gorm.DB) {
			sqlDB, err := db.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
			_ = os.Remove(dbName)
		}
	default:
		dbName = filepath.Join(os.TempDir(), fmt.Sprintf("gogs-%s-%d.db", suite, time.Now().Unix()))
		dbOpts = conf.DatabaseOpts{
			Type: "sqlite3",
			Path: dbName,
		}
		cleanup = func(db *gorm.DB) {
			sqlDB, err := db.DB()
			if err == nil {
				_ = sqlDB.Close()
			}
			_ = os.Remove(dbName)
		}
	}

	now := time.Now().UTC().Truncate(time.Second)
	db, err := dbutil.OpenDB(
		dbOpts,
		&gorm.Config{
			SkipDefaultTransaction: true,
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			NowFunc: func() time.Time {
				return now
			},
		},
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("Database %q left intact for inspection", dbName)
			return
		}

		cleanup(db)
	})

	err = db.Migrator().AutoMigrate(tables...)
	require.NoError(t, err)

	return db
}
