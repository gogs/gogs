// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"strings"
	"time"

	"github.com/gogits/gogs/modules/base"
)

var (
	ErrIssueNotExist = errors.New("Issue does not exist")
)

// Issue represents an issue or pull request of repository.
type Issue struct {
	Id              int64
	Index           int64 // Index in one repository.
	Name            string
	RepoId          int64       `xorm:"index"`
	Repo            *Repository `xorm:"-"`
	PosterId        int64
	Poster          *User `xorm:"-"`
	MilestoneId     int64
	AssigneeId      int64
	IsPull          bool // Indicates whether is a pull request or not.
	IsClosed        bool
	Labels          string `xorm:"TEXT"`
	Mentions        string `xorm:"TEXT"`
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-"`
	NumComments     int
	Created         time.Time `xorm:"created"`
	Updated         time.Time `xorm:"updated"`
}

// CreateIssue creates new issue for repository.
func CreateIssue(userId, repoId, milestoneId, assigneeId int64, issueCount int, name, labels, content string, isPull bool) (issue *Issue, err error) {
	// TODO: find out mentions
	mentions := ""

	sess := orm.NewSession()
	defer sess.Close()
	sess.Begin()

	issue = &Issue{
		Index:       int64(issueCount) + 1,
		Name:        name,
		RepoId:      repoId,
		PosterId:    userId,
		MilestoneId: milestoneId,
		AssigneeId:  assigneeId,
		IsPull:      isPull,
		Labels:      labels,
		Mentions:    mentions,
		Content:     content,
	}
	if _, err = sess.Insert(issue); err != nil {
		sess.Rollback()
		return nil, err
	}

	rawSql := "UPDATE `repository` SET num_issues = num_issues + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, repoId); err != nil {
		sess.Rollback()
		return nil, err
	}

	if err = sess.Commit(); err != nil {
		sess.Rollback()
		return nil, err
	}

	return issue, nil
}

// GetIssueById returns issue object by given id.
func GetIssueByIndex(repoId, index int64) (*Issue, error) {
	issue := &Issue{RepoId: repoId, Index: index}
	has, err := orm.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist
	}
	return issue, nil
}

// GetIssues returns a list of issues by given conditions.
func GetIssues(userId, repoId, posterId, milestoneId int64, page int, isClosed, isMention bool, labels, sortType string) ([]Issue, error) {
	sess := orm.Limit(20, (page-1)*20)

	if repoId > 0 {
		sess.Where("repo_id=?", repoId).And("is_closed=?", isClosed)
	} else {
		sess.Where("is_closed=?", isClosed)
	}

	if userId > 0 {
		sess.And("assignee_id=?", userId)
	} else if posterId > 0 {
		sess.And("poster_id=?", posterId)
	} else if isMention {
		sess.And("mentions like '%$" + base.ToStr(userId) + "|%'")
	}

	if milestoneId > 0 {
		sess.And("milestone_id=?", milestoneId)
	}

	if len(labels) > 0 {
		for _, label := range strings.Split(labels, ",") {
			sess.And("mentions like '%$" + label + "|%'")
		}
	}

	switch sortType {
	case "oldest":
		sess.Asc("created")
	case "recentupdate":
		sess.Desc("updated")
	case "leastupdate":
		sess.Asc("updated")
	case "mostcomment":
		sess.Desc("num_comments")
	case "leastcomment":
		sess.Asc("num_comments")
	default:
		sess.Desc("created")
	}

	var issues []Issue
	err := sess.Find(&issues)
	return issues, err
}

// GetUserIssueCount returns the number of issues that were created by given user in repository.
func GetUserIssueCount(userId, repoId int64) int64 {
	count, _ := orm.Where("poster_id=?", userId).And("repo_id=?", repoId).Count(new(Issue))
	return count
}

// UpdateIssue updates information of issue.
func UpdateIssue(issue *Issue) error {
	_, err := orm.Id(issue.Id).AllCols().Update(issue)
	return err
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

// Issue types.
const (
	IT_PLAIN  = iota // Pure comment.
	IT_REOPEN        // Issue reopen status change prompt.
	IT_CLOSE         // Issue close status change prompt.
)

// Comment represents a comment in commit and issue page.
type Comment struct {
	Id       int64
	Type     int
	PosterId int64
	Poster   *User `xorm:"-"`
	IssueId  int64
	CommitId int64
	Line     int64
	Content  string
	Created  time.Time `xorm:"created"`
}

// CreateComment creates comment of issue or commit.
func CreateComment(userId, repoId, issueId, commitId, line int64, cmtType int, content string) error {
	sess := orm.NewSession()
	defer sess.Close()
	sess.Begin()

	if _, err := orm.Insert(&Comment{PosterId: userId, Type: cmtType, IssueId: issueId,
		CommitId: commitId, Line: line, Content: content}); err != nil {
		sess.Rollback()
		return err
	}

	// Check comment type.
	switch cmtType {
	case IT_PLAIN:
		rawSql := "UPDATE `issue` SET num_comments = num_comments + 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, issueId); err != nil {
			sess.Rollback()
			return err
		}
	case IT_REOPEN:
		rawSql := "UPDATE `repository` SET num_closed_issues = num_closed_issues - 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, repoId); err != nil {
			sess.Rollback()
			return err
		}
	case IT_CLOSE:
		rawSql := "UPDATE `repository` SET num_closed_issues = num_closed_issues + 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, repoId); err != nil {
			sess.Rollback()
			return err
		}
	}
	return sess.Commit()
}

// GetIssueComments returns list of comment by given issue id.
func GetIssueComments(issueId int64) ([]Comment, error) {
	comments := make([]Comment, 0, 10)
	err := orm.Asc("created").Find(&comments, &Comment{IssueId: issueId})
	return comments, err
}
