// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/mssql"
	_ "github.com/jinzhu/gorm/dialects/mysql"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
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

// parseDSN takes given database options and returns parsed DSN.
func parseDSN(opts conf.DatabaseOpts) (dsn string, err error) {
	// In case the database name contains "?" with some parameters
	concate := "?"
	if strings.Contains(opts.Name, concate) {
		concate = "&"
	}

	switch opts.Type {
	case "mysql":
		if opts.Host[0] == '/' { // Looks like a unix socket
			dsn = fmt.Sprintf("%s:%s@unix(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		} else {
			dsn = fmt.Sprintf("%s:%s@tcp(%s)/%s%scharset=utf8mb4&parseTime=true",
				opts.User, opts.Password, opts.Host, opts.Name, concate)
		}

	case "postgres":
		host, port := parsePostgreSQLHostPort(opts.Host)
		if host[0] == '/' { // looks like a unix socket
			dsn = fmt.Sprintf("postgres://%s:%s@:%s/%s%ssslmode=%s&host=%s",
				url.QueryEscape(opts.User), url.QueryEscape(opts.Password), port, opts.Name, concate, opts.SSLMode, host)
		} else {
			dsn = fmt.Sprintf("postgres://%s:%s@%s:%s/%s%ssslmode=%s",
				url.QueryEscape(opts.User), url.QueryEscape(opts.Password), host, port, opts.Name, concate, opts.SSLMode)
		}

	case "mssql":
		host, port := parseMSSQLHostPort(opts.Host)
		dsn = fmt.Sprintf("server=%s; port=%s; database=%s; user id=%s; password=%s;",
			host, port, opts.Name, opts.User, opts.Password)

	case "sqlite3":
		dsn = "file:" + opts.Path + "?cache=shared&mode=rwc"

	default:
		return "", errors.Errorf("unrecognized dialect: %s", opts.Type)
	}

	return dsn, nil
}

func openDB(opts conf.DatabaseOpts) (*gorm.DB, error) {
	dsn, err := parseDSN(opts)
	if err != nil {
		return nil, errors.Wrap(err, "parse DSN")
	}

	return gorm.Open(opts.Type, dsn)
}

func getLogWriter() (io.Writer, error) {
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
		return nil, errors.Wrap(err, `create "gorm.log"`)
	}
	return w, nil
}

func Init() error {
	db, err := openDB(conf.Database)
	if err != nil {
		return errors.Wrap(err, "open database")
	}
	db.SingularTable(true)
	db.DB().SetMaxOpenConns(conf.Database.MaxOpenConns)
	db.DB().SetMaxIdleConns(conf.Database.MaxIdleConns)
	db.DB().SetConnMaxLifetime(time.Minute)

	w, err := getLogWriter()
	if err != nil {
		return errors.Wrap(err, "get log writer")
	}
	db.SetLogger(&dbutil.Writer{Writer: w})
	if !conf.IsProdMode() {
		db = db.LogMode(true)
	}

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

	err = db.AutoMigrate(new(LFSObject)).Error
	if err != nil {
		return errors.Wrap(err, "migrate schemes")
	}

	// Initialize stores, sorted in alphabetical order.
	AccessTokens = &accessTokens{DB: db}
	LoginSources = &loginSources{DB: db}
	LFS = &lfs{DB: db}
	Perms = &perms{DB: db}
	Repos = &repos{DB: db}
	TwoFactors = &twoFactors{DB: db}
	Users = &users{DB: db}

	return db.DB().Ping()
}
