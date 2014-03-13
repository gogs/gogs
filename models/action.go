// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
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
	Id      int64
	UserId  int64
	OpType  int
	RepoId  int64
	Content string
	Created time.Time `xorm:"created"`
}

type NewRepoContent struct {
	UserName string
	RepoName string
}

// NewRepoAction inserts action for create repository.
func NewRepoAction(user *User, repo *Repository) error {
	content, err := json.Marshal(&NewRepoContent{user.Name, repo.Name})
	if err != nil {
		return err
	}
	_, err = orm.InsertOne(&Action{
		UserId:  user.Id,
		OpType:  OP_CREATE_REPO,
		RepoId:  repo.Id,
		Content: string(content),
	})
	return err
}

func GetFeeds(userid, offset int64) ([]Action, error) {
	actions := make([]Action, 0, 20)
	err := orm.Limit(20, int(offset)).Desc("id").Where("user_id=?", userid).Find(&actions)
	return actions, err
}
