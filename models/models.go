// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"os"
	"os/user"

	_ "github.com/go-sql-driver/mysql"
	"github.com/lunny/xorm"

	"github.com/gogits/gogs/utils"
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

	uname, err := user.Current()
	if err != nil {
		fmt.Printf("models.init -> fail to get user: %s\n", err)
		os.Exit(2)
	}

	if uname.Username == "jiahuachen" {
		dbPwd = utils.Cfg.MustValue("database", "PASSWD_jiahua")
	}

	switch dbType {
	case "mysql":
		orm, err = xorm.NewEngine("mysql", fmt.Sprintf("%v:%v@%v/%v?charset=utf8",
			dbUser, dbPwd, dbHost, dbName))
	default:
		fmt.Printf("Unknown database type: %s\n", dbType)
		os.Exit(2)
	}

	if err != nil {
		fmt.Printf("models.init -> fail to conntect database: %s\n", dbType)
		os.Exit(2)
	}

	//TODO: for serv command, MUST remove the output to os.stdout, so
	// use log file to instead print to stdout

	//x.ShowDebug = true
	//orm.ShowErr = true
	f, _ := os.Create("xorm.log")
	orm.Logger = f
	orm.ShowSQL = true

	//log.Trace("Initialized database -> %s", dbName)

	RepoRootPath = utils.Cfg.MustValue("repository", "ROOT")
}

func init() {
	setEngine()
	err := orm.Sync(new(User), new(PublicKey), new(Repo), new(Access))
	if err != nil {
		fmt.Printf("sync database struct error: %s\n", err)
		os.Exit(2)
	}
}
