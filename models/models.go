// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"os"
	"path"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/go-xorm/xorm"
	_ "github.com/lib/pq"

	"github.com/gogits/gogs/modules/base"
)

var (
	orm    *xorm.Engine
	tables []interface{}

	HasEngine bool

	DbCfg struct {
		Type, Host, Name, User, Pwd, Path, SslMode string
	}

	EnableSQLite3 bool
	UseSQLite3    bool
)

func init() {
	tables = append(tables, new(User), new(PublicKey), new(Repository), new(Watch),
		new(Action), new(Access), new(Issue), new(Comment), new(Oauth2), new(Follow),
		new(Mirror), new(Release))
}

func LoadModelsConfig() {
	DbCfg.Type = base.Cfg.MustValue("database", "DB_TYPE")
	if DbCfg.Type == "sqlite3" {
		UseSQLite3 = true
	}
	DbCfg.Host = base.Cfg.MustValue("database", "HOST")
	DbCfg.Name = base.Cfg.MustValue("database", "NAME")
	DbCfg.User = base.Cfg.MustValue("database", "USER")
	DbCfg.Pwd = base.Cfg.MustValue("database", "PASSWD")
	DbCfg.SslMode = base.Cfg.MustValue("database", "SSL_MODE")
	DbCfg.Path = base.Cfg.MustValue("database", "PATH", "data/gogs.db")
}

func NewTestEngine(x *xorm.Engine) (err error) {
	switch DbCfg.Type {
	case "mysql":
		x, err = xorm.NewEngine("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8",
			DbCfg.User, DbCfg.Pwd, DbCfg.Host, DbCfg.Name))
	case "postgres":
		var host, port = "127.0.0.1", "5432"
		fields := strings.Split(DbCfg.Host, ":")
		if len(fields) > 0 {
			host = fields[0]
		}
		if len(fields) > 1 {
			port = fields[1]
		}
		cnnstr := fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s sslmode=%s",
			DbCfg.User, DbCfg.Pwd, host, port, DbCfg.Name, DbCfg.SslMode)
		//fmt.Println(cnnstr)
		x, err = xorm.NewEngine("postgres", cnnstr)
	case "sqlite3":
		if !EnableSQLite3 {
			return fmt.Errorf("Unknown database type: %s", DbCfg.Type)
		}
		os.MkdirAll(path.Dir(DbCfg.Path), os.ModePerm)
		x, err = xorm.NewEngine("sqlite3", DbCfg.Path)
	default:
		return fmt.Errorf("Unknown database type: %s", DbCfg.Type)
	}
	if err != nil {
		return fmt.Errorf("models.init(fail to conntect database): %v", err)
	}
	return x.Sync(tables...)
}

func SetEngine() (err error) {
	switch DbCfg.Type {
	case "mysql":
		orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%s:%s@tcp(%s)/%s?charset=utf8",
			DbCfg.User, DbCfg.Pwd, DbCfg.Host, DbCfg.Name))
	case "postgres":
		var host, port = "127.0.0.1", "5432"
		fields := strings.Split(DbCfg.Host, ":")
		if len(fields) > 0 {
			host = fields[0]
		}
		if len(fields) > 1 {
			port = fields[1]
		}
		orm, err = xorm.NewEngine("postgres", fmt.Sprintf("user=%s password=%s host=%s port=%s dbname=%s sslmode=%s",
			DbCfg.User, DbCfg.Pwd, host, port, DbCfg.Name, DbCfg.SslMode))
	case "sqlite3":
		os.MkdirAll(path.Dir(DbCfg.Path), os.ModePerm)
		orm, err = xorm.NewEngine("sqlite3", DbCfg.Path)
	default:
		return fmt.Errorf("Unknown database type: %s", DbCfg.Type)
	}
	if err != nil {
		return fmt.Errorf("models.init(fail to conntect database): %v", err)
	}

	// WARNNING: for serv command, MUST remove the output to os.stdout,
	// so use log file to instead print to stdout.
	execDir, _ := base.ExecDir()
	logPath := execDir + "/log/xorm.log"
	os.MkdirAll(path.Dir(logPath), os.ModePerm)

	f, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("models.init(fail to create xorm.log): %v", err)
	}
	orm.Logger = xorm.NewSimpleLogger(f)

	orm.ShowSQL = true
	orm.ShowDebug = true
	orm.ShowErr = true
	return nil
}

func NewEngine() (err error) {
	if err = SetEngine(); err != nil {
		return err
	}
	if err = orm.Sync(tables...); err != nil {
		return fmt.Errorf("sync database struct error: %v\n", err)
	}
	return nil
}

type Statistic struct {
	Counter struct {
		User, PublicKey, Repo,
		Watch, Action, Access,
		Issue, Comment,
		Mirror, Oauth, Release int64
	}
}

func GetStatistic() (stats Statistic) {
	stats.Counter.User, _ = orm.Count(new(User))
	stats.Counter.PublicKey, _ = orm.Count(new(PublicKey))
	stats.Counter.Repo, _ = orm.Count(new(Repository))
	stats.Counter.Watch, _ = orm.Count(new(Watch))
	stats.Counter.Action, _ = orm.Count(new(Action))
	stats.Counter.Access, _ = orm.Count(new(Access))
	stats.Counter.Issue, _ = orm.Count(new(Issue))
	stats.Counter.Comment, _ = orm.Count(new(Comment))
	stats.Counter.Mirror, _ = orm.Count(new(Mirror))
	stats.Counter.Oauth, _ = orm.Count(new(Oauth2))
	stats.Counter.Release, _ = orm.Count(new(Release))
	return
}
