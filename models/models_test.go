// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"testing"

	"github.com/lunny/xorm"
	_ "github.com/mattn/go-sqlite3"
	. "github.com/smartystreets/goconvey/convey"

	"github.com/gogits/gogs/modules/base"
)

func init() {
	var err error
	orm, err = xorm.NewEngine("sqlite3", "./test.db")
	if err != nil {
		fmt.Println(err)
	}

	orm.ShowSQL = true
	orm.ShowDebug = true

	err = orm.Sync(&User{}, &Repository{})
	if err != nil {
		fmt.Println(err)
	}

	base.RepoRootPath = "test"
}

func TestCreateRepository(t *testing.T) {
	user := User{Id: 1, Name: "foobar", Type: UT_INDIVIDUAL}
	_, err := CreateRepository(&user, "test", "", "", "test repo desc", false, false)
	if err != nil {
		t.Error(err)
	}
}

func TestDeleteRepository(t *testing.T) {
	err := DeleteRepository(1, 1, "foobar")
	if err != nil {
		t.Error(err)
	}
}

func TestCommitRepoAction(t *testing.T) {
	Convey("Create a commit repository action", t, func() {

	})
}
