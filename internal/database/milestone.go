package database

import (
	"fmt"
	"time"

	"github.com/cockroachdb/errors"
	api "github.com/gogs/go-gogs-client"
	"gorm.io/gorm"
	log "unknwon.dev/clog/v2"

	"gogs.io/gogs/internal/conf"
	"gogs.io/gogs/internal/errutil"
)

// Milestone represents a milestone of repository.
type Milestone struct {
	ID              int64
	RepoID          int64 `gorm:"index"`
	Name            string
	Content         string `gorm:"type:text"`
	RenderedContent string `gorm:"-" json:"-"`
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `gorm:"-" json:"-"`
	Completeness    int  // Percentage(1-100).
	IsOverDue       bool `gorm:"-" json:"-"`

	DeadlineString string    `gorm:"-" json:"-"`
	Deadline       time.Time `gorm:"-" json:"-"`
	DeadlineUnix   int64
	ClosedDate     time.Time `gorm:"-" json:"-"`
	ClosedDateUnix int64
}

func (m *Milestone) BeforeCreate(tx *gorm.DB) error {
	m.DeadlineUnix = m.Deadline.Unix()
	return nil
}

func (m *Milestone) BeforeUpdate(tx *gorm.DB) error {
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	} else {
		m.Completeness = 0
	}

	m.DeadlineUnix = m.Deadline.Unix()
	m.ClosedDateUnix = m.ClosedDate.Unix()
	return nil
}

func (m *Milestone) AfterFind(tx *gorm.DB) error {
	m.NumOpenIssues = m.NumIssues - m.NumClosedIssues

	m.Deadline = time.Unix(m.DeadlineUnix, 0).Local()
	if m.Deadline.Year() != 9999 {
		m.DeadlineString = m.Deadline.Format("2006-01-02")
		if time.Now().Local().After(m.Deadline) {
			m.IsOverDue = true
		}
	}

	m.ClosedDate = time.Unix(m.ClosedDateUnix, 0).Local()
	return nil
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
	query := db.Model(new(Issue)).Where("milestone_id = ? AND is_closed = ?", m.ID, isClosed)
	if !includePulls {
		query = query.Where("is_pull = ?", false)
	}
	var count int64
	query.Count(&count)
	return count
}

// NewMilestone creates new milestone of repository.
func NewMilestone(m *Milestone) (err error) {
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(m).Error; err != nil {
			return err
		}

		return tx.Exec("UPDATE `repository` SET num_milestones = num_milestones + 1 WHERE id = ?", m.RepoID).Error
	})
}

var _ errutil.NotFound = (*ErrMilestoneNotExist)(nil)

type ErrMilestoneNotExist struct {
	args map[string]any
}

func IsErrMilestoneNotExist(err error) bool {
	_, ok := err.(ErrMilestoneNotExist)
	return ok
}

func (err ErrMilestoneNotExist) Error() string {
	return fmt.Sprintf("milestone does not exist: %v", err.args)
}

func (ErrMilestoneNotExist) NotFound() bool {
	return true
}

func getMilestoneByRepoID(e *gorm.DB, repoID, id int64) (*Milestone, error) {
	m := &Milestone{}
	err := e.Where("id = ? AND repo_id = ?", id, repoID).First(m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMilestoneNotExist{args: map[string]any{"repoID": repoID, "milestoneID": id}}
		}
		return nil, err
	}
	return m, nil
}

// GetWebhookByRepoID returns the milestone in a repository.
func GetMilestoneByRepoID(repoID, id int64) (*Milestone, error) {
	return getMilestoneByRepoID(db, repoID, id)
}

// GetMilestonesByRepoID returns all milestones of a repository.
func GetMilestonesByRepoID(repoID int64) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, 10)
	return miles, db.Where("repo_id = ?", repoID).Find(&miles).Error
}

// GetMilestones returns a list of milestones of given repository and status.
func GetMilestones(repoID int64, page int, isClosed bool) ([]*Milestone, error) {
	miles := make([]*Milestone, 0, conf.UI.IssuePagingNum)
	query := db.Where("repo_id = ? AND is_closed = ?", repoID, isClosed)
	if page > 0 {
		query = query.Limit(conf.UI.IssuePagingNum).Offset((page - 1) * conf.UI.IssuePagingNum)
	}
	return miles, query.Find(&miles).Error
}

func updateMilestone(e *gorm.DB, m *Milestone) error {
	return e.Model(m).Where("id = ?", m.ID).Updates(m).Error
}

// UpdateMilestone updates information of given milestone.
func UpdateMilestone(m *Milestone) error {
	return updateMilestone(db, m)
}

func countRepoMilestones(e *gorm.DB, repoID int64) int64 {
	var count int64
	e.Model(new(Milestone)).Where("repo_id = ?", repoID).Count(&count)
	return count
}

// CountRepoMilestones returns number of milestones in given repository.
func CountRepoMilestones(repoID int64) int64 {
	return countRepoMilestones(db, repoID)
}

func countRepoClosedMilestones(e *gorm.DB, repoID int64) int64 {
	var count int64
	e.Model(new(Milestone)).Where("repo_id = ? AND is_closed = ?", repoID, true).Count(&count)
	return count
}

// CountRepoClosedMilestones returns number of closed milestones in given repository.
func CountRepoClosedMilestones(repoID int64) int64 {
	return countRepoClosedMilestones(db, repoID)
}

// MilestoneStats returns number of open and closed milestones of given repository.
func MilestoneStats(repoID int64) (open, closed int64) {
	db.Model(new(Milestone)).Where("repo_id = ? AND is_closed = ?", repoID, false).Count(&open)
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

	return db.Transaction(func(tx *gorm.DB) error {
		m.IsClosed = isClosed
		if err := updateMilestone(tx, m); err != nil {
			return err
		}

		repo.NumMilestones = int(countRepoMilestones(tx, repo.ID))
		repo.NumClosedMilestones = int(countRepoClosedMilestones(tx, repo.ID))
		return tx.Model(repo).Where("id = ?", repo.ID).Updates(repo).Error
	})
}

func changeMilestoneIssueStats(e *gorm.DB, issue *Issue) error {
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
	return db.Transaction(func(tx *gorm.DB) error {
		return changeMilestoneIssueStats(tx, issue)
	})
}

func changeMilestoneAssign(e *gorm.DB, issue *Issue, oldMilestoneID int64) error {
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
		}

		if err = e.Exec("UPDATE `issue_user` SET milestone_id = 0 WHERE issue_id = ?", issue.ID).Error; err != nil {
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
		}

		if err = e.Exec("UPDATE `issue_user` SET milestone_id = ? WHERE issue_id = ?", m.ID, issue.ID).Error; err != nil {
			return err
		}

		issue.Milestone = m
	}

	return updateIssue(e, issue)
}

// ChangeMilestoneAssign changes assignment of milestone for issue.
func ChangeMilestoneAssign(doer *User, issue *Issue, oldMilestoneID int64) (err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		return changeMilestoneAssign(tx, issue, oldMilestoneID)
	})
	if err != nil {
		return errors.Newf("transaction: %v", err)
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
			log.Error("LoadIssue: %v", err)
			return err
		}
		err = PrepareWebhooks(issue.Repo, HookEventTypePullRequest, &api.PullRequestPayload{
			Action:      hookAction,
			Index:       issue.Index,
			PullRequest: issue.PullRequest.APIFormat(),
			Repository:  issue.Repo.APIFormatLegacy(nil),
			Sender:      doer.APIFormat(),
		})
	} else {
		err = PrepareWebhooks(issue.Repo, HookEventTypeIssues, &api.IssuesPayload{
			Action:     hookAction,
			Index:      issue.Index,
			Issue:      issue.APIFormat(),
			Repository: issue.Repo.APIFormatLegacy(nil),
			Sender:     doer.APIFormat(),
		})
	}
	if err != nil {
		log.Error("PrepareWebhooks [is_pull: %v]: %v", issue.IsPull, err)
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

	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ?", m.ID).Delete(new(Milestone)).Error; err != nil {
			return err
		}

		repo.NumMilestones = int(countRepoMilestones(tx, repo.ID))
		repo.NumClosedMilestones = int(countRepoClosedMilestones(tx, repo.ID))
		if err := tx.Model(repo).Where("id = ?", repo.ID).Updates(repo).Error; err != nil {
			return err
		}

		if err := tx.Exec("UPDATE `issue` SET milestone_id = 0 WHERE milestone_id = ?", m.ID).Error; err != nil {
			return err
		}

		return tx.Exec("UPDATE `issue_user` SET milestone_id = 0 WHERE milestone_id = ?", m.ID).Error
	})
}
