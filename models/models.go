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

	// TODO: for serv command, MUST remove the output to os.stdout, so
	// use log file to instead print to stdout

	//x.ShowDebug = true
	//orm.ShowErr = true
	f, err := os.Create("xorm.log")
	if err != nil {
		fmt.Printf("models.init(fail to create xorm.log): %v\n", err)
		os.Exit(2)
	}
	orm.Logger = f
	orm.ShowSQL = true

	// Determine and create root git reposiroty path.
	RepoRootPath = base.Cfg.MustValue("repository", "ROOT")
	if err = os.MkdirAll(RepoRootPath, os.ModePerm); err != nil {
		fmt.Printf("models.init(fail to create RepoRootPath(%s)): %v\n", RepoRootPath, err)
		os.Exit(2)
	}
}

func init() {
	setEngine()
	if err := orm.Sync(new(User), new(PublicKey), new(Repository), new(Access),
		new(Action), new(Watch)); err != nil {
		fmt.Printf("sync database struct error: %v\n", err)
		os.Exit(2)
	}
}
