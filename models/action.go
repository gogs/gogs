// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"encoding/json"
	"time"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
)

// Operation types of user action.
const (
	OP_CREATE_REPO = iota + 1
	OP_DELETE_REPO
	OP_STAR_REPO
	OP_FOLLOW_REPO
	OP_COMMIT_REPO
	OP_CREATE_ISSUE
	OP_PULL_REQUEST
)

// Action represents user operation type and other information to repository.,
// it implemented interface base.Actioner so that can be used in template render.
type Action struct {
	Id          int64
	UserId      int64  // Receiver user id.
	OpType      int    // Operations: CREATE DELETE STAR ...
	ActUserId   int64  // Action user id.
	ActUserName string // Action user name.
	RepoId      int64
	RepoName    string
	RefName     string
	Content     string    `xorm:"TEXT"`
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

func (a Action) GetBranch() string {
	return a.RefName
}

func (a Action) GetContent() string {
	return a.Content
}

// CommitRepoAction adds new action for committing repository.
func CommitRepoAction(userId int64, userName string,
	repoId int64, repoName string, refName string, commits *base.PushCommits) error {
	log.Trace("action.CommitRepoAction(start): %d/%s", userId, repoName)

	bs, err := json.Marshal(commits)
	if err != nil {
		log.Error("action.CommitRepoAction(json): %d/%s", userId, repoName)
		return err
	}

	if err = NotifyWatchers(&Action{ActUserId: userId, ActUserName: userName, OpType: OP_COMMIT_REPO,
		Content: string(bs), RepoId: repoId, RepoName: repoName, RefName: refName}); err != nil {
		log.Error("action.CommitRepoAction(notify watchers): %d/%s", userId, repoName)
		return err
	}

	// Change repository bare status and update last updated time.
	repo, err := GetRepositoryByName(userId, repoName)
	if err != nil {
		log.Error("action.CommitRepoAction(GetRepositoryByName): %d/%s", userId, repoName)
		return err
	}
	repo.IsBare = false
	if err = UpdateRepository(repo); err != nil {
		log.Error("action.CommitRepoAction(UpdateRepository): %d/%s", userId, repoName)
		return err
	}

	log.Trace("action.CommitRepoAction(end): %d/%s", userId, repoName)
	return nil
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

	log.Trace("action.NewRepoAction: %s/%s", user.LowerName, repo.LowerName)
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
