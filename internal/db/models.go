// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package db

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"
	"xorm.io/core"
	"xorm.io/xorm"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/db/migrations"
)

// Engine represents a XORM engine or session.
type Engine interface {
	Delete(interface{}) (int64, error)
	Exec(...interface{}) (sql.Result, error)
	Find(interface{}, ...interface{}) error
	Get(interface{}) (bool, error)
	ID(interface{}) *xorm.Session
	In(string, ...interface{}) *xorm.Session
	Insert(...interface{}) (int64, error)
	InsertOne(interface{}) (int64, error)
	Iterate(interface{}, xorm.IterFunc) error
	Sql(string, ...interface{}) *xorm.Session
	Table(interface{}) *xorm.Session
	Where(interface{}, ...interface{}) *xorm.Session
}

var (
	x            *xorm.Engine
	legacyTables []interface{}
	HasEngine    bool
)

func init() {
	legacyTables = append(legacyTables,
		new(User), new(PublicKey), new(TwoFactor), new(TwoFactorRecoveryCode),
		new(Repository), new(DeployKey), new(Collaboration), new(Access), new(Upload),
		new(Watch), new(Star), new(Follow), new(Action),
		new(Issue), new(PullRequest), new(Comment), new(Attachment), new(IssueUser),
		new(Label), new(IssueLabel), new(Milestone),
		new(Mirror), new(Release), new(Webhook), new(HookTask),
		new(ProtectBranch), new(ProtectBranchWhitelist),
		new(Team), new(OrgUser), new(TeamUser), new(TeamRepo),
		new(Notice), new(EmailAddress))

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
		var engineParams = map[string]string{"rowFormat": "DYNAMIC"}
		return xorm.NewEngineWithParams(conf.Database.Type, connStr, engineParams)

	case "postgres":
		conf.UsePostgreSQL = true
		host, port := parsePostgreSQLHostPort(conf.Database.Host)
		if host[0] == '/' { // looks like a unix socket
			connStr = fmt.Sprintf("postgres://%s:%s@:%s/%s%ssslmode=%s&host=%s",
				url.QueryEscape(conf.Database.User), url.QueryEscape(conf.Database.Password), port, conf.Database.Name, Param, conf.Database.SSLMode, host)
		} else {
			connStr = fmt.Sprintf("postgres://%s:%s@%s:%s/%s%ssslmode=%s",
				url.QueryEscape(conf.Database.User), url.QueryEscape(conf.Database.Password), host, port, conf.Database.Name, Param, conf.Database.SSLMode)
		}
		driver = "pgx"

	case "mssql":
		conf.UseMSSQL = true
		host, port := parseMSSQLHostPort(conf.Database.Host)
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

	x.SetMapper(core.GonicMapper{})
	return x.StoreEngine("InnoDB").Sync2(legacyTables...)
}

func SetEngine() (*gorm.DB, error) {
	var err error
	x, err = getEngine()
	if err != nil {
		return nil, fmt.Errorf("connect to database: %v", err)
	}

	x.SetMapper(core.GonicMapper{})

	// WARNING: for serv command, MUST remove the output to os.stdout,
	// so use log file to instead print to stdout.
	sec := conf.File.Section("log.xorm")
	logger, err := log.NewFileWriter(path.Join(conf.Log.RootPath, "xorm.log"),
		log.FileRotationConfig{
			Rotate:  sec.Key("ROTATE").MustBool(true),
			Daily:   sec.Key("ROTATE_DAILY").MustBool(true),
			MaxSize: sec.Key("MAX_SIZE").MustInt64(100) * 1024 * 1024,
			MaxDays: sec.Key("MAX_DAYS").MustInt64(3),
		})
	if err != nil {
		return nil, fmt.Errorf("create 'xorm.log': %v", err)
	}

	x.SetMaxOpenConns(conf.Database.MaxOpenConns)
	x.SetMaxIdleConns(conf.Database.MaxIdleConns)
	x.SetConnMaxLifetime(time.Second)

	if conf.IsProdMode() {
		x.SetLogger(xorm.NewSimpleLogger3(logger, xorm.DEFAULT_LOG_PREFIX, xorm.DEFAULT_LOG_FLAG, core.LOG_WARNING))
	} else {
		x.SetLogger(xorm.NewSimpleLogger(logger))
	}
	x.ShowSQL(true)
	return Init()
}

func NewEngine() (err error) {
	if _, err = SetEngine(); err != nil {
		return err
	}

	if err = migrations.Migrate(x); err != nil {
		return fmt.Errorf("migrate: %v", err)
	}

	if err = x.StoreEngine("InnoDB").Sync2(legacyTables...); err != nil {
		return fmt.Errorf("sync structs to database tables: %v\n", err)
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

func GetStatistic() (stats Statistic) {
	stats.Counter.User = CountUsers()
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
	stats.Counter.LoginSource = LoginSources.Count()
	stats.Counter.Webhook, _ = x.Count(new(Webhook))
	stats.Counter.Milestone, _ = x.Count(new(Milestone))
	stats.Counter.Label, _ = x.Count(new(Label))
	stats.Counter.HookTask, _ = x.Count(new(HookTask))
	stats.Counter.Team, _ = x.Count(new(Team))
	stats.Counter.Attachment, _ = x.Count(new(Attachment))
	return
}

func Ping() error {
	return x.Ping()
}

// The version table. Should have only one row with id==1
type Version struct {
	ID      int64
	Version int64
}
