// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"

	"github.com/lunny/xorm"
)

var (
	orm          *xorm.Engine
	repoRootPath string
)

type PublicKey struct {
	Id      int64
	Name    string    `xorm:"unique not null"`
	Content string    `xorm:"text not null"`
	Created time.Time `xorm:"created"`
	Updated time.Time `xorm:"updated"`
}

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
