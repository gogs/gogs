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
	Id          int64
	UserId      int64 // Receiver user id.
	OpType      int
	ActUserId   int64  // Action user id.
	ActUserName string // Action user name.
	RepoId      int64
	RepoName    string
	Content     string
	Created     time.Time `xorm:"created"`
}

func (a Action) GetOpType() int {
	return a.OpType
}

func (a Action) GetActUserName() string {
	return a.ActUserName
}

func (a Action) GetRepoName() string {
	return a.RepoName
}

// CommitRepoAction records action for commit repository.
func CommitRepoAction(userId int64, userName string,
	repoId int64, repoName string, msg string) error {
	_, err := orm.InsertOne(&Action{
		UserId:      userId,
		ActUserId:   userId,
		ActUserName: userName,
		OpType:      OP_COMMIT_REPO,
		Content:     msg,
		RepoId:      repoId,
		RepoName:    repoName,
	})
	return err
}

// NewRepoAction records action for create repository.
func NewRepoAction(user *User, repo *Repository) error {
	_, err := orm.InsertOne(&Action{
		UserId:      user.Id,
		ActUserId:   user.Id,
		ActUserName: user.Name,
		OpType:      OP_CREATE_REPO,
		RepoId:      repo.Id,
		RepoName:    repo.Name,
	})
	return err
}

// GetFeeds returns action list of given user in given context.
func GetFeeds(userid, offset int64, isProfile bool) ([]Action, error) {
	actions := make([]Action, 0, 20)
	sess := orm.Limit(20, int(offset)).Desc("id").Where("user_id=?", userid)
	if isProfile {
		sess.And("act_user_id=?", userid)
	} else {
		sess.And("act_user_id!=?", userid)
	}
	err := sess.Find(&actions)
	return actions, err
}
