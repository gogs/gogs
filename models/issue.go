// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrIssueNotExist = errors.New("Issue does not exist")
)

// Issue represents an issue or pull request of repository.
type Issue struct {
	Id              int64
	Index           int64 // Index in one repository.
	Name            string
	RepoId          int64       `xorm:"INDEX"`
	Repo            *Repository `xorm:"-"`
	PosterId        int64
	Poster          *User `xorm:"-"`
	MilestoneId     int64
	AssigneeId      int64
	IsPull          bool // Indicates whether is a pull request or not.
	IsClosed        bool
	Labels          string `xorm:"TEXT"`
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-"`
	Priority        int
	NumComments     int
	Deadline        time.Time
	Created         time.Time `xorm:"CREATED"`
	Updated         time.Time `xorm:"UPDATED"`
}

func (i *Issue) GetPoster() (err error) {
	i.Poster, err = GetUserById(i.PosterId)
	return err
}

// IssseUser represents an issue-user relation.
type IssseUser struct {
	Id          int64
	Iid         int64 // Issue ID.
	Rid         int64 // Repository ID.
	Uid         int64 // User ID.
	IsRead      bool
	IsAssigned  bool
	IsMentioned bool
	IsClosed    bool
}

// CreateIssue creates new issue for repository.
func NewIssue(issue *Issue) (err error) {
	sess := orm.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(issue); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `repository` SET num_issues = num_issues + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, issue.RepoId); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// GetIssueByIndex returns issue by given index in repository.
func GetIssueByIndex(rid, index int64) (*Issue, error) {
	issue := &Issue{RepoId: rid, Index: index}
	has, err := orm.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist
	}
	return issue, nil
}

// GetIssues returns a list of issues by given conditions.
func GetIssues(uid, rid, pid, mid int64, page int, isClosed bool, labels, sortType string) ([]Issue, error) {
	sess := orm.Limit(20, (page-1)*20)

	if rid > 0 {
		sess.Where("repo_id=?", rid).And("is_closed=?", isClosed)
	} else {
		sess.Where("is_closed=?", isClosed)
	}

	if uid > 0 {
		sess.And("assignee_id=?", uid)
	} else if pid > 0 {
		sess.And("poster_id=?", pid)
	}

	if mid > 0 {
		sess.And("milestone_id=?", mid)
	}

	if len(labels) > 0 {
		for _, label := range strings.Split(labels, ",") {
			sess.And("labels like '%$" + label + "|%'")
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

// PairsContains returns true when pairs list contains given issue.
func PairsContains(ius []*IssseUser, issueId int64) bool {
	for i := range ius {
		if ius[i].Iid == issueId {
			return true
		}
	}
	return false
}

// GetIssueUserPairs returns all issue-user pairs by given repository and user.
func GetIssueUserPairs(rid, uid int64, isClosed bool) ([]*IssseUser, error) {
	ius := make([]*IssseUser, 0, 10)
	err := orm.Find(&ius, &IssseUser{Rid: rid, Uid: uid, IsClosed: isClosed})
	return ius, err
}

// GetUserIssueCount returns the number of issues that were created by given user in repository.
func GetUserIssueCount(uid, rid int64) int64 {
	count, _ := orm.Where("poster_id=?", uid).And("repo_id=?", rid).Count(new(Issue))
	return count
}

// IssueStats represents issue statistic information.
type IssueStats struct {
	OpenCount, ClosedCount int64
	AllCount               int64
	AssignCount            int64
	CreateCount            int64
	MentionCount           int64
}

// Filter modes.
const (
	FM_ASSIGN = iota + 1
	FM_CREATE
	FM_MENTION
)

// GetIssueStats returns issue statistic information by given condition.
func GetIssueStats(rid, uid int64, isShowClosed bool, filterMode int) *IssueStats {
	stats := &IssueStats{}
	issue := new(Issue)

	sess := orm.Where("repo_id=?", rid)
	tmpSess := sess
	stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(issue)
	*tmpSess = *sess
	stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(issue)
	if isShowClosed {
		stats.AllCount = stats.ClosedCount
	} else {
		stats.AllCount = stats.OpenCount
	}

	if filterMode != FM_MENTION {
		sess = orm.Where("repo_id=?", rid)
		switch filterMode {
		case FM_ASSIGN:
			sess.And("assignee_id=?", uid)
		case FM_CREATE:
			sess.And("poster_id=?", uid)
		default:
			goto nofilter
		}
		*tmpSess = *sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(issue)
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(issue)
	} else {
		sess := orm.Where("rid=?", rid).And("uid=?", uid).And("is_mentioned=?", true)
		tmpSess := sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(new(IssseUser))
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(new(IssseUser))
	}
nofilter:
	stats.AssignCount, _ = orm.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("assignee_id=?", uid).Count(issue)
	stats.CreateCount, _ = orm.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("poster_id=?", uid).Count(issue)
	stats.MentionCount, _ = orm.Where("rid=?", rid).And("uid=?", uid).And("is_closed=?", isShowClosed).And("is_mentioned=?", true).Count(new(IssseUser))
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given condition.
func GetUserIssueStats(uid int64, filterMode int) *IssueStats {
	stats := &IssueStats{}
	issue := new(Issue)
	iu := new(IssseUser)

	sess := orm.Where("uid=?", uid)
	tmpSess := sess
	if filterMode == 0 {
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(iu)
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(iu)
	}

	switch filterMode {
	case FM_ASSIGN:
		sess.And("is_assigned=?", true)
		*tmpSess = *sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(iu)
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(iu)
	case FM_CREATE:
		sess.Where("poster_id=?", uid)
		*tmpSess = *sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(issue)
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(issue)
	}

	stats.AssignCount, _ = orm.Where("assignee_id=?", uid).And("is_closed=?", false).Count(issue)
	stats.CreateCount, _ = orm.Where("poster_id=?", uid).And("is_closed=?", false).Count(issue)
	return stats
}

// UpdateIssue updates information of issue.
func UpdateIssue(issue *Issue) error {
	_, err := orm.AllCols().Update(issue)
	return err
}

// Label represents a list of labels of repository for issues.
type Label struct {
	Id     int64
	RepoId int64 `xorm:"INDEX"`
	Names  string
	Colors string
}

// Milestone represents a milestone of repository.
type Milestone struct {
	Id        int64
	Name      string
	RepoId    int64 `xorm:"INDEX"`
	IsClosed  bool
	Content   string
	NumIssues int
	DueDate   time.Time
	Created   time.Time `xorm:"CREATED"`
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
	Created  time.Time `xorm:"CREATED"`
}

// CreateComment creates comment of issue or commit.
func CreateComment(userId, repoId, issueId, commitId, line int64, cmtType int, content string) error {
	sess := orm.NewSession()
	defer sess.Close()
	sess.Begin()

	if _, err := sess.Insert(&Comment{PosterId: userId, Type: cmtType, IssueId: issueId,
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
