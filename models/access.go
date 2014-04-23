// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"
	"time"

	"github.com/go-xorm/xorm"
)

// Access types.
const (
	AU_READABLE = iota + 1
	AU_WRITABLE
)

// Access represents the accessibility of user to repository.
type Access struct {
	Id       int64
	UserName string    `xorm:"unique(s)"`
	RepoName string    `xorm:"unique(s)"` // <user name>/<repo name>
	Mode     int       `xorm:"unique(s)"`
	Created  time.Time `xorm:"created"`
}

// AddAccess adds new access record.
func AddAccess(access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	_, err := orm.Insert(access)
	return err
}

// UpdateAccess updates access information.
func UpdateAccess(access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	_, err := orm.Id(access.Id).Update(access)
	return err
}

// UpdateAccess updates access information with session for rolling back.
func UpdateAccessWithSession(sess *xorm.Session, access *Access) error {
	if _, err := sess.Id(access.Id).Update(access); err != nil {
		sess.Rollback()
		return err
	}
	return nil
}

// HasAccess returns true if someone can read or write to given repository.
func HasAccess(userName, repoName string, mode int) (bool, error) {
	access := &Access{
		UserName: strings.ToLower(userName),
		RepoName: strings.ToLower(repoName),
	}
	has, err := orm.Get(access)
	if err != nil {
		return false, err
	} else if !has {
		return false, nil
	} else if mode > access.Mode {
		return false, nil
	}
	return true, nil
}
