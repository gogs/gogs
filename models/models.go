// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"os"
	"path"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/lunny/xorm"

	"github.com/gogits/gogs/modules/base"
)

var (
	orm *xorm.Engine

	dbCfg struct {
		Type, Host, Name, User, Pwd, Path, SslMode string
	}
)

func LoadModelsConfig() {
	dbCfg.Type = base.Cfg.MustValue("database", "DB_TYPE")
	dbCfg.Host = base.Cfg.MustValue("database", "HOST")
	dbCfg.Name = base.Cfg.MustValue("database", "NAME")
	dbCfg.User = base.Cfg.MustValue("database", "USER")
	dbCfg.Pwd = base.Cfg.MustValue("database", "PASSWD")
	dbCfg.Path = base.Cfg.MustValue("database", "PATH", "data/gogs.db")
	dbCfg.SslMode = base.Cfg.MustValue("database", "SSL_MODE")
}

func setEngine() {

	var err error
	switch dbCfg.Type {
	case "mysql":
		orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%s:%s@%s/%s?charset=utf8",
			dbCfg.User, dbCfg.Pwd, dbCfg.Host, dbCfg.Name))
	case "postgres":
		orm, err = xorm.NewEngine("postgres", fmt.Sprintf("user=%s password=%s dbname=%s sslmode=%s",
			dbCfg.User, dbCfg.Pwd, dbCfg.Name, dbCfg.SslMode))
	case "sqlite3":
		os.MkdirAll(path.Dir(dbCfg.Path), os.ModePerm)
		orm, err = xorm.NewEngine("sqlite3", dbCfg.Path)
	default:
		fmt.Printf("Unknown database type: %s\n", dbCfg.Type)
		os.Exit(2)
	}
	if err != nil {
		fmt.Printf("models.init(fail to conntect database): %v\n", err)
		os.Exit(2)
	}

	// WARNNING: for serv command, MUST remove the output to os.stdout,
	// so use log file to instead print to stdout.

	//x.ShowDebug = true
	//orm.ShowErr = true
	f, err := os.Create("xorm.log")
	if err != nil {
		fmt.Printf("models.init(fail to create xorm.log): %v\n", err)
		os.Exit(2)
	}
	orm.Logger = f
	orm.ShowSQL = true
}

func NewEngine() {
	setEngine()
	if err := orm.Sync(new(User), new(PublicKey), new(Repository), new(Watch),
		new(Action), new(Access)); err != nil {
		fmt.Printf("sync database struct error: %v\n", err)
		os.Exit(2)
	}
}

type Statistic struct {
	Counter struct {
		User, PublicKey, Repo, Watch, Action, Access int64
	}
}

func GetStatistic() (stats Statistic) {
	stats.Counter.User, _ = orm.Count(new(User))
	stats.Counter.PublicKey, _ = orm.Count(new(PublicKey))
	stats.Counter.Repo, _ = orm.Count(new(Repository))
	stats.Counter.Watch, _ = orm.Count(new(Watch))
	stats.Counter.Action, _ = orm.Count(new(Action))
	stats.Counter.Access, _ = orm.Count(new(Access))
	return stats
}
