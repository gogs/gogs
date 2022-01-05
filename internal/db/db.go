// Copyright 2020 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/driver/sqlserver"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
)

// parsePostgreSQLHostPort parses given input in various forms defined in
// https://www.postgresql.org/docs/current/static/libpq-connect.html#LIBPQ-CONNSTRING
// and returns proper host and port number.
func parsePostgreSQLHostPort(info string) (host, port string) {
	host, port = "127.0.0.1", "5432"
	if strings.Contains(info, ":") && !strings.HasSuffix(info, "]") {
		idx := strings.LastIndex(info, ":")
		host = info[:idx]
		port = info[idx+1:]
	} else if len(info) > 0 {
		host = info
	}
	return host, port
}

func parseMSSQLHostPort(info string) (host, port string) {
	host, port = "127.0.0.1", "1433"
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
		dsn = fmt.Sprintf("user='%s' password='%s' host='%s' port='%s' dbname='%s' sslmode='%s' search_path='%s'",
			opts.User, opts.Password, host, port, opts.Name, opts.SSLMode, opts.Schema)

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

func newLogWriter() (logger.Writer, error) {
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
	return &dbutil.Logger{Writer: w}, nil
}

func openDB(opts conf.DatabaseOpts, cfg *gorm.Config) (*gorm.DB, error) {
	dsn, err := parseDSN(opts)
	if err != nil {
		return nil, errors.Wrap(err, "parse DSN")
	}

	var dialector gorm.Dialector
	switch opts.Type {
	case "mysql":
		dialector = mysql.Open(dsn)
	case "postgres":
		dialector = postgres.Open(dsn)
	case "mssql":
		dialector = sqlserver.Open(dsn)
	case "sqlite3":
		dialector = sqlite.Open(dsn)
	default:
		panic("unreachable")
	}

	return gorm.Open(dialector, cfg)
}

// Tables is the list of struct-to-table mappings.
//
// NOTE: Lines are sorted in alphabetical order, each letter in its own line.
var Tables = []interface{}{
	new(Access), new(AccessToken),
	new(LFSObject), new(LoginSource),
}

func Init(w logger.Writer) (*gorm.DB, error) {
	level := logger.Info
	if conf.IsProdMode() {
		level = logger.Warn
	}

	// NOTE: AutoMigrate does not respect logger passed in gorm.Config.
	logger.Default = logger.New(w, logger.Config{
		SlowThreshold: 100 * time.Millisecond,
		LogLevel:      level,
	})

	db, err := openDB(conf.Database, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		NowFunc: func() time.Time {
			return time.Now().UTC().Truncate(time.Microsecond)
		},
	})
	if err != nil {
		return nil, errors.Wrap(err, "open database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, errors.Wrap(err, "get underlying *sql.DB")
	}
	sqlDB.SetMaxOpenConns(conf.Database.MaxOpenConns)
	sqlDB.SetMaxIdleConns(conf.Database.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Minute)

	switch conf.Database.Type {
	case "postgres":
		conf.UsePostgreSQL = true
	case "mysql":
		conf.UseMySQL = true
		db = db.Set("gorm:table_options", "ENGINE=InnoDB").Session(&gorm.Session{})
	case "sqlite3":
		conf.UseSQLite3 = true
	case "mssql":
		conf.UseMSSQL = true
	default:
		panic("unreachable")
	}

	// NOTE: GORM has problem detecting existing columns, see https://github.com/gogs/gogs/issues/6091.
	// Therefore only use it to create new tables, and do customized migration with future changes.
	for _, table := range Tables {
		if db.Migrator().HasTable(table) {
			continue
		}

		name := strings.TrimPrefix(fmt.Sprintf("%T", table), "*db.")
		err = db.Migrator().AutoMigrate(table)
		if err != nil {
			return nil, errors.Wrapf(err, "auto migrate %q", name)
		}
		log.Trace("Auto migrated %q", name)
	}

	sourceFiles, err := loadLoginSourceFiles(filepath.Join(conf.CustomDir(), "conf", "auth.d"), db.NowFunc)
	if err != nil {
		return nil, errors.Wrap(err, "load login source files")
	}

	// Initialize stores, sorted in alphabetical order.
	AccessTokens = &accessTokens{DB: db}
	LoginSources = &loginSources{DB: db, files: sourceFiles}
	LFS = &lfs{DB: db}
	Perms = &perms{DB: db}
	Repos = &repos{DB: db}
	TwoFactors = &twoFactors{DB: db}
	Users = &users{DB: db}

	return db, nil
}
