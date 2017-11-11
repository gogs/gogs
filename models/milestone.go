// Copyright 2017 The Gogs Authors. All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package models

import (
	"fmt"
	"time"

	"github.com/go-xorm/xorm"
	log "gopkg.in/clog.v1"

	api "github.com/gogits/go-gogs-client"

	"github.com/gogits/gogs/pkg/setting"
)

// Milestone represents a milestone of repository.
type Milestone struct {
	ID              int64
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
	m.DeadlineUnix = m.Deadline.Unix()
}

func (m *Milestone) BeforeUpdate() {
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	} else {
		m.Completeness = 0
	}

	m.DeadlineUnix = m.Deadline.Unix()
	m.ClosedDateUnix = m.ClosedDate.Unix()
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
func (m *Milestone) State() api.StateType {
	if m.IsClosed {
		return api.STATE_CLOSED
	}
	return api.STATE_OPEN
}

func (m *Milestone) ChangeStatus(isClosed bool) error {
	return ChangeMilestoneStatus(m, isClosed)
}

func (m *Milestone) APIFormat() *api.Milestone {
	apiMilestone := &api.Milestone{
		ID:           m.ID,
		State:        m.State(),
		Title:        m.Name,
		Description:  m.Content,
		OpenIssues:   m.NumOpenIssues,
		ClosedIssues: m.NumClosedIssues,
	}
	if m.IsClosed {
		apiMilestone.Closed = &m.ClosedDate
	}
	if m.Deadline.Year() < 9999 {
		apiMilestone.Deadline = &m.Deadline
	}
	return apiMilestone
}

func (m *Milestone) CountIssues(isClosed, includePulls bool) int64 {
	sess := x.Where("milestone_id = ?", m.ID).And("is_closed = ?", isClosed)
	if !includePulls {
		sess.And("is_pull = ?", false)
	}
	count, _ := sess.Count(new(Issue))
	return count
}

// NewMilestone creates new milestone of repository.
func NewMilestone(m *Milestone) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if _, err = sess.Insert(m); err != nil {
		return err
	}

	if _, err = sess.Exec("UPDATE `repository` SET num_milestones = num_milestones + 1 WHERE id = ?", m.RepoID); err != nil {
		return err
	}
	return sess.Commit()
}

func getMilestoneByRepoID(e Engine, repoID, id int64) (*Milestone, error) {
	m := &Milestone{
		ID:     id,
		RepoID: repoID,
	}
	has, err := e.Get(m)
	if err != nil {
		return nil, err
	} else if !has {
		return nil, ErrMilestoneNotExist{id, repoID}
	}
	return m, nil
}

// GetWebhookByRepoID returns the milestone in a repository.
func GetMilestoneByRepoID(repoID, id int64) (*Milestone, error) {
	return getMilestoneByRepoID(x, repoID, id)
}

// GetMilestonesByRepoID returns all milestones of a repository.
func GetMilestonesByRepoID(repoID int64) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, 10)
	return miles, x.Where("repo_id = ?", repoID).Find(&miles)
}

// GetMilestones returns a list of milestones of given repository and status.
func GetMilestones(repoID int64, page int, isClosed bool) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, setting.UI.IssuePagingNum)
	sess := x.Where("repo_id = ? AND is_closed = ?", repoID, isClosed)
	if page > 0 {
		sess = sess.Limit(setting.UI.IssuePagingNum, (page-1)*setting.UI.IssuePagingNum)
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
// If milestone passes with changed values, those values will be
// updated to database as well.
func ChangeMilestoneStatus(m *Milestone, isClosed bool) (err error) {
	repo, err := GetRepositoryByID(m.RepoID)
	if err != nil {
		return err
	}

	sess := x.NewSession()
	defer sess.Close()
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

	m, err := getMilestoneByRepoID(e, issue.RepoID, issue.MilestoneID)
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
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = changeMilestoneIssueStats(sess, issue); err != nil {
		return err
	}

	return sess.Commit()
}

func changeMilestoneAssign(e *xorm.Session, issue *Issue, oldMilestoneID int64) error {
	if oldMilestoneID > 0 {
		m, err := getMilestoneByRepoID(e, issue.RepoID, oldMilestoneID)
		if err != nil {
			return err
		}

		m.NumIssues--
		if issue.IsClosed {
			m.NumClosedIssues--
		}

		if err = updateMilestone(e, m); err != nil {
			return err
		} else if _, err = e.Exec("UPDATE `issue_user` SET milestone_id = 0 WHERE issue_id = ?", issue.ID); err != nil {
			return err
		}

		issue.Milestone = nil
	}

	if issue.MilestoneID > 0 {
		m, err := getMilestoneByRepoID(e, issue.RepoID, issue.MilestoneID)
		if err != nil {
			return err
		}

		m.NumIssues++
		if issue.IsClosed {
			m.NumClosedIssues++
		}

		if err = updateMilestone(e, m); err != nil {
			return err
		} else if _, err = e.Exec("UPDATE `issue_user` SET milestone_id = ? WHERE issue_id = ?", m.ID, issue.ID); err != nil {
			return err
		}

		issue.Milestone = m
	}

	return updateIssue(e, issue)
}

// ChangeMilestoneAssign changes assignment of milestone for issue.
func ChangeMilestoneAssign(doer *User, issue *Issue, oldMilestoneID int64) (err error) {
	sess := x.NewSession()
	defer sess.Close()
	if err = sess.Begin(); err != nil {
		return err
	}

	if err = changeMilestoneAssign(sess, issue, oldMilestoneID); err != nil {
		return err
	}

	if err = sess.Commit(); err != nil {
		return fmt.Errorf("Commit: %v", err)
	}

	var hookAction api.HookIssueAction
	if issue.MilestoneID > 0 {
		hookAction = api.HOOK_ISSUE_MILESTONED
	} else {
		hookAction = api.HOOK_ISSUE_DEMILESTONED
	}

	if issue.IsPull {
		err = issue.PullRequest.LoadIssue()
		if err != nil {
			log.Error(2, "LoadIssue: %v", err)
			return
		}
		err = PrepareWebhooks(issue.Repo, HOOK_EVENT_PULL_REQUEST, &api.PullRequestPayload{
			Action:      hookAction,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormat(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HOOK_EVENT_ISSUES, &api.IssuesPayload{
			Action:     hookAction,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormat(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error(2, "PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
	}

	return nil
}

// DeleteMilestoneOfRepoByID deletes a milestone from a repository.
func DeleteMilestoneOfRepoByID(repoID, id int64) error {
	m, err := GetMilestoneByRepoID(repoID, id)
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
	defer sess.Close()
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

	if _, err = sess.Exec("UPDATE `issue` SET milestone_id = 0 WHERE milestone_id = ?", m.ID); err != nil {
		return err
	} else if _, err = sess.Exec("UPDATE `issue_user` SET milestone_id = 0 WHERE milestone_id = ?", m.ID); err != nil {
		return err
	}
	return sess.Commit()
}
