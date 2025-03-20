// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	log "unknwon.dev/clog/v2"
	"xorm.io/core"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/database/migrations"
	"gogs.io/gogs/internal/dbutil"
)

// Engine represents a XORM engine or session.
type Engine interface {
	Delete(any) (int64, error)
	Exec(...any) (sql.Result, error)
	Find(any, ...any) error
	Get(any) (bool, error)
	ID(any) *xorm.Session
	In(string, ...any) *xorm.Session
	Insert(...any) (int64, error)
	InsertOne(any) (int64, error)
	Iterate(any, xorm.IterFunc) error
	Sql(string, ...any) *xorm.Session
	Table(any) *xorm.Session
	Where(any, ...any) *xorm.Session
}

var (
	x            *xorm.Engine
	legacyTables []any
	HasEngine    bool
)

func init() {
	legacyTables = append(legacyTables,
		new(User), new(PublicKey), new(TwoFactor), new(TwoFactorRecoveryCode),
		new(Repository), new(DeployKey), new(Collaboration), new(Upload),
		new(Watch), new(Star),
		new(Issue), new(PullRequest), new(Comment), new(Attachment), new(IssueUser),
		new(Label), new(IssueLabel), new(Milestone),
		new(Mirror), new(Release), new(Webhook), new(HookTask),
		new(ProtectBranch), new(ProtectBranchWhitelist),
		new(Team), new(OrgUser), new(TeamUser), new(TeamRepo),
	)

	gonicNames := []string{"SSL"}
	for _, name := range gonicNames {
		core.LintGonicMapper[name] = true
	}
}

func getEngine() (*xorm.Engine, error) {
	Param := "?"
	if strings.Contains(conf.Database.Name, Param) {
		Param = "&"
	}

	driver := conf.Database.Type
	connStr := ""
	switch conf.Database.Type {
	case "mysql":
		conf.UseMySQL = true
		if conf.Database.Host[0] == '/' { // looks like a unix socket
			connStr = fmt.Sprintf("%s:%s@unix(%s)/%s%scharset=utf8mb4&parseTime=true",
				conf.Database.User, conf.Database.Password, conf.Database.Host, conf.Database.Name, Param)
		} else {
			connStr = fmt.Sprintf("%s:%s@tcp(%s)/%s%scharset=utf8mb4&parseTime=true",
				conf.Database.User, conf.Database.Password, conf.Database.Host, conf.Database.Name, Param)
		}
		engineParams := map[string]string{"rowFormat": "DYNAMIC"}
		return xorm.NewEngineWithParams(conf.Database.Type, connStr, engineParams)

	case "postgres":
		conf.UsePostgreSQL = true
		host, port := dbutil.ParsePostgreSQLHostPort(conf.Database.Host)
		connStr = fmt.Sprintf("user='%s' password='%s' host='%s' port='%s' dbname='%s' sslmode='%s' search_path='%s'",
			conf.Database.User, conf.Database.Password, host, port, conf.Database.Name, conf.Database.SSLMode, conf.Database.Schema)
		driver = "pgx"

	case "mssql":
		conf.UseMSSQL = true
		host, port := dbutil.ParseMSSQLHostPort(conf.Database.Host)
		connStr = fmt.Sprintf("server=%s; port=%s; database=%s; user id=%s; password=%s;", host, port, conf.Database.Name, conf.Database.User, conf.Database.Password)

	case "sqlite3":
		if err := os.MkdirAll(path.Dir(conf.Database.Path), os.ModePerm); err != nil {
			return nil, fmt.Errorf("create directories: %v", err)
		}
		conf.UseSQLite3 = true
		connStr = "file:" + conf.Database.Path + "?cache=shared&mode=rwc"

	default:
		return nil, fmt.Errorf("unknown database type: %s", conf.Database.Type)
	}
	return xorm.NewEngine(driver, connStr)
}

func NewTestEngine() error {
	x, err := getEngine()
	if err != nil {
		return fmt.Errorf("connect to database: %v", err)
	}

	if conf.UsePostgreSQL {
		x.SetSchema(conf.Database.Schema)
	}

	x.SetMapper(core.GonicMapper{})
	return x.StoreEngine("InnoDB").Sync2(legacyTables...)
}

func SetEngine() (*gorm.DB, error) {
	var err error
	x, err = getEngine()
	if err != nil {
		return nil, fmt.Errorf("connect to database: %v", err)
	}

	if conf.UsePostgreSQL {
		x.SetSchema(conf.Database.Schema)
	}

	x.SetMapper(core.GonicMapper{})

	var logPath string
	if conf.HookMode {
		logPath = filepath.Join(conf.Log.RootPath, "hooks", "xorm.log")
	} else {
		logPath = filepath.Join(conf.Log.RootPath, "xorm.log")
	}
	sec := conf.File.Section("log.xorm")
	fileWriter, err := log.NewFileWriter(logPath,
		log.FileRotationConfig{
			Rotate:  sec.Key("ROTATE").MustBool(true),
			Daily:   sec.Key("ROTATE_DAILY").MustBool(true),
			MaxSize: sec.Key("MAX_SIZE").MustInt64(100) * 1024 * 1024,
			MaxDays: sec.Key("MAX_DAYS").MustInt64(3),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create 'xorm.log': %v", err)
	}

	x.SetMaxOpenConns(conf.Database.MaxOpenConns)
	x.SetMaxIdleConns(conf.Database.MaxIdleConns)
	x.SetConnMaxLifetime(time.Second)

	if conf.IsProdMode() {
		x.SetLogger(xorm.NewSimpleLogger3(fileWriter, xorm.DEFAULT_LOG_PREFIX, xorm.DEFAULT_LOG_FLAG, core.LOG_ERR))
	} else {
		x.SetLogger(xorm.NewSimpleLogger(fileWriter))
	}
	x.ShowSQL(true)

	var gormLogger logger.Writer
	if conf.HookMode {
		gormLogger = &dbutil.Logger{Writer: fileWriter}
	} else {
		gormLogger, err = newLogWriter()
		if err != nil {
			return nil, errors.Wrap(err, "new log writer")
		}
	}
	return NewConnection(gormLogger)
}

func NewEngine() error {
	db, err := SetEngine()
	if err != nil {
		return err
	}

	if err = migrations.Migrate(db); err != nil {
		return fmt.Errorf("migrate: %v", err)
	}

	if err = x.StoreEngine("InnoDB").Sync2(legacyTables...); err != nil {
		return errors.Wrap(err, "sync tables")
	}

	return nil
}

type Statistic struct {
	Counter struct {
		User, Org, PublicKey,
		Repo, Watch, Star, Action, Access,
		Issue, Comment, Oauth, Follow,
		Mirror, Release, LoginSource, Webhook,
		Milestone, Label, HookTask,
		Team, UpdateTask, Attachment int64
	}
}

func GetStatistic(ctx context.Context) (stats Statistic) {
	stats.Counter.User = Handle.Users().Count(ctx)
	stats.Counter.Org = CountOrganizations()
	stats.Counter.PublicKey, _ = x.Count(new(PublicKey))
	stats.Counter.Repo = CountRepositories(true)
	stats.Counter.Watch, _ = x.Count(new(Watch))
	stats.Counter.Star, _ = x.Count(new(Star))
	stats.Counter.Action, _ = x.Count(new(Action))
	stats.Counter.Access, _ = x.Count(new(Access))
	stats.Counter.Issue, _ = x.Count(new(Issue))
	stats.Counter.Comment, _ = x.Count(new(Comment))
	stats.Counter.Oauth = 0
	stats.Counter.Follow, _ = x.Count(new(Follow))
	stats.Counter.Mirror, _ = x.Count(new(Mirror))
	stats.Counter.Release, _ = x.Count(new(Release))
	stats.Counter.LoginSource = Handle.LoginSources().Count(ctx)
	stats.Counter.Webhook, _ = x.Count(new(Webhook))
	stats.Counter.Milestone, _ = x.Count(new(Milestone))
	stats.Counter.Label, _ = x.Count(new(Label))
	stats.Counter.HookTask, _ = x.Count(new(HookTask))
	stats.Counter.Team, _ = x.Count(new(Team))
	stats.Counter.Attachment, _ = x.Count(new(Attachment))
	return stats
}

func Ping() error {
	if x == nil {
		return errors.New("database not available")
	}
	return x.Ping()
}

// The version table. Should have only one row with id==1
type Version struct {
	ID      int64
	Version int64
}
