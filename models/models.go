// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"
	"github.com/lunny/xorm"

	"github.com/gogits/gogs/utils"
	"github.com/gogits/gogs/utils/log"
)

var (
	orm          *xorm.Engine
	RepoRootPath string
)

type Members struct {
	Id     int64
	OrgId  int64 `xorm:"unique(s) index"`
	UserId int64 `xorm:"unique(s)"`
}

type Issue struct {
	Id       int64
	RepoId   int64 `xorm:"index"`
	PosterId int64
}

type PullRequest struct {
	Id int64
}

type Comment struct {
	Id int64
}

func setEngine() {
	dbType := utils.Cfg.MustValue("database", "DB_TYPE")
	dbHost := utils.Cfg.MustValue("database", "HOST")
	dbName := utils.Cfg.MustValue("database", "NAME")
	dbUser := utils.Cfg.MustValue("database", "USER")
	dbPwd := utils.Cfg.MustValue("database", "PASSWD")

	var err error
	switch dbType {
	case "mysql":
		orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%v:%v@%v/%v?charset=utf8",
			dbUser, dbPwd, dbHost, dbName))
	default:
		log.Critical("Unknown database type: %s", dbType)
		os.Exit(2)
	}

	if err != nil {
		log.Critical("models.init -> Conntect database: %s", dbType)
		os.Exit(2)
	}

	//x.ShowDebug = true
	orm.ShowErr = true
	//x.ShowSQL = true

	log.Trace("Initialized database -> %s", dbName)
}

func init() {
	setEngine()
	err := orm.Sync(new(User), new(PublicKey), new(Repo), new(Access))
	if err != nil {
		log.Error("sync database struct error: %s", err)
		os.Exit(1)
	}
}
