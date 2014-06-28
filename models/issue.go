// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
	"errors"
	"strings"
	"time"

	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/base"
)

var (
	ErrIssueNotExist     = errors.New("Issue does not exist")
	ErrLabelNotExist     = errors.New("Label does not exist")
	ErrMilestoneNotExist = errors.New("Milestone does not exist")
)

// Issue represents an issue or pull request of repository.
type Issue struct {
	Id              int64
	RepoId          int64 `xorm:"INDEX"`
	Index           int64 // Index in one repository.
	Name            string
	Repo            *Repository `xorm:"-"`
	PosterId        int64
	Poster          *User    `xorm:"-"`
	LabelIds        string   `xorm:"TEXT"`
	Labels          []*Label `xorm:"-"`
	MilestoneId     int64
	AssigneeId      int64
	Assignee        *User `xorm:"-"`
	IsRead          bool  `xorm:"-"`
	IsPull          bool  // Indicates whether is a pull request or not.
	IsClosed        bool
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

func (i *Issue) GetLabels() error {
	if len(i.LabelIds) < 3 {
		return nil
	}

	strIds := strings.Split(strings.TrimSuffix(i.LabelIds[1:], "|"), "|$")
	i.Labels = make([]*Label, 0, len(strIds))
	for _, strId := range strIds {
		id, _ := base.StrTo(strId).Int64()
		if id > 0 {
			l, err := GetLabelById(id)
			if err != nil {
				if err == ErrLabelNotExist {
					continue
				}
				return err
			}
			i.Labels = append(i.Labels, l)
		}
	}
	return nil
}

func (i *Issue) GetAssignee() (err error) {
	if i.AssigneeId == 0 {
		return nil
	}
	i.Assignee, err = GetUserById(i.AssigneeId)
	if err == ErrUserNotExist {
		return nil
	}
	return err
}

// CreateIssue creates new issue for repository.
func NewIssue(issue *Issue) (err error) {
	sess := x.NewSession()
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
	has, err := x.Get(issue)
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
	has, err := x.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist
	}
	return issue, nil
}

// GetIssues returns a list of issues by given conditions.
func GetIssues(uid, rid, pid, mid int64, page int, isClosed bool, labelIds, sortType string) ([]Issue, error) {
	sess := x.Limit(20, (page-1)*20)

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

	if len(labelIds) > 0 {
		for _, label := range strings.Split(labelIds, ",") {
			sess.And("label_ids like '%$" + label + "|%'")
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

type IssueStatus int

const (
	IS_OPEN = iota + 1
	IS_CLOSE
)

// GetIssuesByLabel returns a list of issues by given label and repository.
func GetIssuesByLabel(repoId int64, label string) ([]*Issue, error) {
	issues := make([]*Issue, 0, 10)
	err := x.Where("repo_id=?", repoId).And("label_ids like '%$" + label + "|%'").Find(&issues)
	return issues, err
}

// GetIssueCountByPoster returns number of issues of repository by poster.
func GetIssueCountByPoster(uid, rid int64, isClosed bool) int64 {
	count, _ := x.Where("repo_id=?", rid).And("poster_id=?", uid).And("is_closed=?", isClosed).Count(new(Issue))
	return count
}

// .___                             ____ ___
// |   | ______ ________ __   ____ |    |   \______ ___________
// |   |/  ___//  ___/  |  \_/ __ \|    |   /  ___// __ \_  __ \
// |   |\___ \ \___ \|  |  /\  ___/|    |  /\___ \\  ___/|  | \/
// |___/____  >____  >____/  \___  >______//____  >\___  >__|
//          \/     \/            \/             \/     \/

// IssueUser represents an issue-user relation.
type IssueUser struct {
	Id          int64
	Uid         int64 `xorm:"INDEX"` // User ID.
	IssueId     int64
	RepoId      int64 `xorm:"INDEX"`
	MilestoneId int64
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
		if _, err = x.Insert(iu); err != nil {
			return err
		}
	}
	if isNeedAddPoster {
		iu.Uid = pid
		iu.IsPoster = true
		iu.IsAssigned = iu.Uid == aid
		if _, err = x.Insert(iu); err != nil {
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
	err := x.Where("is_closed=?", isClosed).Find(&ius, &IssueUser{RepoId: rid, Uid: uid})
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
	sess := x.Limit(20, (page-1)*20).Where("is_closed=?", isClosed)
	if len(cond) > 0 {
		sess.And(cond)
	}
	err := sess.Find(&ius)
	return ius, err
}

// GetIssueUserPairsByMode returns issue-user pairs by given repository and user.
func GetIssueUserPairsByMode(uid, rid int64, isClosed bool, page, filterMode int) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	sess := x.Limit(20, (page-1)*20).Where("uid=?", uid).And("is_closed=?", isClosed)
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
	tmpSess := &xorm.Session{}

	sess := x.Where("repo_id=?", rid)
	*tmpSess = *sess
	stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(issue)
	*tmpSess = *sess
	stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(issue)
	if isShowClosed {
		stats.AllCount = stats.ClosedCount
	} else {
		stats.AllCount = stats.OpenCount
	}

	if filterMode != FM_MENTION {
		sess = x.Where("repo_id=?", rid)
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
		sess := x.Where("repo_id=?", rid).And("uid=?", uid).And("is_mentioned=?", true)
		*tmpSess = *sess
		stats.OpenCount, _ = tmpSess.And("is_closed=?", false).Count(new(IssueUser))
		*tmpSess = *sess
		stats.ClosedCount, _ = tmpSess.And("is_closed=?", true).Count(new(IssueUser))
	}
nofilter:
	stats.AssignCount, _ = x.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("assignee_id=?", uid).Count(issue)
	stats.CreateCount, _ = x.Where("repo_id=?", rid).And("is_closed=?", isShowClosed).And("poster_id=?", uid).Count(issue)
	stats.MentionCount, _ = x.Where("repo_id=?", rid).And("uid=?", uid).And("is_closed=?", isShowClosed).And("is_mentioned=?", true).Count(new(IssueUser))
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(uid int64, filterMode int) *IssueStats {
	stats := &IssueStats{}
	issue := new(Issue)
	stats.AssignCount, _ = x.Where("assignee_id=?", uid).And("is_closed=?", false).Count(issue)
	stats.CreateCount, _ = x.Where("poster_id=?", uid).And("is_closed=?", false).Count(issue)
	return stats
}

// UpdateIssue updates information of issue.
func UpdateIssue(issue *Issue) error {
	_, err := x.Id(issue.Id).AllCols().Update(issue)
	return err
}

// UpdateIssueUserByStatus updates issue-user pairs by issue status.
func UpdateIssueUserPairsByStatus(iid int64, isClosed bool) error {
	rawSql := "UPDATE `issue_user` SET is_closed = ? WHERE issue_id = ?"
	_, err := x.Exec(rawSql, isClosed, iid)
	return err
}

// UpdateIssueUserPairByAssignee updates issue-user pair for assigning.
func UpdateIssueUserPairByAssignee(aid, iid int64) error {
	rawSql := "UPDATE `issue_user` SET is_assigned = ? WHERE issue_id = ?"
	if _, err := x.Exec(rawSql, false, iid); err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if aid == 0 {
		return nil
	}
	rawSql = "UPDATE `issue_user` SET is_assigned = true WHERE uid = ? AND issue_id = ?"
	_, err := x.Exec(rawSql, aid, iid)
	return err
}

// UpdateIssueUserPairByRead updates issue-user pair for reading.
func UpdateIssueUserPairByRead(uid, iid int64) error {
	rawSql := "UPDATE `issue_user` SET is_read = ? WHERE uid = ? AND issue_id = ?"
	_, err := x.Exec(rawSql, true, uid, iid)
	return err
}

// UpdateIssueUserPairsByMentions updates issue-user pairs by mentioning.
func UpdateIssueUserPairsByMentions(uids []int64, iid int64) error {
	for _, uid := range uids {
		iu := &IssueUser{Uid: uid, IssueId: iid}
		has, err := x.Get(iu)
		if err != nil {
			return err
		}

		iu.IsMentioned = true
		if has {
			_, err = x.Id(iu.Id).AllCols().Update(iu)
		} else {
			_, err = x.Insert(iu)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// .____          ___.          .__
// |    |   _____ \_ |__   ____ |  |
// |    |   \__  \ | __ \_/ __ \|  |
// |    |___ / __ \| \_\ \  ___/|  |__
// |_______ (____  /___  /\___  >____/
//         \/    \/    \/     \/

// Label represents a label of repository for issues.
type Label struct {
	Id              int64
	RepoId          int64 `xorm:"INDEX"`
	Name            string
	Color           string `xorm:"VARCHAR(7)"`
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `xorm:"-"`
	IsChecked       bool `xorm:"-"`
}

// CalOpenIssues calculates the open issues of label.
func (m *Label) CalOpenIssues() {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues
}

// NewLabel creates new label of repository.
func NewLabel(l *Label) error {
	_, err := x.Insert(l)
	return err
}

// GetLabelById returns a label by given ID.
func GetLabelById(id int64) (*Label, error) {
	if id <= 0 {
		return nil, ErrLabelNotExist
	}

	l := &Label{Id: id}
	has, err := x.Get(l)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLabelNotExist
	}
	return l, nil
}

// GetLabels returns a list of labels of given repository ID.
func GetLabels(repoId int64) ([]*Label, error) {
	labels := make([]*Label, 0, 10)
	err := x.Where("repo_id=?", repoId).Find(&labels)
	return labels, err
}

// UpdateLabel updates label information.
func UpdateLabel(l *Label) error {
	_, err := x.Id(l.Id).Update(l)
	return err
}

// DeleteLabel delete a label of given repository.
func DeleteLabel(repoId int64, strId string) error {
	id, _ := base.StrTo(strId).Int64()
	l, err := GetLabelById(id)
	if err != nil {
		if err == ErrLabelNotExist {
			return nil
		}
		return err
	}

	issues, err := GetIssuesByLabel(repoId, strId)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	for _, issue := range issues {
		issue.LabelIds = strings.Replace(issue.LabelIds, "$"+strId+"|", "", -1)
		if _, err = sess.Id(issue.Id).AllCols().Update(issue); err != nil {
			sess.Rollback()
			return err
		}
	}

	if _, err = sess.Delete(l); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

//    _____  .__.__                   __
//   /     \ |__|  |   ____   _______/  |_  ____   ____   ____
//  /  \ /  \|  |  | _/ __ \ /  ___/\   __\/  _ \ /    \_/ __ \
// /    Y    \  |  |_\  ___/ \___ \  |  | (  <_> )   |  \  ___/
// \____|__  /__|____/\___  >____  > |__|  \____/|___|  /\___  >
//         \/             \/     \/                   \/     \/

// Milestone represents a milestone of repository.
type Milestone struct {
	Id              int64
	RepoId          int64 `xorm:"INDEX"`
	Index           int64
	Name            string
	Content         string
	RenderedContent string `xorm:"-"`
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int `xorm:"-"`
	Completeness    int // Percentage(1-100).
	Deadline        time.Time
	DeadlineString  string `xorm:"-"`
	ClosedDate      time.Time
}

// CalOpenIssues calculates the open issues of milestone.
func (m *Milestone) CalOpenIssues() {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues
}

// NewMilestone creates new milestone of repository.
func NewMilestone(m *Milestone) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(m); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `repository` SET num_milestones = num_milestones + 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, m.RepoId); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// GetMilestoneById returns the milestone by given ID.
func GetMilestoneById(id int64) (*Milestone, error) {
	m := &Milestone{Id: id}
	has, err := x.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMilestoneNotExist
	}
	return m, nil
}

// GetMilestoneByIndex returns the milestone of given repository and index.
func GetMilestoneByIndex(repoId, idx int64) (*Milestone, error) {
	m := &Milestone{RepoId: repoId, Index: idx}
	has, err := x.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMilestoneNotExist
	}
	return m, nil
}

// GetMilestones returns a list of milestones of given repository and status.
func GetMilestones(repoId int64, isClosed bool) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, 10)
	err := x.Where("repo_id=?", repoId).And("is_closed=?", isClosed).Find(&miles)
	return miles, err
}

// UpdateMilestone updates information of given milestone.
func UpdateMilestone(m *Milestone) error {
	_, err := x.Id(m.Id).Update(m)
	return err
}

// ChangeMilestoneStatus changes the milestone open/closed status.
func ChangeMilestoneStatus(m *Milestone, isClosed bool) (err error) {
	repo, err := GetRepositoryById(m.RepoId)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	m.IsClosed = isClosed
	if _, err = sess.Id(m.Id).AllCols().Update(m); err != nil {
		sess.Rollback()
		return err
	}

	if isClosed {
		repo.NumClosedMilestones++
	} else {
		repo.NumClosedMilestones--
	}
	if _, err = sess.Id(repo.Id).Update(repo); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// ChangeMilestoneAssign changes assignment of milestone for issue.
func ChangeMilestoneAssign(oldMid, mid int64, issue *Issue) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if oldMid > 0 {
		m, err := GetMilestoneById(oldMid)
		if err != nil {
			return err
		}

		m.NumIssues--
		if issue.IsClosed {
			m.NumClosedIssues--
		}
		if m.NumIssues > 0 {
			m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
		} else {
			m.Completeness = 0
		}
		if _, err = sess.Id(m.Id).Update(m); err != nil {
			sess.Rollback()
			return err
		}

		rawSql := "UPDATE `issue_user` SET milestone_id = 0 WHERE issue_id = ?"
		if _, err = sess.Exec(rawSql, issue.Id); err != nil {
			sess.Rollback()
			return err
		}
	}

	if mid > 0 {
		m, err := GetMilestoneById(mid)
		if err != nil {
			return err
		}
		m.NumIssues++
		if issue.IsClosed {
			m.NumClosedIssues++
		}
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
		if _, err = sess.Id(m.Id).Update(m); err != nil {
			sess.Rollback()
			return err
		}

		rawSql := "UPDATE `issue_user` SET milestone_id = ? WHERE issue_id = ?"
		if _, err = sess.Exec(rawSql, m.Id, issue.Id); err != nil {
			sess.Rollback()
			return err
		}
	}
	return sess.Commit()
}

// DeleteMilestone deletes a milestone.
func DeleteMilestone(m *Milestone) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Delete(m); err != nil {
		sess.Rollback()
		return err
	}

	rawSql := "UPDATE `repository` SET num_milestones = num_milestones - 1 WHERE id = ?"
	if _, err = sess.Exec(rawSql, m.RepoId); err != nil {
		sess.Rollback()
		return err
	}

	rawSql = "UPDATE `issue` SET milestone_id = 0 WHERE milestone_id = ?"
	if _, err = sess.Exec(rawSql, m.Id); err != nil {
		sess.Rollback()
		return err
	}

	rawSql = "UPDATE `issue_user` SET milestone_id = 0 WHERE milestone_id = ?"
	if _, err = sess.Exec(rawSql, m.Id); err != nil {
		sess.Rollback()
		return err
	}
	return sess.Commit()
}

// _________                                       __
// \_   ___ \  ____   _____   _____   ____   _____/  |_
// /    \  \/ /  _ \ /     \ /     \_/ __ \ /    \   __\
// \     \___(  <_> )  Y Y  \  Y Y  \  ___/|   |  \  |
//  \______  /\____/|__|_|  /__|_|  /\___  >___|  /__|
//         \/             \/      \/     \/     \/

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
	Content  string    `xorm:"TEXT"`
	Created  time.Time `xorm:"CREATED"`
}

// CreateComment creates comment of issue or commit.
func CreateComment(userId, repoId, issueId, commitId, line int64, cmtType int, content string) error {
	sess := x.NewSession()
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
	err := x.Asc("created").Find(&comments, &Comment{IssueId: issueId})
	return comments, err
}
