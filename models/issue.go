// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
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
	RepoId          int64       `xorm:"INDEX"`
	Repo            *Repository `xorm:"-"`
	PosterId        int64
	Poster          *User `xorm:"-"`
	MilestoneId     int64
	AssigneeId      int64
	Assignee        *User `xorm:"-"`
	IsRead          bool  `xorm:"-"`
	IsPull          bool  // Indicates whether is a pull request or not.
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
	if err == ErrUserNotExist {
		i.Poster = &User{Name: "FakeUser"}
		return nil
	}
	return err
}

func (i *Issue) GetAssignee() (err error) {
	if i.AssigneeId == 0 {
		return nil
	}
	i.Assignee, err = GetUserById(i.AssigneeId)
	return err
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

// GetIssueById returns an issue by ID.
func GetIssueById(id int64) (*Issue, error) {
	issue := &Issue{Id: id}
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
	case "priority":
		sess.Desc("priority")
	default:
		sess.Desc("created")
	}

	var issues []Issue
	err := sess.Find(&issues)
	return issues, err
}

// GetIssueCountByPoster returns number of issues of repository by poster.
func GetIssueCountByPoster(uid, rid int64, isClosed bool) int64 {
	count, _ := orm.Where("repo_id=?", rid).And("poster_id=?", uid).And("is_closed=?", isClosed).Count(new(Issue))
	return count
}

// IssueUser represents an issue-user relation.
type IssueUser struct {
	Id          int64
	Uid         int64 // User ID.
	IssueId     int64
	RepoId      int64
	IsRead      bool
	IsAssigned  bool
	IsMentioned bool
	IsPoster    bool
	IsClosed    bool
}

// NewIssueUserPairs adds new issue-user pairs for new issue of repository.
func NewIssueUserPairs(rid, iid, oid, pid, aid int64, repoName string) (err error) {
	iu := &IssueUser{IssueId: iid, RepoId: rid}

	us, err := GetCollaborators(repoName)
	if err != nil {
		return err
	}

	isNeedAddPoster := true
	for _, u := range us {
		iu.Uid = u.Id
		iu.IsPoster = iu.Uid == pid
		if isNeedAddPoster && iu.IsPoster {
			isNeedAddPoster = false
		}
		iu.IsAssigned = iu.Uid == aid
		if _, err = orm.Insert(iu); err != nil {
			return err
		}
	}
	if isNeedAddPoster {
		iu.Uid = pid
		iu.IsPoster = true
		iu.IsAssigned = iu.Uid == aid
		if _, err = orm.Insert(iu); err != nil {
			return err
		}
	}

	return nil
}

// PairsContains returns true when pairs list contains given issue.
func PairsContains(ius []*IssueUser, issueId int64) int {
	for i := range ius {
		if ius[i].IssueId == issueId {
			return i
		}
	}
	return -1
}

// GetIssueUserPairs returns issue-user pairs by given repository and user.
func GetIssueUserPairs(rid, uid int64, isClosed bool) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	err := orm.Where("is_closed=?", isClosed).Find(&ius, &IssueUser{RepoId: rid, Uid: uid})
	return ius, err
}

// GetIssueUserPairsByRepoIds returns issue-user pairs by given repository IDs.
func GetIssueUserPairsByRepoIds(rids []int64, isClosed bool, page int) ([]*IssueUser, error) {
	buf := bytes.NewBufferString("")
	for _, rid := range rids {
		buf.WriteString("repo_id=")
		buf.WriteString(base.ToStr(rid))
		buf.WriteString(" OR ")
	}
	cond := strings.TrimSuffix(buf.String(), " OR ")

	ius := make([]*IssueUser, 0, 10)
	sess := orm.Limit(20, (page-1)*20).Where("is_closed=?", isClosed)
	if len(cond) > 0 {
		sess.And(cond)
	}
	err := sess.Find(&ius)
	return ius, err
}

// GetIssueUserPairsByMode returns issue-user pairs by given repository and user.
func GetIssueUserPairsByMode(uid, rid int64, isClosed bool, page, filterMode int) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	sess := orm.Limit(20, (page-1)*20).Where("uid=?", uid).And("is_closed=?", isClosed)
	if rid > 0 {
		sess.And("repo_id=?", rid)
	}

	switch filterMode {
	case FM_ASSIGN:
		sess.And("is_assigned=?", true)
	case FM_CREATE:
		sess.And("is_poster=?", true)
	default:
		return ius, nil
	}
	err := sess.Find(&ius)
	return ius, err
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

// GetIssueStats returns issue statistic information by given conditions.
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
		sess := orm.Where("repo_id=?", rid).And("uid=?", uid).And("is_mentioned=?", true)
		*tmpSess = *sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(new(IssueUser))
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(new(IssueUser))
	}
nofilter:
	stats.AssignCount, _ = orm.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("assignee_id=?", uid).Count(issue)
	stats.CreateCount, _ = orm.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("poster_id=?", uid).Count(issue)
	stats.MentionCount, _ = orm.Where("repo_id=?", rid).And("uid=?", uid).And("is_closed=?", isShowClosed).And("is_mentioned=?", true).Count(new(IssueUser))
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(uid int64, filterMode int) *IssueStats {
	stats := &IssueStats{}
	issue := new(Issue)
	stats.AssignCount, _ = orm.Where("assignee_id=?", uid).And("is_closed=?", false).Count(issue)
	stats.CreateCount, _ = orm.Where("poster_id=?", uid).And("is_closed=?", false).Count(issue)
	return stats
}

// UpdateIssue updates information of issue.
func UpdateIssue(issue *Issue) error {
	_, err := orm.Id(issue.Id).AllCols().Update(issue)
	return err
}

// UpdateIssueUserByStatus updates issue-user pairs by issue status.
func UpdateIssueUserPairsByStatus(iid int64, isClosed bool) error {
	rawSql := "UPDATE `issue_user` SET is_closed = ? WHERE issue_id = ?"
	_, err := orm.Exec(rawSql, isClosed, iid)
	return err
}

// UpdateIssueUserPairByAssignee updates issue-user pair for assigning.
func UpdateIssueUserPairByAssignee(aid, iid int64) error {
	rawSql := "UPDATE `issue_user` SET is_assigned = ? WHERE issue_id = ?"
	if _, err := orm.Exec(rawSql, false, iid); err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if aid == 0 {
		return nil
	}
	rawSql = "UPDATE `issue_user` SET is_assigned = true WHERE uid = ? AND issue_id = ?"
	_, err := orm.Exec(rawSql, aid, iid)
	return err
}

// UpdateIssueUserPairByRead updates issue-user pair for reading.
func UpdateIssueUserPairByRead(uid, iid int64) error {
	rawSql := "UPDATE `issue_user` SET is_read = ? WHERE uid = ? AND issue_id = ?"
	_, err := orm.Exec(rawSql, true, uid, iid)
	return err
}

// UpdateIssueUserPairsByMentions updates issue-user pairs by mentioning.
func UpdateIssueUserPairsByMentions(uids []int64, iid int64) error {
	for _, uid := range uids {
		iu := &IssueUser{Uid: uid, IssueId: iid}
		has, err := orm.Get(iu)
		if err != nil {
			return err
		}

		iu.IsMentioned = true
		if has {
			_, err = orm.Id(iu.Id).AllCols().Update(iu)
		} else {
			_, err = orm.Insert(iu)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// Label represents a label of repository for issues.
type Label struct {
	Id              int64
	Rid             int64 `xorm:"INDEX"`
	Name            string
	Color           string
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int `xorm:"-"`
}

// Milestone represents a milestone of repository.
type Milestone struct {
	Id              int64
	Rid             int64 `xorm:"INDEX"`
	Name            string
	Content         string
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	Completeness    int // Percentage(1-100).
	Deadline        time.Time
	ClosedDate      time.Time
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
	if err := sess.Begin(); err != nil {
		return err
	}

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
