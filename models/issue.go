// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"strings"
	"time"

	"github.com/gogits/gogs/modules/base"
)

// Issue represents an issue or pull request of repository.
type Issue struct {
	Id          int64
	Index       int64 // Index in one repository.
	Name        string
	RepoId      int64 `xorm:"index"`
	PosterId    int64
	MilestoneId int64
	AssigneeId  int64
	IsPull      bool // Indicates whether is a pull request or not.
	IsClosed    bool
	Labels      string
	Mentions    string
	Content     string
	NumComments int
	Created     time.Time `xorm:"created"`
	Updated     time.Time `xorm:"updated"`
}

// CreateIssue creates new issue for repository.
func CreateIssue(userId, repoId, milestoneId, assigneeId int64, name, labels, mentions, content string, isPull bool) error {
	count, err := GetIssueCount(repoId)
	if err != nil {
		return err
	}

	_, err = orm.Insert(&Issue{
		Index:       count + 1,
		Name:        name,
		RepoId:      repoId,
		PosterId:    userId,
		MilestoneId: milestoneId,
		AssigneeId:  assigneeId,
		IsPull:      isPull,
		Labels:      labels,
		Mentions:    mentions,
		Content:     content,
	})
	return err
}

// GetIssueCount returns count of issues in the repository.
func GetIssueCount(repoId int64) (int64, error) {
	return orm.Count(&Issue{RepoId: repoId})
}

// GetIssues returns a list of issues by given conditions.
func GetIssues(userId, repoId, posterId, milestoneId int64, page int, isClosed, isMention bool, labels, sortType string) ([]Issue, error) {
	sess := orm.Limit(20, (page-1)*20).Where("repo_id=?", repoId).And("is_closed=?", isClosed)
	if userId > 0 {
		sess = sess.And("assignee_id=?", userId)
	} else if posterId > 0 {
		sess = sess.And("poster_id=?", posterId)
	} else if isMention {
		sess = sess.And("mentions like '%$" + base.ToStr(userId) + "|%'")
	}

	if milestoneId > 0 {
		sess = sess.And("milestone_id=?", milestoneId)
	}

	if len(labels) > 0 {
		for _, label := range strings.Split(labels, ",") {
			sess = sess.And("mentions like '%$" + label + "|%'")
		}
	}

	switch sortType {
	case "oldest":
		sess = sess.Asc("created")
	case "recentupdate":
		sess = sess.Desc("updated")
	case "leastupdate":
		sess = sess.Asc("updated")
	case "mostcomment":
		sess = sess.Desc("num_comments")
	case "leastcomment":
		sess = sess.Asc("num_comments")
	default:
		sess = sess.Desc("created")
	}

	var issues []Issue
	err := sess.Find(&issues)
	return issues, err
}

// Label represents a list of labels of repository for issues.
type Label struct {
	Id     int64
	RepoId int64 `xorm:"index"`
	Names  string
	Colors string
}

// Milestone represents a milestone of repository.
type Milestone struct {
	Id        int64
	Name      string
	RepoId    int64 `xorm:"index"`
	IsClosed  bool
	Content   string
	NumIssues int
	DueDate   time.Time
	Created   time.Time `xorm:"created"`
}

// Comment represents a comment in commit and issue page.
type Comment struct {
	Id       int64
	PosterId int64
	IssueId  int64
	CommitId int64
	Line     int
	Content  string
	Created  time.Time `xorm:"created"`
}
