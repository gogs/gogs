// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"os"
	"path/filepath"
	"github.com/lunny/xorm"

	git "github.com/libgit2/git2go"
)

const (
	UserRepo = iota
	OrgRepo
)

type User struct {
	Id   int64
	Name string `xorm:"unique"`
}

type Org struct {
	Id int64
}

type Repo struct {
	Id      int64
	OwnerId int64  `xorm:"unique(s)"`
	Type    int    `xorm:"unique(s)"`
	Name    string `xorm:"unique(s)"`
}

var (
	orm *xorm.Engine
)

//
// create a repository for a user or orgnaziation
//
func CreateUserRepository(root string, user *User, reposName string) error {
	p := filepath.Join(root, user.Name)
	os.MkdirAll(p, os.ModePerm)
	f := filepath.Join(p, reposName)
	_, err := git.InitRepository(f, false)
	if err != nil {
		return err
	}

	repo := Repo{OwnerId: user.Id, Type: UserRepo, Name: reposName}
	_, err = orm.Insert(&repo)
	if err != nil {
		os.RemoveAll(f)
	}
	return err
}

//
// delete a repository for a user or orgnaztion
//
func DeleteUserRepository(root string, user *User, reposName string) error {
	err := os.RemoveAll(filepath.Join(root, user.Name, reposName))
	if err != nil {
		return err
	}

	_, err = orm.Delete(&Repo{OwnerId: user.Id, Type: UserRepo, Name: reposName})
	return err
}
