// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/lunny/xorm"

	"github.com/gogits/gogs/modules/base"
)

var orm *xorm.Engine

func setEngine() {
	dbType := base.Cfg.MustValue("database", "DB_TYPE")
	dbHost := base.Cfg.MustValue("database", "HOST")
	dbName := base.Cfg.MustValue("database", "NAME")
	dbUser := base.Cfg.MustValue("database", "USER")
	dbPwd := base.Cfg.MustValue("database", "PASSWD")
	sslMode := base.Cfg.MustValue("database", "SSL_MODE")

	var err error
	switch dbType {
	case "mysql":
		orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%s:%s@%s/%s?charset=utf8",
			dbUser, dbPwd, dbHost, dbName))
	case "postgres":
		orm, err = xorm.NewEngine("postgres", fmt.Sprintf("user=%s password=%s dbname=%s sslmode=%s",
			dbUser, dbPwd, dbName, sslMode))
	default:
		fmt.Printf("Unknown database type: %s\n", dbType)
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

func init() {
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
