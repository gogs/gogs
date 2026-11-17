package database

import (
	"fmt"
	"strconv"
	"time"

	"github.com/cockroachdb/errors"
	"gorm.io/gorm"

	"gogs.io/gogs/internal/errx"
	apiv1types "gogs.io/gogs/internal/route/api/v1/types"
)

var ErrMissingIssueNumber = errors.New("no issue number specified")

// Issue represents an issue or pull request of repository.
type Issue struct {
	ID              int64       `gorm:"primaryKey"`
	RepoID          int64       `gorm:"index:IDX_issue_repo_id;uniqueIndex:UQE_issue_repo_index"`
	Repo            *Repository `json:"-" gorm:"-"`
	Index           int64       `gorm:"uniqueIndex:UQE_issue_repo_index"` // Index in one repository.
	PosterID        int64
	Poster          *User    `json:"-" gorm:"-"`
	Title           string   `gorm:"column:name;size:255"`
	Content         string   `gorm:"type:TEXT"`
	RenderedContent string   `json:"-" gorm:"-"`
	Labels          []*Label `json:"-" gorm:"-"`
	MilestoneID     int64
	Milestone       *Milestone `json:"-" gorm:"-"`
	Priority        int        `gorm:"size:32"`
	AssigneeID      int64
	Assignee        *User `json:"-" gorm:"-"`
	IsClosed        bool
	IsRead          bool         `json:"-" gorm:"-"`
	IsPull          bool         // Indicates whether is a pull request or not.
	PullRequest     *PullRequest `json:"-" gorm:"-"`
	NumComments     int          `gorm:"size:32"`

	Deadline     time.Time `json:"-" gorm:"-"`
	DeadlineUnix int64
	Created      time.Time `json:"-" gorm:"-"`
	CreatedUnix  int64
	Updated      time.Time `json:"-" gorm:"-"`
	UpdatedUnix  int64

	Attachments []*Attachment `json:"-" gorm:"-"`
	Comments    []*Comment    `json:"-" gorm:"-"`
}

// BeforeCreate implements the GORM create hook.
func (issue *Issue) BeforeCreate(tx *gorm.DB) error {
	if issue.CreatedUnix == 0 {
		issue.CreatedUnix = tx.NowFunc().Unix()
		issue.UpdatedUnix = issue.CreatedUnix
	}
	// Webhook payloads and API responses use the issue just created.
	issue.Created = time.Unix(issue.CreatedUnix, 0).Local()
	issue.Updated = time.Unix(issue.UpdatedUnix, 0).Local()
	return nil
}

// BeforeUpdate implements the GORM update hook.
func (issue *Issue) BeforeUpdate(tx *gorm.DB) error {
	issue.UpdatedUnix = tx.NowFunc().Unix()
	issue.DeadlineUnix = issue.Deadline.Unix()
	return nil
}

// AfterFind implements the GORM query hook.
func (issue *Issue) AfterFind(_ *gorm.DB) error {
	issue.Deadline = time.Unix(issue.DeadlineUnix, 0).Local()
	issue.Created = time.Unix(issue.CreatedUnix, 0).Local()
	issue.Updated = time.Unix(issue.UpdatedUnix, 0).Local()
	return nil
}

func (issue *Issue) HTMLURL() string {
	var path string
	if issue.IsPull {
		path = "pulls"
	} else {
		path = "issues"
	}
	return fmt.Sprintf("%s/%s/%d", issue.Repo.HTMLURL(), path, issue.Index)
}

// State returns string representation of issue status.
func (issue *Issue) State() apiv1types.IssueStateType {
	if issue.IsClosed {
		return apiv1types.IssueStateClosed
	}
	return apiv1types.IssueStateOpen
}

// This method assumes some fields assigned with values:
// Required - Poster, Labels,
// Optional - Milestone, Assignee, PullRequest
func (issue *Issue) APIFormat() *apiv1types.Issue {
	apiLabels := make([]*apiv1types.IssueLabel, len(issue.Labels))
	for i := range issue.Labels {
		apiLabels[i] = issue.Labels[i].APIFormat()
	}

	apiIssue := &apiv1types.Issue{
		ID:       issue.ID,
		Index:    issue.Index,
		Poster:   issue.Poster.APIFormat(),
		Title:    issue.Title,
		Body:     issue.Content,
		Labels:   apiLabels,
		State:    issue.State(),
		Comments: issue.NumComments,
		Created:  issue.Created,
		Updated:  issue.Updated,
	}

	if issue.Milestone != nil {
		apiIssue.Milestone = issue.Milestone.APIFormat()
	}
	if issue.Assignee != nil {
		apiIssue.Assignee = issue.Assignee.APIFormat()
	}
	if issue.IsPull {
		apiIssue.PullRequest = &apiv1types.PullRequestMeta{
			HasMerged: issue.PullRequest.HasMerged,
		}
		if issue.PullRequest.HasMerged {
			apiIssue.PullRequest.Merged = &issue.PullRequest.Merged
		}
	}

	return apiIssue
}

// HashTag returns unique hash tag for issue.
func (issue *Issue) HashTag() string {
	return "issue-" + strconv.FormatInt(issue.ID, 10)
}

// IsPoster returns true if given user by ID is the poster.
func (issue *Issue) IsPoster(uid int64) bool {
	return issue.PosterID == uid
}

type NewIssueOptions struct {
	Repo        *Repository
	Issue       *Issue
	LableIDs    []int64
	Attachments []string // In UUID format.
	IsPull      bool
}

var _ errx.NotFound = (*ErrIssueNotExist)(nil)

type ErrIssueNotExist struct {
	args map[string]any
}

func IsErrIssueNotExist(err error) bool {
	_, ok := err.(ErrIssueNotExist)
	return ok
}

func (err ErrIssueNotExist) Error() string {
	return fmt.Sprintf("issue does not exist: %v", err.args)
}

func (ErrIssueNotExist) NotFound() bool {
	return true
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

// SuggestIssuesOptions is the options for SuggestIssues.
type SuggestIssuesOptions struct {
	RepoID int64
	// Keyword matches the issues whose number starts with it or whose title
	// contains it, ignoring case. An empty keyword matches every issue.
	Keyword       string
	IncludeIssues bool
	IncludePulls  bool
	Limit         int
}

// .___                             ____ ___
// |   | ______ ________ __   ____ |    |   \______ ___________
// |   |/  ___//  ___/  |  \_/ __ \|    |   /  ___// __ \_  __ \
// |   |\___ \ \___ \|  |  /\  ___/|    |  /\___ \\  ___/|  | \/
// |___/____  >____  >____/  \___  >______//____  >\___  >__|
//          \/     \/            \/             \/     \/

// IssueUser represents an issue-user relation.
type IssueUser struct {
	ID          int64 `gorm:"primary_key"`
	UserID      int64 `gorm:"column:uid;index:IDX_issue_user_uid"`
	IssueID     int64
	RepoID      int64 `gorm:"index:IDX_issue_user_repo_id"`
	MilestoneID int64
	IsRead      bool
	IsAssigned  bool
	IsMentioned bool
	IsPoster    bool
	IsClosed    bool
}

// PairsContains returns true when pairs list contains given issue.
func PairsContains(ius []*IssueUser, issueID, uid int64) int {
	for i := range ius {
		if ius[i].IssueID == issueID &&
			ius[i].UserID == uid {
			return i
		}
	}
	return -1
}

// IssueStats represents issue statistic information.
type IssueStats struct {
	OpenCount, ClosedCount int64
	YourReposCount         int64
	AssignCount            int64
	CreateCount            int64
	MentionCount           int64
}

type FilterMode string

const (
	FilterModeYourRepos FilterMode = "your_repositories"
	FilterModeAssign    FilterMode = "assigned"
	FilterModeCreate    FilterMode = "created_by"
	FilterModeMention   FilterMode = "mentioned"
)

type IssueStatsOptions struct {
	RepoID      int64
	UserID      int64
	Labels      string
	MilestoneID int64
	AssigneeID  int64
	FilterMode  FilterMode
	IsPull      bool
}
