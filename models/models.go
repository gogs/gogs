// Copyright (c) 2013 GPMGo Members. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package models implemented database access funtions.

package models

import (
	"database/sql"
	"errors"
	//"os"
	"strconv"
	"strings"
	"time"

	"github.com/coocood/qbs"
	_ "github.com/mattn/go-sqlite3"
)

const (
	DB_NAME         = "./data/gowalker.db"
	_SQLITE3_DRIVER = "sqlite3"
)

// PkgInfo is package information.
type PkgInfo struct {
	Id      int64
	Path    string `qbs:"index"` // Import path of package.
	AbsPath string
	Imports []string
	Note    string
	Created time.Time `qbs:"index"` // Time when information last updated.
	Commit  string    // Revision tag and project tags.
}

func connDb() *qbs.Qbs {
	// 'sql.Open' only returns error when unknown driver, so it's not necessary to check in other places.
	db, err := sql.Open(_SQLITE3_DRIVER, DB_NAME)
	if err != nil {
		//beego.Error("models.connDb():", err)
	}
	q := qbs.New(db, qbs.NewSqlite3())
	return q
}

func setMg() (*qbs.Migration, error) {
	db, err := sql.Open(_SQLITE3_DRIVER, DB_NAME)
	mg := qbs.NewMigration(db, DB_NAME, qbs.NewSqlite3())
	return mg, err
}

/*func init() {
	// Initialize database.
	os.Mkdir("./data", os.ModePerm)

	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	mg, err := setMg()
	if err != nil {
		beego.Error("models.init():", err)
	}
	defer mg.Db.Close()

	// Create data tables.
	mg.CreateTableIfNotExists(new(PkgInfo))

	beego.Trace("Initialized database ->", DB_NAME)
}*/

// GetProInfo returns package information from database.
func GetPkgInfo(path string) (*PkgInfo, error) {
	// Check path length to reduce connect times.
	if len(path) == 0 {
		return nil, errors.New("models.GetPkgInfo(): Empty path as not found.")
	}

	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	pinfo := new(PkgInfo)
	err := q.WhereEqual("path", path).Find(pinfo)

	return pinfo, err
}

// GetGroupPkgInfo returns group of package infomration in order to reduce database connect times.
func GetGroupPkgInfo(paths []string) ([]*PkgInfo, error) {
	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	pinfos := make([]*PkgInfo, 0, len(paths))
	for _, v := range paths {
		if len(v) > 0 {
			pinfo := new(PkgInfo)
			err := q.WhereEqual("path", v).Find(pinfo)
			if err == nil {
				pinfos = append(pinfos, pinfo)
			} else {
				pinfos = append(pinfos, &PkgInfo{Path: v})
			}
		}
	}
	return pinfos, nil
}

// GetPkgInfoById returns package information from database by pid.
func GetPkgInfoById(pid int) (*PkgInfo, error) {
	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	pinfo := new(PkgInfo)
	err := q.WhereEqual("id", pid).Find(pinfo)

	return pinfo, err
}

// GetGroupPkgInfoById returns group of package infomration by pid in order to reduce database connect times.
// The formatted pid looks like '$<pid>|', so we need to cut '$' here.
func GetGroupPkgInfoById(pids []string) ([]*PkgInfo, error) {
	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	pinfos := make([]*PkgInfo, 0, len(pids))
	for _, v := range pids {
		if len(v) > 1 {
			pid, err := strconv.Atoi(v[1:])
			if err == nil {
				pinfo := new(PkgInfo)
				err = q.WhereEqual("id", pid).Find(pinfo)
				if err == nil {
					pinfos = append(pinfos, pinfo)
				}
			}
		}
	}
	return pinfos, nil
}

// DeleteProject deletes everything about the path in database, and update import information.
func DeleteProject(path string) error {
	// Check path length to reduce connect times. (except launchpad.net)
	if path[0] != 'l' && len(strings.Split(path, "/")) <= 2 {
		return errors.New("models.DeleteProject(): Short path as not needed.")
	}

	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	var i1 int64
	// Delete package information.
	info := new(PkgInfo)
	err := q.WhereEqual("path", path).Find(info)
	if err == nil {
		i1, err = q.Delete(info)
		if err != nil {
			//beego.Error("models.DeleteProject(): Information:", err)
		}
	}

	if i1 > 0 {
		//beego.Info("models.DeleteProject(", path, i1, ")")
	}

	return nil
}

// SearchDoc returns packages information that contain keyword
func SearchDoc(key string) ([]*PkgInfo, error) {
	// Connect to database.
	q := connDb()
	defer q.Db.Close()

	var pkgInfos []*PkgInfo
	condition := qbs.NewCondition("path like ?", "%"+key+"%")
	err := q.Condition(condition).OrderBy("path").FindAll(&pkgInfos)
	return pkgInfos, err
}
