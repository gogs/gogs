// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"
	"time"

	"github.com/go-xorm/xorm"
)

type AccessType int

const (
	READABLE AccessType = iota + 1
	WRITABLE
)

// Access represents the accessibility of user to repository.
type Access struct {
	Id       int64
	UserName string     `xorm:"unique(s)"`
	RepoName string     `xorm:"unique(s)"` // <user name>/<repo name>
	Mode     AccessType `xorm:"unique(s)"`
	Created  time.Time  `xorm:"created"`
}

// AddAccess adds new access record.
func AddAccess(access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	_, err := x.Insert(access)
	return err
}

// UpdateAccess updates access information.
func UpdateAccess(access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	_, err := x.Id(access.Id).Update(access)
	return err
}

// DeleteAccess deletes access record.
func DeleteAccess(access *Access) error {
	_, err := x.Delete(access)
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
// The repoName should be in format <username>/<reponame>.
func HasAccess(uname, repoName string, mode AccessType) (bool, error) {
	if len(repoName) == 0 {
		return false, nil
	}
	access := &Access{
		UserName: strings.ToLower(uname),
		RepoName: strings.ToLower(repoName),
	}
	has, err := x.Get(access)
	if err != nil {
		return false, err
	} else if !has {
		return false, nil
	} else if mode > access.Mode {
		return false, nil
	}
	return true, nil
}
