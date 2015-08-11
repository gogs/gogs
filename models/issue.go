// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime/multipart"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
	gouuid "github.com/gogits/gogs/modules/uuid"
)

var (
	ErrIssueNotExist       = errors.New("Issue does not exist")
	ErrWrongIssueCounter   = errors.New("Invalid number of issues for this milestone")
	ErrAttachmentNotLinked = errors.New("Attachment does not belong to this issue")
	ErrMissingIssueNumber  = errors.New("No issue number specified")
)

// Issue represents an issue or pull request of repository.
type Issue struct {
	ID              int64 `xorm:"pk autoincr"`
	RepoID          int64 `xorm:"INDEX"`
	Index           int64 // Index in one repository.
	Name            string
	Repo            *Repository `xorm:"-"`
	PosterID        int64
	Poster          *User    `xorm:"-"`
	Labels          []*Label `xorm:"-"`
	MilestoneID     int64
	Milestone       *Milestone `xorm:"-"`
	AssigneeID      int64
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

func (i *Issue) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "milestone_id":
		if i.MilestoneID == 0 {
			return
		}

		i.Milestone, err = GetMilestoneByID(i.MilestoneID)
		if err != nil {
			log.Error(3, "GetMilestoneById: %v", err)
		}
	case "assignee_id":
		if i.AssigneeID == 0 {
			return
		}

		i.Assignee, err = GetUserByID(i.AssigneeID)
		if err != nil {
			log.Error(3, "GetUserByID: %v", err)
		}
	}
}

func (i *Issue) GetPoster() (err error) {
	i.Poster, err = GetUserByID(i.PosterID)
	if IsErrUserNotExist(err) {
		i.Poster = &User{Name: "FakeUser"}
		return nil
	}
	return err
}

func (i *Issue) hasLabel(e Engine, labelID int64) bool {
	return hasIssueLabel(e, i.ID, labelID)
}

// HasLabel returns true if issue has been labeled by given ID.
func (i *Issue) HasLabel(labelID int64) bool {
	return i.hasLabel(x, labelID)
}

func (i *Issue) addLabel(e Engine, labelID int64) error {
	return newIssueLabel(e, i.ID, labelID)
}

// AddLabel adds new label to issue by given ID.
func (i *Issue) AddLabel(labelID int64) error {
	return i.addLabel(x, labelID)
}

func (i *Issue) getLabels(e Engine) (err error) {
	if len(i.Labels) > 0 {
		return nil
	}

	i.Labels, err = getLabelsByIssueID(e, i.ID)
	if err != nil {
		return fmt.Errorf("getLabelsByIssueID: %v", err)
	}
	return nil
}

// GetLabels retrieves all labels of issue and assign to corresponding field.
func (i *Issue) GetLabels() error {
	return i.getLabels(x)
}

func (i *Issue) removeLabel(e Engine, labelID int64) error {
	return deleteIssueLabel(e, i.ID, labelID)
}

// RemoveLabel removes a label from issue by given ID.
func (i *Issue) RemoveLabel(labelID int64) error {
	return i.removeLabel(x, labelID)
}

func (i *Issue) GetAssignee() (err error) {
	if i.AssigneeID == 0 || i.Assignee != nil {
		return nil
	}

	i.Assignee, err = GetUserByID(i.AssigneeID)
	if IsErrUserNotExist(err) {
		return nil
	}
	return err
}

func (i *Issue) Attachments() []*Attachment {
	a, _ := GetAttachmentsForIssue(i.ID)
	return a
}

func (i *Issue) AfterDelete() {
	_, err := DeleteAttachmentsByIssue(i.ID, true)

	if err != nil {
		log.Info("Could not delete files for issue #%d: %s", i.ID, err)
	}
}

// CreateIssue creates new issue with labels for repository.
func NewIssue(repo *Repository, issue *Issue, labelIDs []int64, uuids []string) (err error) {
	// Check attachments.
	attachments := make([]*Attachment, 0, len(uuids))
	for _, uuid := range uuids {
		attach, err := GetAttachmentByUUID(uuid)
		if err != nil {
			if IsErrAttachmentNotExist(err) {
				continue
			}
			return fmt.Errorf("GetAttachmentByUUID[%s]: %v", uuid, err)
		}
		attachments = append(attachments, attach)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(issue); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `repository` SET num_issues=num_issues+1 WHERE id=?", issue.RepoID); err != nil {
		return err
	}

	for _, id := range labelIDs {
		if err = issue.addLabel(sess, id); err != nil {
			return fmt.Errorf("addLabel: %v", err)
		}
	}

	if issue.MilestoneID > 0 {
		if err = changeMilestoneAssign(sess, 0, issue); err != nil {
			return err
		}
	}

	if err = newIssueUsers(sess, repo, issue); err != nil {
		return err
	}

	for i := range attachments {
		attachments[i].IssueID = issue.ID
		// No assign value could be 0, so ignore AllCols().
		if _, err = sess.Id(attachments[i].ID).Update(attachments[i]); err != nil {
			return fmt.Errorf("update attachment[%d]: %v", attachments[i].ID, err)
		}
	}

	// Notify watchers.
	act := &Action{
		ActUserID:    issue.Poster.Id,
		ActUserName:  issue.Poster.Name,
		ActEmail:     issue.Poster.Email,
		OpType:       CREATE_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Name),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}
	if err = notifyWatchers(sess, act); err != nil {
		return err
	}

	return sess.Commit()
}

// GetIssueByRef returns an Issue specified by a GFM reference.
// See https://help.github.com/articles/writing-on-github#references for more information on the syntax.
func GetIssueByRef(ref string) (issue *Issue, err error) {
	var issueNumber int64
	var repo *Repository

	n := strings.IndexByte(ref, byte('#'))

	if n == -1 {
		return nil, ErrMissingIssueNumber
	}

	if issueNumber, err = strconv.ParseInt(ref[n+1:], 10, 64); err != nil {
		return
	}

	if repo, err = GetRepositoryByRef(ref[:n]); err != nil {
		return
	}

	return GetIssueByIndex(repo.ID, issueNumber)
}

// GetIssueByIndex returns issue by given index in repository.
func GetIssueByIndex(rid, index int64) (*Issue, error) {
	issue := &Issue{RepoID: rid, Index: index}
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
	issue := &Issue{ID: id}
	has, err := x.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist
	}
	return issue, nil
}

// Issues returns a list of issues by given conditions.
func Issues(uid, assigneeID, repoID, posterID, milestoneID int64, page int, isClosed, isMention bool, labels, sortType string) ([]*Issue, error) {
	sess := x.Limit(setting.IssuePagingNum, (page-1)*setting.IssuePagingNum)

	if repoID > 0 {
		sess.Where("issue.repo_id=?", repoID).And("issue.is_closed=?", isClosed)
	} else {
		sess.Where("issue.is_closed=?", isClosed)
	}

	if assigneeID > 0 {
		sess.And("issue.assignee_id=?", assigneeID)
	} else if posterID > 0 {
		sess.And("issue.poster_id=?", posterID)
	}

	if milestoneID > 0 {
		sess.And("issue.milestone_id=?", milestoneID)
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

	labelIDs := base.StringsToInt64s(strings.Split(labels, ","))
	if len(labelIDs) > 0 {
		validJoin := false
		queryStr := "issue.id=issue_label.issue_id"
		for _, id := range labelIDs {
			if id == 0 {
				continue
			}
			validJoin = true
			queryStr += " AND issue_label.label_id=" + com.ToStr(id)
		}
		if validJoin {
			sess.Join("INNER", "issue_label", queryStr)
		}
	}

	if isMention {
		queryStr := "issue.id=issue_user.issue_id AND issue_user.is_mentioned=1"
		if uid > 0 {
			queryStr += " AND issue_user.uid=" + com.ToStr(uid)
		}
		sess.Join("INNER", "issue_user", queryStr)
	}

	issues := make([]*Issue, 0, setting.IssuePagingNum)
	return issues, sess.Find(&issues)
}

type IssueStatus int

const (
	IS_OPEN = iota + 1
	IS_CLOSE
)

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
	ID          int64 `xorm:"pk autoincr"`
	UID         int64 `xorm:"uid INDEX"` // User ID.
	IssueID     int64
	RepoID      int64 `xorm:"INDEX"`
	MilestoneID int64
	IsRead      bool
	IsAssigned  bool
	IsMentioned bool
	IsPoster    bool
	IsClosed    bool
}

func newIssueUsers(e *xorm.Session, repo *Repository, issue *Issue) error {
	users, err := repo.GetAssignees()
	if err != nil {
		return err
	}

	iu := &IssueUser{
		IssueID: issue.ID,
		RepoID:  repo.ID,
	}

	// Poster can be anyone.
	isNeedAddPoster := true
	for _, u := range users {
		iu.ID = 0
		iu.UID = u.Id
		iu.IsPoster = iu.UID == issue.PosterID
		if isNeedAddPoster && iu.IsPoster {
			isNeedAddPoster = false
		}
		iu.IsAssigned = iu.UID == issue.AssigneeID
		if _, err = e.Insert(iu); err != nil {
			return err
		}
	}
	if isNeedAddPoster {
		iu.ID = 0
		iu.UID = issue.PosterID
		iu.IsPoster = true
		if _, err = e.Insert(iu); err != nil {
			return err
		}
	}
	return nil
}

// NewIssueUsers adds new issue-user relations for new issue of repository.
func NewIssueUsers(repo *Repository, issue *Issue) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssueUsers(sess, repo, issue); err != nil {
		return err
	}

	return sess.Commit()
}

// PairsContains returns true when pairs list contains given issue.
func PairsContains(ius []*IssueUser, issueId, uid int64) int {
	for i := range ius {
		if ius[i].IssueID == issueId &&
			ius[i].UID == uid {
			return i
		}
	}
	return -1
}

// GetIssueUsers returns issue-user pairs by given repository and user.
func GetIssueUsers(rid, uid int64, isClosed bool) ([]*IssueUser, error) {
	ius := make([]*IssueUser, 0, 10)
	err := x.Where("is_closed=?", isClosed).Find(&ius, &IssueUser{RepoID: rid, UID: uid})
	return ius, err
}

// GetIssueUserPairsByRepoIds returns issue-user pairs by given repository IDs.
func GetIssueUserPairsByRepoIds(rids []int64, isClosed bool, page int) ([]*IssueUser, error) {
	if len(rids) == 0 {
		return []*IssueUser{}, nil
	}

	buf := bytes.NewBufferString("")
	for _, rid := range rids {
		buf.WriteString("repo_id=")
		buf.WriteString(com.ToStr(rid))
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
	FM_ALL = iota
	FM_ASSIGN
	FM_CREATE
	FM_MENTION
)

func parseCountResult(results []map[string][]byte) int64 {
	if len(results) == 0 {
		return 0
	}
	for _, result := range results[0] {
		return com.StrTo(string(result)).MustInt64()
	}
	return 0
}

// GetIssueStats returns issue statistic information by given conditions.
func GetIssueStats(repoID, uid, labelID, milestoneID int64, isShowClosed bool, filterMode int) *IssueStats {
	stats := &IssueStats{}
	// issue := new(Issue)

	queryStr := "SELECT COUNT(*) FROM `issue` "
	if labelID > 0 {
		queryStr += "INNER JOIN `issue_label` ON `issue`.id=`issue_label`.issue_id AND `issue_label`.label_id=" + com.ToStr(labelID)
	}

	baseCond := " WHERE issue.repo_id=? AND issue.is_closed=?"
	if milestoneID > 0 {
		baseCond += " AND issue.milestone_id=" + com.ToStr(milestoneID)
	}
	switch filterMode {
	case FM_ALL:
		resutls, _ := x.Query(queryStr+baseCond, repoID, false)
		stats.OpenCount = parseCountResult(resutls)
		resutls, _ = x.Query(queryStr+baseCond, repoID, true)
		stats.ClosedCount = parseCountResult(resutls)

	case FM_ASSIGN:
		baseCond += " AND assignee_id=?"
		resutls, _ := x.Query(queryStr+baseCond, repoID, false, uid)
		stats.OpenCount = parseCountResult(resutls)
		resutls, _ = x.Query(queryStr+baseCond, repoID, true, uid)
		stats.ClosedCount = parseCountResult(resutls)

	case FM_CREATE:
		baseCond += " AND poster_id=?"
		resutls, _ := x.Query(queryStr+baseCond, repoID, false, uid)
		stats.OpenCount = parseCountResult(resutls)
		resutls, _ = x.Query(queryStr+baseCond, repoID, true, uid)
		stats.ClosedCount = parseCountResult(resutls)

	case FM_MENTION:
		queryStr += " INNER JOIN `issue_user` ON `issue`.id=`issue_user`.issue_id"
		baseCond += " AND `issue_user`.uid=? AND `issue_user`.is_mentioned=?"
		resutls, _ := x.Query(queryStr+baseCond, repoID, false, uid, true)
		stats.OpenCount = parseCountResult(resutls)
		resutls, _ = x.Query(queryStr+baseCond, repoID, true, uid, true)
		stats.ClosedCount = parseCountResult(resutls)
	}
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

func updateIssue(e Engine, issue *Issue) error {
	_, err := e.Id(issue.ID).AllCols().Update(issue)
	return err
}

// UpdateIssue updates information of issue.
func UpdateIssue(issue *Issue) error {
	return updateIssue(x, issue)
}

// UpdateIssueUserByStatus updates issue-user pairs by issue status.
func UpdateIssueUserPairsByStatus(iid int64, isClosed bool) error {
	rawSql := "UPDATE `issue_user` SET is_closed = ? WHERE issue_id = ?"
	_, err := x.Exec(rawSql, isClosed, iid)
	return err
}

func updateIssueUserByAssignee(e *xorm.Session, issueID, assigneeID int64) (err error) {
	if _, err = e.Exec("UPDATE `issue_user` SET is_assigned=? WHERE issue_id=?", false, issueID); err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if assigneeID == 0 {
		return nil
	}
	_, err = e.Exec("UPDATE `issue_user` SET is_assigned=? WHERE uid=? AND issue_id=?", true, assigneeID, issueID)
	return err
}

// UpdateIssueUserByAssignee updates issue-user relation for assignee.
func UpdateIssueUserByAssignee(issueID, assigneeID int64) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = updateIssueUserByAssignee(sess, issueID, assigneeID); err != nil {
		return err
	}

	return sess.Commit()
}

// UpdateIssueUserPairByRead updates issue-user pair for reading.
func UpdateIssueUserPairByRead(uid, iid int64) error {
	rawSql := "UPDATE `issue_user` SET is_read = ? WHERE uid = ? AND issue_id = ?"
	_, err := x.Exec(rawSql, true, uid, iid)
	return err
}

// UpdateIssueUsersByMentions updates issue-user pairs by mentioning.
func UpdateIssueUsersByMentions(uids []int64, iid int64) error {
	for _, uid := range uids {
		iu := &IssueUser{UID: uid, IssueID: iid}
		has, err := x.Get(iu)
		if err != nil {
			return err
		}

		iu.IsMentioned = true
		if has {
			_, err = x.Id(iu.ID).AllCols().Update(iu)
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
	ID              int64 `xorm:"pk autoincr"`
	RepoID          int64 `xorm:"INDEX"`
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

func getLabelByID(e Engine, id int64) (*Label, error) {
	if id <= 0 {
		return nil, ErrLabelNotExist{id}
	}

	l := &Label{ID: id}
	has, err := x.Get(l)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrLabelNotExist{l.ID}
	}
	return l, nil
}

// GetLabelByID returns a label by given ID.
func GetLabelByID(id int64) (*Label, error) {
	return getLabelByID(x, id)
}

// GetLabelsByRepoID returns all labels that belong to given repository by ID.
func GetLabelsByRepoID(repoID int64) ([]*Label, error) {
	labels := make([]*Label, 0, 10)
	return labels, x.Where("repo_id=?", repoID).Find(&labels)
}

func getLabelsByIssueID(e Engine, issueID int64) ([]*Label, error) {
	issueLabels, err := getIssueLabels(e, issueID)
	if err != nil {
		return nil, fmt.Errorf("getIssueLabels: %v", err)
	}

	var label *Label
	labels := make([]*Label, 0, len(issueLabels))
	for idx := range issueLabels {
		label, err = getLabelByID(e, issueLabels[idx].LabelID)
		if err != nil && !IsErrLabelNotExist(err) {
			return nil, fmt.Errorf("getLabelByID: %v", err)
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// GetLabelsByIssueID returns all labels that belong to given issue by ID.
func GetLabelsByIssueID(issueID int64) ([]*Label, error) {
	return getLabelsByIssueID(x, issueID)
}

// UpdateLabel updates label information.
func UpdateLabel(l *Label) error {
	_, err := x.Id(l.ID).AllCols().Update(l)
	return err
}

// DeleteLabel delete a label of given repository.
func DeleteLabel(repoID, labelID int64) error {
	l, err := GetLabelByID(labelID)
	if err != nil {
		if IsErrLabelNotExist(err) {
			return nil
		}
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = x.Where("label_id=?", labelID).Delete(new(IssueLabel)); err != nil {
		return err
	} else if _, err = sess.Delete(l); err != nil {
		return err
	}
	return sess.Commit()
}

// .___                            .____          ___.          .__
// |   | ______ ________ __   ____ |    |   _____ \_ |__   ____ |  |
// |   |/  ___//  ___/  |  \_/ __ \|    |   \__  \ | __ \_/ __ \|  |
// |   |\___ \ \___ \|  |  /\  ___/|    |___ / __ \| \_\ \  ___/|  |__
// |___/____  >____  >____/  \___  >_______ (____  /___  /\___  >____/
//          \/     \/            \/        \/    \/    \/     \/

// IssueLabel represetns an issue-lable relation.
type IssueLabel struct {
	ID      int64 `xorm:"pk autoincr"`
	IssueID int64 `xorm:"UNIQUE(s)"`
	LabelID int64 `xorm:"UNIQUE(s)"`
}

func hasIssueLabel(e Engine, issueID, labelID int64) bool {
	has, _ := e.Where("issue_id=? AND label_id=?", issueID, labelID).Get(new(IssueLabel))
	return has
}

// HasIssueLabel returns true if issue has been labeled.
func HasIssueLabel(issueID, labelID int64) bool {
	return hasIssueLabel(x, issueID, labelID)
}

func newIssueLabel(e Engine, issueID, labelID int64) error {
	if issueID == 0 || labelID == 0 {
		return nil
	}

	_, err := e.Insert(&IssueLabel{
		IssueID: issueID,
		LabelID: labelID,
	})
	return err
}

// NewIssueLabel creates a new issue-label relation.
func NewIssueLabel(issueID, labelID int64) error {
	return newIssueLabel(x, issueID, labelID)
}

func getIssueLabels(e Engine, issueID int64) ([]*IssueLabel, error) {
	issueLabels := make([]*IssueLabel, 0, 10)
	return issueLabels, e.Where("issue_id=?", issueID).Asc("label_id").Find(&issueLabels)
}

// GetIssueLabels returns all issue-label relations of given issue by ID.
func GetIssueLabels(issueID int64) ([]*IssueLabel, error) {
	return getIssueLabels(x, issueID)
}

func deleteIssueLabel(e Engine, issueID, labelID int64) error {
	_, err := e.Delete(&IssueLabel{
		IssueID: issueID,
		LabelID: labelID,
	})
	return err
}

// DeleteIssueLabel deletes issue-label relation.
func DeleteIssueLabel(issueID, labelID int64) error {
	return deleteIssueLabel(x, issueID, labelID)
}

//    _____  .__.__                   __
//   /     \ |__|  |   ____   _______/  |_  ____   ____   ____
//  /  \ /  \|  |  | _/ __ \ /  ___/\   __\/  _ \ /    \_/ __ \
// /    Y    \  |  |_\  ___/ \___ \  |  | (  <_> )   |  \  ___/
// \____|__  /__|____/\___  >____  > |__|  \____/|___|  /\___  >
//         \/             \/     \/                   \/     \/

// Milestone represents a milestone of repository.
type Milestone struct {
	ID              int64 `xorm:"pk autoincr"`
	RepoID          int64 `xorm:"INDEX"`
	Name            string
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-"`
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int `xorm:"-"`
	Completeness    int // Percentage(1-100).
	Deadline        time.Time
	DeadlineString  string `xorm:"-"`
	IsOverDue       bool   `xorm:"-"`
	ClosedDate      time.Time
}

func (m *Milestone) BeforeUpdate() {
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	} else {
		m.Completeness = 0
	}
}

func (m *Milestone) AfterSet(colName string, _ xorm.Cell) {
	if colName == "deadline" {
		if m.Deadline.Year() == 9999 {
			return
		}

		m.DeadlineString = m.Deadline.Format("2006-01-02")
		if time.Now().After(m.Deadline) {
			m.IsOverDue = true
		}
	}
}

// CalOpenIssues calculates the open issues of milestone.
func (m *Milestone) CalOpenIssues() {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues
}

// NewMilestone creates new milestone of repository.
func NewMilestone(m *Milestone) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(m); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `repository` SET num_milestones=num_milestones+1 WHERE id=?", m.RepoID); err != nil {
		return err
	}
	return sess.Commit()
}

func getMilestoneByID(e Engine, id int64) (*Milestone, error) {
	m := &Milestone{ID: id}
	has, err := x.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMilestoneNotExist{id, 0}
	}
	return m, nil
}

// GetMilestoneByID returns the milestone of given ID.
func GetMilestoneByID(id int64) (*Milestone, error) {
	return getMilestoneByID(x, id)
}

// GetRepoMilestoneByID returns the milestone of given ID and repository.
func GetRepoMilestoneByID(repoID, milestoneID int64) (*Milestone, error) {
	m := &Milestone{ID: milestoneID, RepoID: repoID}
	has, err := x.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMilestoneNotExist{milestoneID, repoID}
	}
	return m, nil
}

// GetAllRepoMilestones returns all milestones of given repository.
func GetAllRepoMilestones(repoID int64) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, 10)
	return miles, x.Where("repo_id=?", repoID).Find(&miles)
}

// GetMilestones returns a list of milestones of given repository and status.
func GetMilestones(repoID int64, page int, isClosed bool) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, setting.IssuePagingNum)
	sess := x.Where("repo_id=? AND is_closed=?", repoID, isClosed)
	if page > 0 {
		sess = sess.Limit(setting.IssuePagingNum, (page-1)*setting.IssuePagingNum)
	}
	return miles, sess.Find(&miles)
}

func updateMilestone(e Engine, m *Milestone) error {
	_, err := e.Id(m.ID).AllCols().Update(m)
	return err
}

// UpdateMilestone updates information of given milestone.
func UpdateMilestone(m *Milestone) error {
	return updateMilestone(x, m)
}

func countRepoMilestones(e Engine, repoID int64) int64 {
	count, _ := e.Where("repo_id=?", repoID).Count(new(Milestone))
	return count
}

// CountRepoMilestones returns number of milestones in given repository.
func CountRepoMilestones(repoID int64) int64 {
	return countRepoMilestones(x, repoID)
}

func countRepoClosedMilestones(e Engine, repoID int64) int64 {
	closed, _ := e.Where("repo_id=? AND is_closed=?", repoID, true).Count(new(Milestone))
	return closed
}

// CountRepoClosedMilestones returns number of closed milestones in given repository.
func CountRepoClosedMilestones(repoID int64) int64 {
	return countRepoClosedMilestones(x, repoID)
}

// MilestoneStats returns number of open and closed milestones of given repository.
func MilestoneStats(repoID int64) (open int64, closed int64) {
	open, _ = x.Where("repo_id=? AND is_closed=?", repoID, false).Count(new(Milestone))
	return open, CountRepoClosedMilestones(repoID)
}

// ChangeMilestoneStatus changes the milestone open/closed status.
func ChangeMilestoneStatus(m *Milestone, isClosed bool) (err error) {
	repo, err := GetRepositoryByID(m.RepoID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	m.IsClosed = isClosed
	if err = updateMilestone(sess, m); err != nil {
		return err
	}

	repo.NumMilestones = int(countRepoMilestones(sess, repo.ID))
	repo.NumClosedMilestones = int(countRepoClosedMilestones(sess, repo.ID))
	if _, err = sess.Id(repo.ID).AllCols().Update(repo); err != nil {
		return err
	}
	return sess.Commit()
}

// ChangeMilestoneIssueStats updates the open/closed issues counter and progress
// for the milestone associated witht the given issue.
func ChangeMilestoneIssueStats(issue *Issue) error {
	if issue.MilestoneID == 0 {
		return nil
	}

	m, err := GetMilestoneByID(issue.MilestoneID)
	if err != nil {
		return err
	}

	if issue.IsClosed {
		m.NumOpenIssues--
		m.NumClosedIssues++
	} else {
		m.NumOpenIssues++
		m.NumClosedIssues--
	}

	return UpdateMilestone(m)
}

func changeMilestoneAssign(e *xorm.Session, oldMid int64, issue *Issue) error {
	if oldMid > 0 {
		m, err := getMilestoneByID(e, oldMid)
		if err != nil {
			return err
		}

		m.NumIssues--
		if issue.IsClosed {
			m.NumClosedIssues--
		}

		if err = updateMilestone(e, m); err != nil {
			return err
		} else if _, err = e.Exec("UPDATE `issue_user` SET milestone_id=0 WHERE issue_id=?", issue.ID); err != nil {
			return err
		}
	}

	if issue.MilestoneID > 0 {
		m, err := GetMilestoneByID(issue.MilestoneID)
		if err != nil {
			return err
		}

		m.NumIssues++
		if issue.IsClosed {
			m.NumClosedIssues++
		}

		if m.NumIssues == 0 {
			return ErrWrongIssueCounter
		}

		if err = updateMilestone(e, m); err != nil {
			return err
		} else if _, err = e.Exec("UPDATE `issue_user` SET milestone_id=? WHERE issue_id=?", m.ID, issue.ID); err != nil {
			return err
		}
	}

	return nil
}

// ChangeMilestoneAssign changes assignment of milestone for issue.
func ChangeMilestoneAssign(oldMid int64, issue *Issue) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = changeMilestoneAssign(sess, oldMid, issue); err != nil {
		return err
	}

	return sess.Commit()
}

// DeleteMilestoneByID deletes a milestone by given ID.
func DeleteMilestoneByID(mid int64) error {
	m, err := GetMilestoneByID(mid)
	if err != nil {
		if IsErrMilestoneNotExist(err) {
			return nil
		}
		return err
	}

	repo, err := GetRepositoryByID(m.RepoID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Id(m.ID).Delete(m); err != nil {
		return err
	}

	repo.NumMilestones = int(countRepoMilestones(sess, repo.ID))
	repo.NumClosedMilestones = int(countRepoClosedMilestones(sess, repo.ID))
	if _, err = sess.Id(repo.ID).AllCols().Update(repo); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `issue` SET milestone_id=0 WHERE milestone_id=?", m.ID); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `issue_user` SET milestone_id=0 WHERE milestone_id=?", m.ID); err != nil {
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

// CommentType defines whether a comment is just a simple comment, an action (like close) or a reference.
type CommentType int

const (
	// Plain comment, can be associated with a commit (CommitId > 0) and a line (Line > 0)
	COMMENT_TYPE_COMMENT CommentType = iota
	COMMENT_TYPE_REOPEN
	COMMENT_TYPE_CLOSE

	// References.
	COMMENT_TYPE_ISSUE
	// Reference from some commit (not part of a pull request)
	COMMENT_TYPE_COMMIT
	// Reference from some pull request
	COMMENT_TYPE_PULL
)

// Comment represents a comment in commit and issue page.
type Comment struct {
	Id       int64
	Type     CommentType
	PosterId int64
	Poster   *User `xorm:"-"`
	IssueId  int64
	CommitId int64
	Line     int64
	Content  string    `xorm:"TEXT"`
	Created  time.Time `xorm:"CREATED"`
}

// CreateComment creates comment of issue or commit.
func CreateComment(userId, repoId, issueId, commitId, line int64, cmtType CommentType, content string, attachments []int64) (*Comment, error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err := sess.Begin(); err != nil {
		return nil, err
	}

	comment := &Comment{PosterId: userId, Type: cmtType, IssueId: issueId,
		CommitId: commitId, Line: line, Content: content}

	if _, err := sess.Insert(comment); err != nil {
		return nil, err
	}

	// Check comment type.
	switch cmtType {
	case COMMENT_TYPE_COMMENT:
		rawSql := "UPDATE `issue` SET num_comments = num_comments + 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, issueId); err != nil {
			return nil, err
		}

		if len(attachments) > 0 {
			rawSql = "UPDATE `attachment` SET comment_id = ? WHERE id IN (?)"

			astrs := make([]string, 0, len(attachments))

			for _, a := range attachments {
				astrs = append(astrs, strconv.FormatInt(a, 10))
			}

			if _, err := sess.Exec(rawSql, comment.Id, strings.Join(astrs, ",")); err != nil {
				return nil, err
			}
		}
	case COMMENT_TYPE_REOPEN:
		rawSql := "UPDATE `repository` SET num_closed_issues = num_closed_issues - 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, repoId); err != nil {
			return nil, err
		}
	case COMMENT_TYPE_CLOSE:
		rawSql := "UPDATE `repository` SET num_closed_issues = num_closed_issues + 1 WHERE id = ?"
		if _, err := sess.Exec(rawSql, repoId); err != nil {
			return nil, err
		}
	}

	return comment, sess.Commit()
}

// GetCommentById returns the comment with the given id
func GetCommentById(commentId int64) (*Comment, error) {
	c := &Comment{Id: commentId}
	_, err := x.Get(c)

	return c, err
}

func (c *Comment) ContentHtml() template.HTML {
	return template.HTML(c.Content)
}

// GetIssueComments returns list of comment by given issue id.
func GetIssueComments(issueId int64) ([]Comment, error) {
	comments := make([]Comment, 0, 10)
	err := x.Asc("created").Find(&comments, &Comment{IssueId: issueId})
	return comments, err
}

// Attachments returns the attachments for this comment.
func (c *Comment) Attachments() []*Attachment {
	a, _ := GetAttachmentsByComment(c.Id)
	return a
}

func (c *Comment) AfterDelete() {
	_, err := DeleteAttachmentsByComment(c.Id, true)

	if err != nil {
		log.Info("Could not delete files for comment %d on issue #%d: %s", c.Id, c.IssueId, err)
	}
}

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID        int64  `xorm:"pk autoincr"`
	UUID      string `xorm:"uuid UNIQUE"`
	IssueID   int64  `xorm:"INDEX"`
	CommentID int64
	ReleaseID int64 `xorm:"INDEX"`
	Name      string
	Created   time.Time `xorm:"CREATED"`
}

// AttachmentLocalPath returns where attachment is stored in local file system based on given UUID.
func AttachmentLocalPath(uuid string) string {
	return path.Join(setting.AttachmentPath, uuid[0:1], uuid[1:2], uuid)
}

// LocalPath returns where attachment is stored in local file system.
func (attach *Attachment) LocalPath() string {
	return AttachmentLocalPath(attach.UUID)
}

// NewAttachment creates a new attachment object.
func NewAttachment(name string, buf []byte, file multipart.File) (_ *Attachment, err error) {
	attach := &Attachment{
		UUID: gouuid.NewV4().String(),
		Name: name,
	}

	if err = os.MkdirAll(path.Dir(attach.LocalPath()), os.ModePerm); err != nil {
		return nil, fmt.Errorf("MkdirAll: %v", err)
	}

	fw, err := os.Create(attach.LocalPath())
	if err != nil {
		return nil, fmt.Errorf("Create: %v", err)
	}
	defer fw.Close()

	if _, err = fw.Write(buf); err != nil {
		return nil, fmt.Errorf("Write: %v", err)
	} else if _, err = io.Copy(fw, file); err != nil {
		return nil, fmt.Errorf("Copy: %v", err)
	}

	sess := x.NewSession()
	defer sessionRelease(sess)
	if err := sess.Begin(); err != nil {
		return nil, err
	}

	if _, err := sess.Insert(attach); err != nil {
		return nil, err
	}

	return attach, sess.Commit()
}

// GetAttachmentByUUID returns attachment by given UUID.
func GetAttachmentByUUID(uuid string) (*Attachment, error) {
	attach := &Attachment{UUID: uuid}
	has, err := x.Get(attach)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAttachmentNotExist{0, uuid}
	}
	return attach, nil
}

func GetAttachmentsForIssue(issueId int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	err := x.Where("issue_id = ?", issueId).And("comment_id = 0").Find(&attachments)
	return attachments, err
}

// GetAttachmentsByIssue returns a list of attachments for the given issue
func GetAttachmentsByIssue(issueId int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	err := x.Where("issue_id = ?", issueId).And("comment_id > 0").Find(&attachments)
	return attachments, err
}

// GetAttachmentsByComment returns a list of attachments for the given comment
func GetAttachmentsByComment(commentId int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	err := x.Where("comment_id = ?", commentId).Find(&attachments)
	return attachments, err
}

// DeleteAttachment deletes the given attachment and optionally the associated file.
func DeleteAttachment(a *Attachment, remove bool) error {
	_, err := DeleteAttachments([]*Attachment{a}, remove)
	return err
}

// DeleteAttachments deletes the given attachments and optionally the associated files.
func DeleteAttachments(attachments []*Attachment, remove bool) (int, error) {
	for i, a := range attachments {
		if remove {
			if err := os.Remove(a.LocalPath()); err != nil {
				return i, err
			}
		}

		if _, err := x.Delete(a.ID); err != nil {
			return i, err
		}
	}

	return len(attachments), nil
}

// DeleteAttachmentsByIssue deletes all attachments associated with the given issue.
func DeleteAttachmentsByIssue(issueId int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByIssue(issueId)

	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}

// DeleteAttachmentsByComment deletes all attachments associated with the given comment.
func DeleteAttachmentsByComment(commentId int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByComment(commentId)

	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}
