// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"database/sql"
	"fmt"
	"os"
	"path"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"

	"github.com/gogits/gogs/models/migrations"
	"github.com/gogits/gogs/modules/setting"
)

// Engine represents a xorm engine or session.
type Engine interface {
	Delete(interface{}) (int64, error)
	Exec(string, ...interface{}) (sql.Result, error)
	Insert(...interface{}) (int64, error)
}

var (
	x         *xorm.Engine
	tables    []interface{}
	HasEngine bool

	DbCfg struct {
		Type, Host, Name, User, Pwd, Path, SslMode string
	}

	EnableSQLite3 bool
	UseSQLite3    bool
)

func init() {
	tables = append(tables,
		new(User), new(PublicKey), new(Follow), new(Oauth2), new(AccessToken),
		new(Repository), new(Watch), new(Star), new(Action), new(Access),
		new(Issue), new(Comment), new(Attachment), new(IssueUser), new(Label), new(Milestone),
		new(Mirror), new(Release), new(LoginSource), new(Webhook),
		new(UpdateTask), new(HookTask), new(Team), new(OrgUser), new(TeamUser),
		new(Notice), new(EmailAddress))
}

func LoadModelsConfig() {
	sec := setting.Cfg.Section("database")
	DbCfg.Type = sec.Key("DB_TYPE").String()
	if DbCfg.Type == "sqlite3" {
		UseSQLite3 = true
	}
	DbCfg.Host = sec.Key("HOST").String()
	DbCfg.Name = sec.Key("NAME").String()
	DbCfg.User = sec.Key("USER").String()
	if len(DbCfg.Pwd) == 0 {
		DbCfg.Pwd = sec.Key("PASSWD").String()
	}
	DbCfg.SslMode = sec.Key("SSL_MODE").String()
	DbCfg.Path = sec.Key("PATH").MustString("data/gogs.db")
}

func getEngine() (*xorm.Engine, error) {
	cnnstr := ""
	switch DbCfg.Type {
	case "mysql":
		cnnstr = fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8",
			DbCfg.User, DbCfg.Pwd, DbCfg.Host, DbCfg.Name)
	case "postgres":
		var host, port = "127.0.0.1", "5432"
		fields := strings.Split(DbCfg.Host, ":")
		if len(fields) > 0 && len(strings.TrimSpace(fields[0])) > 0 {
			host = fields[0]
		}
		if len(fields) > 1 && len(strings.TrimSpace(fields[1])) > 0 {
			port = fields[1]
		}
		cnnstr = fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s sslmode=%s",
			DbCfg.User, DbCfg.Pwd, host, port, DbCfg.Name, DbCfg.SslMode)
	case "sqlite3":
		if !EnableSQLite3 {
			return nil, fmt.Errorf("Unknown database type: %s", DbCfg.Type)
		}
		os.MkdirAll(path.Dir(DbCfg.Path), os.ModePerm)
		cnnstr = "file:" + DbCfg.Path + "?cache=shared&mode=rwc"
	default:
		return nil, fmt.Errorf("Unknown database type: %s", DbCfg.Type)
	}
	return xorm.NewEngine(DbCfg.Type, cnnstr)
}

func NewTestEngine(x *xorm.Engine) (err error) {
	x, err = getEngine()
	if err != nil {
		return fmt.Errorf("models.init(fail to connect to database): %v", err)
	}

	return x.Sync(tables...)
}

func SetEngine() (err error) {
	x, err = getEngine()
	if err != nil {
		return fmt.Errorf("models.init(fail to connect to database): %v", err)
	}

	// WARNING: for serv command, MUST remove the output to os.stdout,
	// so use log file to instead print to stdout.
	logPath := path.Join(setting.LogRootPath, "xorm.log")
	os.MkdirAll(path.Dir(logPath), os.ModePerm)

	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("models.init(fail to create xorm.log): %v", err)
	}
	x.Logger = xorm.NewSimpleLogger(f)

	x.ShowSQL = true
	x.ShowInfo = true
	x.ShowDebug = true
	x.ShowErr = true
	x.ShowWarn = true
	return nil
}

func NewEngine() (err error) {
	if err = SetEngine(); err != nil {
		return err
	}

	if err = migrations.Migrate(x); err != nil {
		return err
	}

	if err = x.StoreEngine("InnoDB").Sync2(tables...); err != nil {
		return fmt.Errorf("sync database struct error: %v\n", err)
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
	stats.Counter.Repo = CountRepositories()
	stats.Counter.Watch, _ = x.Count(new(Watch))
	stats.Counter.Star, _ = x.Count(new(Star))
	stats.Counter.Action, _ = x.Count(new(Action))
	stats.Counter.Access, _ = x.Count(new(Access))
	stats.Counter.Issue, _ = x.Count(new(Issue))
	stats.Counter.Comment, _ = x.Count(new(Comment))
	stats.Counter.Oauth, _ = x.Count(new(Oauth2))
	stats.Counter.Follow, _ = x.Count(new(Follow))
	stats.Counter.Mirror, _ = x.Count(new(Mirror))
	stats.Counter.Release, _ = x.Count(new(Release))
	stats.Counter.LoginSource, _ = x.Count(new(LoginSource))
	stats.Counter.Webhook, _ = x.Count(new(Webhook))
	stats.Counter.Milestone, _ = x.Count(new(Milestone))
	stats.Counter.Label, _ = x.Count(new(Label))
	stats.Counter.HookTask, _ = x.Count(new(HookTask))
	stats.Counter.Team, _ = x.Count(new(Team))
	stats.Counter.UpdateTask, _ = x.Count(new(UpdateTask))
	stats.Counter.Attachment, _ = x.Count(new(Attachment))
	return
}

func Ping() error {
	return x.Ping()
}

// DumpDatabase dumps all data from database to file system.
func DumpDatabase(filePath string) error {
	return x.DumpAllToFile(filePath)
}
