// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	"github.com/jinzhu/gorm/dialects/mssql"
	"github.com/jinzhu/gorm/dialects/mysql"
	"github.com/jinzhu/gorm/dialects/postgres"
	"github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/jinzhu/gorm/logger"
	"github.com/jinzhu/gorm/schema"
	"github.com/pkg/errors"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
)

// parsePostgreSQLHostPort parses given input in various forms defined in
// https://www.postgresql.org/docs/current/static/libpq-connect.html#LIBPQ-CONNSTRING
// and returns proper host and port number.
func parsePostgreSQLHostPort(info string) (string, string) {
	host, port := "127.0.0.1", "5432"
	if strings.Contains(info, ":") && !strings.HasSuffix(info, "]") {
		idx := strings.LastIndex(info, ":")
		host = info[:idx]
		port = info[idx+1:]
	} else if len(info) > 0 {
		host = info
	}
	return host, port
}

func parseMSSQLHostPort(info string) (string, string) {
	host, port := "127.0.0.1", "1433"
	if strings.Contains(info, ":") {
		host = strings.Split(info, ":")[0]
		port = strings.Split(info, ":")[1]
	} else if strings.Contains(info, ",") {
		host = strings.Split(info, ",")[0]
		port = strings.TrimSpace(strings.Split(info, ",")[1])
	} else if len(info) > 0 {
		host = info
	}
	return host, port
}

func parseOpts(opts conf.DatabaseOpts) (gorm.Dialector, error) {
	// In case the database name contains "?" with some parameters
	concate := "?"
	if strings.Contains(opts.Name, concate) {
		concate = "&"
	}

	var d gorm.Dialector
	switch opts.Type {
	case "mysql":
		var dsn string
		if opts.Host[0] == '/' { // Looks like a unix socket
			dsn = fmt.Sprintf("%s:%s@unix(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		}
		d = mysql.Open(dsn)

	case "postgres":
		var dsn string
		host, port := parsePostgreSQLHostPort(opts.Host)
		if host[0] == '/' { // looks like a unix socket
			dsn = fmt.Sprintf("postgres://%s:%s@:%s/%s%ssslmode=%s&host=%s",
				url.QueryEscape(opts.User), url.QueryEscape(opts.Password), port, opts.Name, concate, opts.SSLMode, host)
		} else {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s%ssslmode=%s",
				url.QueryEscape(opts.User), url.QueryEscape(opts.Password), host, port, opts.Name, concate, opts.SSLMode)
		}
		d = postgres.Open(dsn)

	case "mssql":
		host, port := parseMSSQLHostPort(opts.Host)
		dsn := fmt.Sprintf("server=%s; port=%s; database=%s; user id=%s; password=%s;",
			host, port, opts.Name, opts.User, opts.Passwd)
		d = mssql.Open(dsn)

	case "sqlite3":
		dsn := "file:" + opts.Path + "?cache=shared&mode=rwc"
		d = sqlite.Open(dsn)

	default:
		return nil, errors.Errorf("unrecognized dialect: %s", opts.Type)
	}

	return d, nil
}

func openDB(opts conf.DatabaseOpts, logger logger.Interface, now func() time.Time) (*gorm.DB, error) {
	dialector, err := parseOpts(opts)
	if err != nil {
		return nil, errors.Wrap(err, "parse options")
	}

	return gorm.Open(dialector, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger:  logger,
		NowFunc: now,
	})
}

func Init(ctx context.Context) error {
	sec := conf.File.Section("log.gorm")
	w, err := log.NewFileWriter(
		filepath.Join(conf.Log.RootPath, "gorm.log"),
		log.FileRotationConfig{
			Rotate:  sec.Key("ROTATE").MustBool(true),
			Daily:   sec.Key("ROTATE_DAILY").MustBool(true),
			MaxSize: sec.Key("MAX_SIZE").MustInt64(100) * 1024 * 1024,
			MaxDays: sec.Key("MAX_DAYS").MustInt64(3),
		},
	)
	if err != nil {
		return errors.Wrap(err, `create "gorm.log"`)
	}

	logLevel := logger.Info
	if conf.IsProdMode() {
		logLevel = logger.Error
	}

	l := logger.New(
		&dbutil.Writer{
			Writer: w,
		},
		logger.Config{
			SlowThreshold: 100 * time.Millisecond,
			LogLevel:      logLevel,
		},
	)

	db, err := openDB(conf.Database, l, func() time.Time { return time.Now().UTC() })
	if err != nil {
		return errors.Wrap(err, "open database")
	}
	db = db.WithContext(ctx)
	db.ConnPool.(*sql.DB).SetMaxOpenConns(conf.Database.MaxOpenConns)
	db.ConnPool.(*sql.DB).SetMaxIdleConns(conf.Database.MaxIdleConns)
	db.ConnPool.(*sql.DB).SetConnMaxLifetime(time.Second)

	switch conf.Database.Type {
	case "mysql":
		conf.UseMySQL = true
		db = db.Set("gorm:table_options", "ENGINE=InnoDB")
	case "postgres":
		conf.UsePostgreSQL = true
	case "mssql":
		conf.UseMSSQL = true
	case "sqlite3":
		conf.UseMySQL = true
	}

	err = db.AutoMigrate(new(LFSObject))
	if err != nil {
		return errors.Wrap(err, "migrate schemes")
	}

	// Initialize stores
	LFS = &lfs{DB: db}
	return nil
}
