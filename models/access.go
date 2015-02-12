// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"
	"time"
)

type AccessType int

const (
	READABLE AccessType = iota + 1
	WRITABLE
)

// Access represents the accessibility of user to repository.
type Access struct {
	Id       int64
	UserName string     `xorm:"UNIQUE(s)"`
	RepoName string     `xorm:"UNIQUE(s)"` // <user name>/<repo name>
	Mode     AccessType `xorm:"UNIQUE(s)"`
	Created  time.Time  `xorm:"CREATED"`
}

func addAccess(e Engine, access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	_, err := e.Insert(access)
	return err
}

// AddAccess adds new access record.
func AddAccess(access *Access) error {
	return addAccess(x, access)
}

func updateAccess(e Engine, access *Access) error {
	if _, err := e.Id(access.Id).Update(access); err != nil {
		return err
	}
	return nil
}

// UpdateAccess updates access information.
func UpdateAccess(access *Access) error {
	access.UserName = strings.ToLower(access.UserName)
	access.RepoName = strings.ToLower(access.RepoName)
	return updateAccess(x, access)
}

func deleteAccess(e Engine, access *Access) error {
	_, err := e.Delete(access)
	return err
}

// DeleteAccess deletes access record.
func DeleteAccess(access *Access) error {
	return deleteAccess(x, access)
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

// GetAccessibleRepositories finds all repositories where a user has access to,
// besides his own.
func (u *User) GetAccessibleRepositories() (map[*Repository]AccessType, error) {
	accesses := make([]*Access, 0, 10)
	if err := x.Find(&accesses, &Access{UserName: u.LowerName}); err != nil {
		return nil, err
	}

	repos := make(map[*Repository]AccessType, len(accesses))
	for _, access := range accesses {
		repo, err := GetRepositoryByRef(access.RepoName)
		if err != nil {
			return nil, err
		}
		if err = repo.GetOwner(); err != nil {
			return nil, err
		} else if repo.OwnerId == u.Id {
			continue
		}
		repos[repo] = access.Mode
	}

	return repos, nil
}
