// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"time"
)

// Operation types of user action.
const (
	OP_CREATE_REPO = iota + 1
	OP_DELETE_REPO
	OP_STAR_REPO
	OP_FOLLOW_REPO
	OP_COMMIT_REPO
	OP_PULL_REQUEST
)

// An Action represents
type Action struct {
	Id       int64
	UserId   int64
	UserName string
	OpType   int
	RepoId   int64
	RepoName string
	Content  string
	Created  time.Time `xorm:"created"`
}

// NewRepoAction inserts action for create repository.
func NewRepoAction(user *User, repo *Repository) error {
	_, err := orm.InsertOne(&Action{
		UserId:   user.Id,
		UserName: user.Name,
		OpType:   OP_CREATE_REPO,
		RepoId:   repo.Id,
		RepoName: repo.Name,
	})
	return err
}

func GetFeeds(userid, offset int64) ([]Action, error) {
	actions := make([]Action, 0, 20)
	err := orm.Limit(20, int(offset)).Desc("id").Where("user_id=?", userid).Find(&actions)
	return actions, err
}
