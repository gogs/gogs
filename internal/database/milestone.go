package database

import (
	"fmt"
	"time"

	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

// Milestone represents a milestone of repository.
type Milestone struct {
	ID              int64
	RepoID          int64 `xorm:"INDEX"`
	Name            string
	Content         string `xorm:"TEXT"`
	RenderedContent string `xorm:"-" json:"-" gorm:"-"`
	IsClosed        bool
	NumIssues       int
	NumClosedIssues int
	NumOpenIssues   int  `xorm:"-" json:"-" gorm:"-"`
	Completeness    int  // Percentage(1-100).
	IsOverDue       bool `xorm:"-" json:"-" gorm:"-"`

	DeadlineString string    `xorm:"-" json:"-" gorm:"-"`
	Deadline       time.Time `xorm:"-" json:"-" gorm:"-"`
	DeadlineUnix   int64
	ClosedDate     time.Time `xorm:"-" json:"-" gorm:"-"`
	ClosedDateUnix int64
}

// BeforeCreate implements the GORM create hook.
func (m *Milestone) BeforeCreate(_ *gorm.DB) error {
	if m.DeadlineUnix == 0 {
		m.DeadlineUnix = m.Deadline.Unix()
	}
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (m *Milestone) BeforeUpdate(_ *gorm.DB) error {
	if m.NumIssues > 0 {
		m.Completeness = m.NumClosedIssues * 100 / m.NumIssues
	} else {
		m.Completeness = 0
	}

	m.DeadlineUnix = m.Deadline.Unix()
	m.ClosedDateUnix = m.ClosedDate.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (m *Milestone) AfterFind(_ *gorm.DB) error {
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
func (m *Milestone) State() apiv1types.IssueStateType {
	if m.IsClosed {
		return apiv1types.IssueStateClosed
	}
	return apiv1types.IssueStateOpen
}

func (m *Milestone) APIFormat() *apiv1types.IssueMilestone {
	apiMilestone := &apiv1types.IssueMilestone{
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

var _ errx.NotFound = (*ErrMilestoneNotExist)(nil)

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
