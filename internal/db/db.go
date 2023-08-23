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
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/dbutil"
)

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

// Tables is the list of struct-to-table mappings.
//
// NOTE: Lines are sorted in alphabetical order, each letter in its own line.
var Tables = []any{
	new(Access), new(AccessToken), new(Action),
	new(EmailAddress),
	new(Follow),
	new(LFSObject), new(LoginSource),
	new(Notice),
}

// Init initializes the database with given logger.
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

	db, err := dbutil.OpenDB(
		conf.Database,
		&gorm.Config{
			SkipDefaultTransaction: true,
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true,
			},
			NowFunc: func() time.Time {
				return time.Now().UTC().Truncate(time.Microsecond)
			},
		},
	)
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

	// NOTE: GORM has problem detecting existing columns, see
	// https://github.com/gogs/gogs/issues/6091. Therefore only use it to create new
	// tables, and do customized migration with future changes.
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
	Actions = NewActionsStore(db)
	LoginSources = &loginSources{DB: db, files: sourceFiles}
	LFS = &lfs{DB: db}
	Notices = NewNoticesStore(db)
	Orgs = NewOrgsStore(db)
	Perms = NewPermsStore(db)
	Repos = NewReposStore(db)
	TwoFactors = &twoFactors{DB: db}
	Users = NewUsersStore(db)

	return db, nil
}
