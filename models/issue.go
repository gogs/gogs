// Copyright 2014 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Unknwon/com"
	"github.com/go-xorm/xorm"
	gouuid "github.com/satori/go.uuid"

	"github.com/gogits/gogs/modules/base"
	"github.com/gogits/gogs/modules/log"
	"github.com/gogits/gogs/modules/setting"
)

var (
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
	*PullRequest    `xorm:"-"`
	IsClosed        bool
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-"`
	Priority        int
	NumComments     int

	Deadline     time.Time `xorm:"-"`
	DeadlineUnix int64
	Created      time.Time `xorm:"-"`
	CreatedUnix  int64
	Updated      time.Time `xorm:"-"`
	UpdatedUnix  int64

	Attachments []*Attachment `xorm:"-"`
	Comments    []*Comment    `xorm:"-"`
}

func (i *Issue) BeforeInsert() {
	i.CreatedUnix = time.Now().UTC().Unix()
	i.UpdatedUnix = i.CreatedUnix
}

func (i *Issue) BeforeUpdate() {
	i.UpdatedUnix = time.Now().UTC().Unix()
	i.DeadlineUnix = i.Deadline.UTC().Unix()
}

func (i *Issue) AfterSet(colName string, _ xorm.Cell) {
	var err error
	switch colName {
	case "id":
		i.Attachments, err = GetAttachmentsByIssueID(i.ID)
		if err != nil {
			log.Error(3, "GetAttachmentsByIssueID[%d]: %v", i.ID, err)
		}

		i.Comments, err = GetCommentsByIssueID(i.ID)
		if err != nil {
			log.Error(3, "GetCommentsByIssueID[%d]: %v", i.ID, err)
		}

		i.Labels, err = GetLabelsByIssueID(i.ID)
		if err != nil {
			log.Error(3, "GetLabelsByIssueID[%d]: %v", i.ID, err)
		}

	case "poster_id":
		i.Poster, err = GetUserByID(i.PosterID)
		if err != nil {
			if IsErrUserNotExist(err) {
				i.PosterID = -1
				i.Poster = NewFakeUser()
			} else {
				log.Error(3, "GetUserByID[%d]: %v", i.ID, err)
			}
			return
		}

	case "milestone_id":
		if i.MilestoneID == 0 {
			return
		}

		i.Milestone, err = GetMilestoneByID(i.MilestoneID)
		if err != nil {
			log.Error(3, "GetMilestoneById[%d]: %v", i.ID, err)
		}

	case "assignee_id":
		if i.AssigneeID == 0 {
			return
		}

		i.Assignee, err = GetUserByID(i.AssigneeID)
		if err != nil {
			log.Error(3, "GetUserByID[%d]: %v", i.ID, err)
		}

	case "deadline_unix":
		i.Deadline = time.Unix(i.DeadlineUnix, 0).Local()
	case "created_unix":
		i.Created = time.Unix(i.CreatedUnix, 0).Local()
	case "updated_unix":
		i.Updated = time.Unix(i.UpdatedUnix, 0).Local()
	}
}

// HashTag returns unique hash tag for issue.
func (i *Issue) HashTag() string {
	return "issue-" + com.ToStr(i.ID)
}

// State returns string representation of issue status.
func (i *Issue) State() string {
	if i.IsClosed {
		return "closed"
	}
	return "open"
}

// IsPoster returns true if given user by ID is the poster.
func (i *Issue) IsPoster(uid int64) bool {
	return i.PosterID == uid
}

func (i *Issue) hasLabel(e Engine, labelID int64) bool {
	return hasIssueLabel(e, i.ID, labelID)
}

// HasLabel returns true if issue has been labeled by given ID.
func (i *Issue) HasLabel(labelID int64) bool {
	return i.hasLabel(x, labelID)
}

func (i *Issue) addLabel(e *xorm.Session, label *Label) error {
	return newIssueLabel(e, i, label)
}

// AddLabel adds new label to issue by given ID.
func (i *Issue) AddLabel(label *Label) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = i.addLabel(sess, label); err != nil {
		return err
	}

	return sess.Commit()
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

func (i *Issue) removeLabel(e *xorm.Session, label *Label) error {
	return deleteIssueLabel(e, i, label)
}

// RemoveLabel removes a label from issue by given ID.
func (i *Issue) RemoveLabel(label *Label) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = i.removeLabel(sess, label); err != nil {
		return err
	}

	return sess.Commit()
}

func (i *Issue) ClearLabels() (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = i.getLabels(sess); err != nil {
		return err
	}

	for idx := range i.Labels {
		if err = i.removeLabel(sess, i.Labels[idx]); err != nil {
			return err
		}
	}

	return sess.Commit()
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

// ReadBy sets issue to be read by given user.
func (i *Issue) ReadBy(uid int64) error {
	return UpdateIssueUserByRead(uid, i.ID)
}

func (i *Issue) changeStatus(e *xorm.Session, doer *User, repo *Repository, isClosed bool) (err error) {
	// Nothing should be performed if current status is same as target status
	if i.IsClosed == isClosed {
		return nil
	}
	i.IsClosed = isClosed

	if err = updateIssueCols(e, i, "is_closed"); err != nil {
		return err
	} else if err = updateIssueUsersByStatus(e, i.ID, isClosed); err != nil {
		return err
	}

	// Update issue count of labels
	if err = i.getLabels(e); err != nil {
		return err
	}
	for idx := range i.Labels {
		if i.IsClosed {
			i.Labels[idx].NumClosedIssues++
		} else {
			i.Labels[idx].NumClosedIssues--
		}
		if err = updateLabel(e, i.Labels[idx]); err != nil {
			return err
		}
	}

	// Update issue count of milestone
	if err = changeMilestoneIssueStats(e, i); err != nil {
		return err
	}

	// New action comment
	if _, err = createStatusComment(e, doer, repo, i); err != nil {
		return err
	}

	return nil
}

// ChangeStatus changes issue status to open or closed.
func (i *Issue) ChangeStatus(doer *User, repo *Repository, isClosed bool) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = i.changeStatus(sess, doer, repo, isClosed); err != nil {
		return err
	}

	return sess.Commit()
}

func (i *Issue) GetPullRequest() (err error) {
	if i.PullRequest != nil {
		return nil
	}

	i.PullRequest, err = GetPullRequestByIssueID(i.ID)
	return err
}

// It's caller's responsibility to create action.
func newIssue(e *xorm.Session, repo *Repository, issue *Issue, labelIDs []int64, uuids []string, isPull bool) (err error) {
	issue.Name = strings.TrimSpace(issue.Name)
	issue.Index = repo.NextIssueIndex()

	if issue.AssigneeID > 0 {
		// Silently drop invalid assignee
		valid, err := hasAccess(e, &User{Id: issue.AssigneeID}, repo, ACCESS_MODE_WRITE)
		if err != nil {
			return fmt.Errorf("hasAccess: %v", err)
		} else if !valid {
			issue.AssigneeID = 0
		}
	}

	if _, err = e.Insert(issue); err != nil {
		return err
	}

	if isPull {
		_, err = e.Exec("UPDATE `repository` SET num_pulls=num_pulls+1 WHERE id=?", issue.RepoID)
	} else {
		_, err = e.Exec("UPDATE `repository` SET num_issues=num_issues+1 WHERE id=?", issue.RepoID)
	}
	if err != nil {
		return err
	}

	if len(labelIDs) > 0 {
		// During the session, SQLite3 dirver cannot handle retrieve objects after update something.
		// So we have to get all needed labels first.
		labels := make([]*Label, 0, len(labelIDs))
		if err = e.In("id", labelIDs).Find(&labels); err != nil {
			return fmt.Errorf("find all labels: %v", err)
		}

		for _, label := range labels {
			if label.RepoID != repo.ID {
				continue
			}

			if err = issue.addLabel(e, label); err != nil {
				return fmt.Errorf("addLabel: %v", err)
			}
		}
	}

	if issue.MilestoneID > 0 {
		if err = changeMilestoneAssign(e, 0, issue); err != nil {
			return err
		}
	}

	if err = newIssueUsers(e, repo, issue); err != nil {
		return err
	}

	// Check attachments.
	attachments := make([]*Attachment, 0, len(uuids))
	for _, uuid := range uuids {
		attach, err := getAttachmentByUUID(e, uuid)
		if err != nil {
			if IsErrAttachmentNotExist(err) {
				continue
			}
			return fmt.Errorf("getAttachmentByUUID[%s]: %v", uuid, err)
		}
		attachments = append(attachments, attach)
	}

	for i := range attachments {
		attachments[i].IssueID = issue.ID
		// No assign value could be 0, so ignore AllCols().
		if _, err = e.Id(attachments[i].ID).Update(attachments[i]); err != nil {
			return fmt.Errorf("update attachment[%d]: %v", attachments[i].ID, err)
		}
	}

	return nil
}

// NewIssue creates new issue with labels for repository.
func NewIssue(repo *Repository, issue *Issue, labelIDs []int64, uuids []string) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = newIssue(sess, repo, issue, labelIDs, uuids, false); err != nil {
		return fmt.Errorf("newIssue: %v", err)
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	// Notify watchers.
	act := &Action{
		ActUserID:    issue.Poster.Id,
		ActUserName:  issue.Poster.Name,
		ActEmail:     issue.Poster.Email,
		OpType:       ACTION_CREATE_ISSUE,
		Content:      fmt.Sprintf("%d|%s", issue.Index, issue.Name),
		RepoID:       repo.ID,
		RepoUserName: repo.Owner.Name,
		RepoName:     repo.Name,
		IsPrivate:    repo.IsPrivate,
	}
	if err = NotifyWatchers(act); err != nil {
		log.Error(4, "notifyWatchers: %v", err)
	}

	return nil
}

// GetIssueByRef returns an Issue specified by a GFM reference.
// See https://help.github.com/articles/writing-on-github#references for more information on the syntax.
func GetIssueByRef(ref string) (*Issue, error) {
	n := strings.IndexByte(ref, byte('#'))
	if n == -1 {
		return nil, ErrMissingIssueNumber
	}

	index, err := com.StrTo(ref[n+1:]).Int64()
	if err != nil {
		return nil, err
	}

	repo, err := GetRepositoryByRef(ref[:n])
	if err != nil {
		return nil, err
	}

	issue, err := GetIssueByIndex(repo.ID, index)
	if err != nil {
		return nil, err
	}

	issue.Repo = repo
	return issue, nil
}

// GetIssueByIndex returns issue by given index in repository.
func GetIssueByIndex(repoID, index int64) (*Issue, error) {
	issue := &Issue{
		RepoID: repoID,
		Index:  index,
	}
	has, err := x.Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{0, repoID, index}
	}
	return issue, nil
}

// GetIssueByID returns an issue by given ID.
func GetIssueByID(id int64) (*Issue, error) {
	issue := new(Issue)
	has, err := x.Id(id).Get(issue)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrIssueNotExist{id, 0, 0}
	}
	return issue, nil
}

type IssuesOptions struct {
	UserID      int64
	AssigneeID  int64
	RepoID      int64
	PosterID    int64
	MilestoneID int64
	RepoIDs     []int64
	Page        int
	IsClosed    bool
	IsMention   bool
	IsPull      bool
	Labels      string
	SortType    string
}

// Issues returns a list of issues by given conditions.
func Issues(opts *IssuesOptions) ([]*Issue, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}

	sess := x.Limit(setting.IssuePagingNum, (opts.Page-1)*setting.IssuePagingNum)

	if opts.RepoID > 0 {
		sess.Where("issue.repo_id=?", opts.RepoID).And("issue.is_closed=?", opts.IsClosed)
	} else if opts.RepoIDs != nil {
		// In case repository IDs are provided but actually no repository has issue.
		if len(opts.RepoIDs) == 0 {
			return make([]*Issue, 0), nil
		}
		sess.Where("issue.repo_id IN ("+strings.Join(base.Int64sToStrings(opts.RepoIDs), ",")+")").And("issue.is_closed=?", opts.IsClosed)
	} else {
		sess.Where("issue.is_closed=?", opts.IsClosed)
	}

	if opts.AssigneeID > 0 {
		sess.And("issue.assignee_id=?", opts.AssigneeID)
	} else if opts.PosterID > 0 {
		sess.And("issue.poster_id=?", opts.PosterID)
	}

	if opts.MilestoneID > 0 {
		sess.And("issue.milestone_id=?", opts.MilestoneID)
	}

	sess.And("issue.is_pull=?", opts.IsPull)

	switch opts.SortType {
	case "oldest":
		sess.Asc("created_unix")
	case "recentupdate":
		sess.Desc("updated_unix")
	case "leastupdate":
		sess.Asc("updated_unix")
	case "mostcomment":
		sess.Desc("num_comments")
	case "leastcomment":
		sess.Asc("num_comments")
	case "priority":
		sess.Desc("priority")
	default:
		sess.Desc("created_unix")
	}

	labelIDs := base.StringsToInt64s(strings.Split(opts.Labels, ","))
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

	if opts.IsMention {
		queryStr := "issue.id=issue_user.issue_id AND issue_user.is_mentioned=1"
		if opts.UserID > 0 {
			queryStr += " AND issue_user.uid=" + com.ToStr(opts.UserID)
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
	UID         int64 `xorm:"INDEX"` // User ID.
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

func UpdateMentions(userNames []string, issueId int64) error {
	for i := range userNames {
		userNames[i] = strings.ToLower(userNames[i])
	}
	users := make([]*User, 0, len(userNames))

	if err := x.Where("lower_name IN (?)", strings.Join(userNames, "\",\"")).OrderBy("lower_name ASC").Find(&users); err != nil {
		return err
	}

	ids := make([]int64, 0, len(userNames))
	for _, user := range users {
		ids = append(ids, user.Id)
		if !user.IsOrganization() {
			continue
		}

		if user.NumMembers == 0 {
			continue
		}

		tempIds := make([]int64, 0, user.NumMembers)
		orgUsers, err := GetOrgUsersByOrgId(user.Id)
		if err != nil {
			return err
		}

		for _, orgUser := range orgUsers {
			tempIds = append(tempIds, orgUser.ID)
		}

		ids = append(ids, tempIds...)
	}

	if err := UpdateIssueUsersByMentions(ids, issueId); err != nil {
		return err
	}

	return nil
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

type IssueStatsOptions struct {
	RepoID      int64
	UserID      int64
	LabelID     int64
	MilestoneID int64
	AssigneeID  int64
	FilterMode  int
	IsPull      bool
}

// GetIssueStats returns issue statistic information by given conditions.
func GetIssueStats(opts *IssueStatsOptions) *IssueStats {
	stats := &IssueStats{}

	queryStr := "SELECT COUNT(*) FROM `issue` "
	if opts.LabelID > 0 {
		queryStr += "INNER JOIN `issue_label` ON `issue`.id=`issue_label`.issue_id AND `issue_label`.label_id=" + com.ToStr(opts.LabelID)
	}

	baseCond := " WHERE issue.repo_id=" + com.ToStr(opts.RepoID) + " AND issue.is_closed=?"
	if opts.MilestoneID > 0 {
		baseCond += " AND issue.milestone_id=" + com.ToStr(opts.MilestoneID)
	}
	if opts.AssigneeID > 0 {
		baseCond += " AND assignee_id=" + com.ToStr(opts.AssigneeID)
	}
	baseCond += " AND issue.is_pull=?"

	switch opts.FilterMode {
	case FM_ALL, FM_ASSIGN:
		results, _ := x.Query(queryStr+baseCond, false, opts.IsPull)
		stats.OpenCount = parseCountResult(results)
		results, _ = x.Query(queryStr+baseCond, true, opts.IsPull)
		stats.ClosedCount = parseCountResult(results)

	case FM_CREATE:
		baseCond += " AND poster_id=?"
		results, _ := x.Query(queryStr+baseCond, false, opts.IsPull, opts.UserID)
		stats.OpenCount = parseCountResult(results)
		results, _ = x.Query(queryStr+baseCond, true, opts.IsPull, opts.UserID)
		stats.ClosedCount = parseCountResult(results)

	case FM_MENTION:
		queryStr += " INNER JOIN `issue_user` ON `issue`.id=`issue_user`.issue_id"
		baseCond += " AND `issue_user`.uid=? AND `issue_user`.is_mentioned=?"
		results, _ := x.Query(queryStr+baseCond, false, opts.IsPull, opts.UserID, true)
		stats.OpenCount = parseCountResult(results)
		results, _ = x.Query(queryStr+baseCond, true, opts.IsPull, opts.UserID, true)
		stats.ClosedCount = parseCountResult(results)
	}
	return stats
}

// GetUserIssueStats returns issue statistic information for dashboard by given conditions.
func GetUserIssueStats(repoID, uid int64, repoIDs []int64, filterMode int, isPull bool) *IssueStats {
	stats := &IssueStats{}

	queryStr := "SELECT COUNT(*) FROM `issue` "
	baseCond := " WHERE issue.is_closed=?"
	if repoID > 0 || len(repoIDs) == 0 {
		baseCond += " AND issue.repo_id=" + com.ToStr(repoID)
	} else {
		baseCond += " AND issue.repo_id IN (" + strings.Join(base.Int64sToStrings(repoIDs), ",") + ")"
	}

	if isPull {
		baseCond += " AND issue.is_pull=1"
	} else {
		baseCond += " AND issue.is_pull=0"
	}

	results, _ := x.Query(queryStr+baseCond+" AND assignee_id=?", false, uid)
	stats.AssignCount = parseCountResult(results)
	results, _ = x.Query(queryStr+baseCond+" AND poster_id=?", false, uid)
	stats.CreateCount = parseCountResult(results)

	switch filterMode {
	case FM_ASSIGN:
		baseCond += " AND assignee_id=" + com.ToStr(uid)

	case FM_CREATE:
		baseCond += " AND poster_id=" + com.ToStr(uid)
	}

	results, _ = x.Query(queryStr+baseCond, false)
	stats.OpenCount = parseCountResult(results)
	results, _ = x.Query(queryStr+baseCond, true)
	stats.ClosedCount = parseCountResult(results)
	return stats
}

// GetRepoIssueStats returns number of open and closed repository issues by given filter mode.
func GetRepoIssueStats(repoID, uid int64, filterMode int, isPull bool) (numOpen int64, numClosed int64) {
	queryStr := "SELECT COUNT(*) FROM `issue` "
	baseCond := " WHERE issue.repo_id=? AND issue.is_closed=?"

	if isPull {
		baseCond += " AND issue.is_pull=1"
	} else {
		baseCond += " AND issue.is_pull=0"
	}

	switch filterMode {
	case FM_ASSIGN:
		baseCond += " AND assignee_id=" + com.ToStr(uid)

	case FM_CREATE:
		baseCond += " AND poster_id=" + com.ToStr(uid)
	}

	results, _ := x.Query(queryStr+baseCond, repoID, false)
	numOpen = parseCountResult(results)
	results, _ = x.Query(queryStr+baseCond, repoID, true)
	numClosed = parseCountResult(results)
	return numOpen, numClosed
}

func updateIssue(e Engine, issue *Issue) error {
	_, err := e.Id(issue.ID).AllCols().Update(issue)
	return err
}

// UpdateIssue updates all fields of given issue.
func UpdateIssue(issue *Issue) error {
	return updateIssue(x, issue)
}

// updateIssueCols only updates values of specific columns for given issue.
func updateIssueCols(e Engine, issue *Issue, cols ...string) error {
	_, err := e.Id(issue.ID).Cols(cols...).Update(issue)
	return err
}

func updateIssueUsersByStatus(e Engine, issueID int64, isClosed bool) error {
	_, err := e.Exec("UPDATE `issue_user` SET is_closed=? WHERE issue_id=?", isClosed, issueID)
	return err
}

// UpdateIssueUsersByStatus updates issue-user relations by issue status.
func UpdateIssueUsersByStatus(issueID int64, isClosed bool) error {
	return updateIssueUsersByStatus(x, issueID, isClosed)
}

func updateIssueUserByAssignee(e *xorm.Session, issue *Issue) (err error) {
	if _, err = e.Exec("UPDATE `issue_user` SET is_assigned=? WHERE issue_id=?", false, issue.ID); err != nil {
		return err
	}

	// Assignee ID equals to 0 means clear assignee.
	if issue.AssigneeID > 0 {
		if _, err = e.Exec("UPDATE `issue_user` SET is_assigned=? WHERE uid=? AND issue_id=?", true, issue.AssigneeID, issue.ID); err != nil {
			return err
		}
	}

	return updateIssue(e, issue)
}

// UpdateIssueUserByAssignee updates issue-user relation for assignee.
func UpdateIssueUserByAssignee(issue *Issue) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = updateIssueUserByAssignee(sess, issue); err != nil {
		return err
	}

	return sess.Commit()
}

// UpdateIssueUserByRead updates issue-user relation for reading.
func UpdateIssueUserByRead(uid, issueID int64) error {
	_, err := x.Exec("UPDATE `issue_user` SET is_read=? WHERE uid=? AND issue_id=?", true, uid, issueID)
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
	NumOpenIssues   int  `xorm:"-"`
	Completeness    int  // Percentage(1-100).
	IsOverDue       bool `xorm:"-"`

	DeadlineString string    `xorm:"-"`
	Deadline       time.Time `xorm:"-"`
	DeadlineUnix   int64
	ClosedDate     time.Time `xorm:"-"`
	ClosedDateUnix int64
}

func (m *Milestone) BeforeInsert() {
	m.DeadlineUnix = m.Deadline.UTC().Unix()
}

func (m *Milestone) BeforeUpdate() {
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	} else {
		m.Completeness = 0
	}

	m.DeadlineUnix = m.Deadline.UTC().Unix()
	m.ClosedDateUnix = m.ClosedDate.UTC().Unix()
}

func (m *Milestone) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "num_closed_issues":
		m.NumOpenIssues = m.NumIssues - m.NumClosedIssues

	case "deadline_unix":
		m.Deadline = time.Unix(m.DeadlineUnix, 0).Local()
		if m.Deadline.Year() == 9999 {
			return
		}

		m.DeadlineString = m.Deadline.Format("2006-01-02")
		if time.Now().Local().After(m.Deadline) {
			m.IsOverDue = true
		}

	case "closed_date_unix":
		m.ClosedDate = time.Unix(m.ClosedDateUnix, 0).Local()
	}
}

// State returns string representation of milestone status.
func (m *Milestone) State() string {
	if m.IsClosed {
		return "closed"
	}
	return "open"
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
	has, err := e.Get(m)
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

func changeMilestoneIssueStats(e *xorm.Session, issue *Issue) error {
	if issue.MilestoneID == 0 {
		return nil
	}

	m, err := getMilestoneByID(e, issue.MilestoneID)
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

	return updateMilestone(e, m)
}

// ChangeMilestoneIssueStats updates the open/closed issues counter and progress
// for the milestone associated with the given issue.
func ChangeMilestoneIssueStats(issue *Issue) (err error) {
	sess := x.NewSession()
	defer sessionRelease(sess)
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = changeMilestoneIssueStats(sess, issue); err != nil {
		return err
	}

	return sess.Commit()
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
		m, err := getMilestoneByID(e, issue.MilestoneID)
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

	return updateIssue(e, issue)
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
func DeleteMilestoneByID(id int64) error {
	m, err := GetMilestoneByID(id)
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

	if _, err = sess.Id(m.ID).Delete(new(Milestone)); err != nil {
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

// Attachment represent a attachment of issue/comment/release.
type Attachment struct {
	ID        int64  `xorm:"pk autoincr"`
	UUID      string `xorm:"uuid UNIQUE"`
	IssueID   int64  `xorm:"INDEX"`
	CommentID int64
	ReleaseID int64 `xorm:"INDEX"`
	Name      string

	Created     time.Time `xorm:"-"`
	CreatedUnix int64
}

func (a *Attachment) BeforeInsert() {
	a.CreatedUnix = time.Now().UTC().Unix()
}

func (a *Attachment) AfterSet(colName string, _ xorm.Cell) {
	switch colName {
	case "created_unix":
		a.Created = time.Unix(a.CreatedUnix, 0).Local()
	}
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

func getAttachmentByUUID(e Engine, uuid string) (*Attachment, error) {
	attach := &Attachment{UUID: uuid}
	has, err := x.Get(attach)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrAttachmentNotExist{0, uuid}
	}
	return attach, nil
}

// GetAttachmentByUUID returns attachment by given UUID.
func GetAttachmentByUUID(uuid string) (*Attachment, error) {
	return getAttachmentByUUID(x, uuid)
}

// GetAttachmentsByIssueID returns all attachments for given issue by ID.
func GetAttachmentsByIssueID(issueID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, x.Where("issue_id=? AND comment_id=0", issueID).Find(&attachments)
}

// GetAttachmentsByCommentID returns all attachments if comment by given ID.
func GetAttachmentsByCommentID(commentID int64) ([]*Attachment, error) {
	attachments := make([]*Attachment, 0, 10)
	return attachments, x.Where("comment_id=?", commentID).Find(&attachments)
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
	attachments, err := GetAttachmentsByIssueID(issueId)

	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}

// DeleteAttachmentsByComment deletes all attachments associated with the given comment.
func DeleteAttachmentsByComment(commentId int64, remove bool) (int, error) {
	attachments, err := GetAttachmentsByCommentID(commentId)

	if err != nil {
		return 0, err
	}

	return DeleteAttachments(attachments, remove)
}
